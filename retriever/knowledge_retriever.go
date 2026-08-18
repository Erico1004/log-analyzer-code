package retriever

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"log-analyzer/llm"
	"log-analyzer/model"

	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

type KnowledgeRetriever struct {
	db       *gorm.DB
	embedder *llm.EmbeddingAdapter
}

func NewKnowledgeRetriever(db *gorm.DB, embedder *llm.EmbeddingAdapter) *KnowledgeRetriever {
	return &KnowledgeRetriever{db: db, embedder: embedder}
}

func (r *KnowledgeRetriever) extractKeywords(ctx *model.LogContext) []string {
	var found []string

	if len(ctx.KeyErrors) > 0 {
		found = append(found, ctx.KeyErrors...)
	}

	text := ctx.ProcessedText
	textUpper := strings.ToUpper(text)

	patterns := []string{
		"502", "503", "500", "504", "401", "403",
		"ORA-01157", "ORA-01110",
		"OOM", "out of memory", "OutOfMemoryError", "memory leak",
		"memory pressure", "memory limit", "memory exhausted",
		"no space left", "disk full", "disk usage 100%", "disk space exhausted",
		"connection refused", "connection timeout", "connection reset",
		"bad gateway", "service unavailable", "unreachable",
		"permission denied", "access denied", "authentication failed",
		"deadlock", "lock wait timeout", "database corruption",
		"core dump", "COREDUMP", "killed", "oom-killer",
		"configuration file", "invalid parameter", "parse error",
		"CPU", "cpu credits", "high cpu", "throttled",
	}

	for _, p := range patterns {
		if strings.Contains(textUpper, strings.ToUpper(p)) {
			found = append(found, p)
		}
	}

	seen := make(map[string]bool)
	var unique []string
	for _, f := range found {
		upper := strings.ToUpper(f)
		if !seen[upper] {
			seen[upper] = true
			unique = append(unique, f)
		}
	}

	if len(unique) == 0 {
		unique = []string{"运维", "故障", "排查"}
	}

	return unique
}

type scoredResult struct {
	item     model.KnowledgeBase
	ftScore  float64
	vecScore float64
	rrfScore float64
}

func (r *KnowledgeRetriever) Retrieve(ctx *model.LogContext, topK int, threshold float64) ([]model.KnowledgeItem, error) {
	keywords := r.extractKeywords(ctx)
	keywordStr := strings.Join(keywords, " ")
	log.Printf("[检索关键词] %s", keywordStr)

	var allResults []scoredResult
	seenIDs := make(map[int]bool)

	// 策略1: PostgreSQL 全文检索 (GIN 索引)
	ftResults := r.searchFullText(keywordStr, topK)
	for i, res := range ftResults {
		if !seenIDs[res.ID] {
			seenIDs[res.ID] = true
			allResults = append(allResults, scoredResult{
				item:    res,
				ftScore: float64(len(ftResults) - i),
			})
		}
	}

	// 策略2: symptoms 字段 LIKE 匹配
	likeResults := r.searchLike(keywords, topK)
	for _, res := range likeResults {
		if !seenIDs[res.ID] {
			seenIDs[res.ID] = true
			allResults = append(allResults, scoredResult{
				item:    res,
				ftScore: 1.5,
			})
		}
	}

	// 策略3: pgvector HNSW 向量检索（ANN 索引，O(log n)）
	if r.embedder != nil {
		queryEmbedding, err := r.embedder.Embed(ctx.ProcessedText)
		if err != nil {
			log.Printf("[向量检索] embedding 生成失败: %v", err)
		} else {
			vecResults := r.searchVector(queryEmbedding, topK)
			for _, res := range vecResults {
				if !seenIDs[res.item.ID] {
					seenIDs[res.item.ID] = true
					allResults = append(allResults, res)
				}
			}
		}
	}

	// RRF 融合：对每个结果计算 RRF 分数
	k := 60.0
	for i := range allResults {
		rrf := 0.0
		if allResults[i].ftScore > 0 {
			rrf += 1.0 / (k + allResults[i].ftScore)
		}
		if allResults[i].vecScore > 0 {
			rrf += 1.0 / (k + allResults[i].vecScore)
		}
		allResults[i].rrfScore = rrf
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].rrfScore > allResults[j].rrfScore
	})

	if len(allResults) == 0 {
		log.Printf("[检索结果] 0 条")
		return nil, nil
	}

	maxRRF := allResults[0].rrfScore
	if maxRRF <= 0 {
		maxRRF = 1
	}

	items := make([]model.KnowledgeItem, 0)
	for _, r := range allResults {
		ns := r.rrfScore / maxRRF
		if ns >= threshold {
			items = append(items, model.KnowledgeItem{
				ID:              r.item.ID,
				Title:           r.item.Title,
				Content:         r.item.Content,
				Category:        r.item.Category,
				SimilarityScore: ns,
			})
		}
	}

	if len(items) > topK {
		items = items[:topK]
	}

	log.Printf("[检索结果] %d 条 (PostgreSQL FTS + pgvector HNSW + RRF 融合)", len(items))
	return items, nil
}

// searchFullText PostgreSQL 原生全文检索（GIN 索引）
func (r *KnowledgeRetriever) searchFullText(keywordStr string, topK int) []model.KnowledgeBase {
	var results []model.KnowledgeBase
	r.db.Raw(`SELECT * FROM knowledge_base
		WHERE to_tsvector('simple', coalesce(content, '') || ' ' || coalesce(keywords, ''))
			@@ plainto_tsquery('simple', ?)
		ORDER BY ts_rank(to_tsvector('simple', coalesce(content, '') || ' ' || coalesce(keywords, '')), plainto_tsquery('simple', ?)) DESC
		LIMIT ?`, keywordStr, keywordStr, topK).Scan(&results)
	return results
}

func (r *KnowledgeRetriever) searchLike(keywords []string, topK int) []model.KnowledgeBase {
	keywordSet := make(map[string]bool)
	for _, kw := range keywords {
		if len(kw) >= 2 {
			keywordSet[kw] = true
		}
	}

	var results []model.KnowledgeBase
	for kw := range keywordSet {
		var batch []model.KnowledgeBase
		likePattern := "%" + kw + "%"
		r.db.Model(&model.KnowledgeBase{}).
			Where("symptoms LIKE ?", likePattern).
			Limit(topK).
			Find(&batch)
		results = append(results, batch...)
	}

	seen := make(map[int]bool)
	var unique []model.KnowledgeBase
	for _, item := range results {
		if !seen[item.ID] {
			seen[item.ID] = true
			unique = append(unique, item)
		}
	}

	return unique
}

// searchVector 使用 pgvector HNSW 索引进行 ANN 检索
// <=> 是余弦距离算子，返回 0(完全相同) ~ 2(完全相反)
// HNSW 索引保证 O(log n) 复杂度，而非暴力扫描
func (r *KnowledgeRetriever) searchVector(queryEmbedding []float64, topK int) []scoredResult {
	queryVec := pgvector.NewVector(llm.ToFloat32(queryEmbedding))

	type vecResult struct {
		model.KnowledgeBase
		Distance float64 `gorm:"column:distance"`
	}

	var results []vecResult
	err := r.db.Raw(`SELECT *, embedding <=> ? AS distance
		FROM knowledge_base
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> ?
		LIMIT ?`, queryVec, queryVec, topK).Scan(&results).Error

	if err != nil {
		log.Printf("[向量检索] pgvector 查询失败: %v", err)
		return nil
	}

	scored := make([]scoredResult, 0, len(results))
	for _, res := range results {
		// 余弦距离转相似度: similarity = 1 - distance
		sim := 1.0 - res.Distance
		if sim > 0 {
			scored = append(scored, scoredResult{
				item:     res.KnowledgeBase,
				vecScore: sim,
			})
		}
	}

	// 将相似度转为排名用于 RRF
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].vecScore > scored[j].vecScore
	})

	for i := range scored {
		scored[i].vecScore = float64(i + 1)
	}

	log.Printf("[向量检索] HNSW 召回 %d 条 (pgvector ANN)", len(scored))
	return scored
}

func (r *KnowledgeRetriever) GetEmbeddingCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.KnowledgeBase{}).
		Where("embedding IS NOT NULL").
		Count(&count).Error
	return count, err
}

func (r *KnowledgeRetriever) String() string {
	return fmt.Sprintf("KnowledgeRetriever(embedder=%v)", r.embedder != nil)
}
