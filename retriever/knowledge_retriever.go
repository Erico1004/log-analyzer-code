package retriever

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"log-analyzer/llm"
	"log-analyzer/model"

	"gorm.io/gorm"
)

type KnowledgeRetriever struct {
	db      *gorm.DB
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

	// 策略1: FULLTEXT 全文检索
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
				item:     res,
				ftScore:  1.5,
				vecScore: 0,
			})
		}
	}

	// 策略3: 向量检索（语义召回）
	if r.embedder != nil {
		queryText := ctx.ProcessedText
		queryEmbedding, err := r.embedder.Embed(queryText)
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
	// RRF = 1 / (k + rank_ft) + 1 / (k + rank_vec)，k=60 是标准参数
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

	// 按 RRF 分数排序
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].rrfScore > allResults[j].rrfScore
	})

	// 归一化 + 阈值过滤
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

	log.Printf("[检索结果] %d 条 (FULLTEXT+LIKE+向量RRF融合)", len(items))
	return items, nil
}

func (r *KnowledgeRetriever) searchFullText(keywordStr string, topK int) []model.KnowledgeBase {
	var results []model.KnowledgeBase
	r.db.Raw(`SELECT *, MATCH(content, keywords) AGAINST(? IN NATURAL LANGUAGE MODE) as score
		FROM knowledge_base
		WHERE MATCH(content, keywords) AGAINST(? IN NATURAL LANGUAGE MODE)
		ORDER BY score DESC
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

	// 去重
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

func (r *KnowledgeRetriever) searchVector(queryEmbedding []float64, topK int) []scoredResult {
	var all []model.KnowledgeBase
	if err := r.db.Where("embedding IS NOT NULL AND JSON_LENGTH(embedding) > 0").Find(&all).Error; err != nil {
		log.Printf("[向量检索] 查询失败: %v", err)
		return nil
	}

	scored := make([]scoredResult, 0)
	for _, item := range all {
		if len(item.Embedding) == 0 {
			continue
		}
		sim := llm.CosineSimilarity(queryEmbedding, item.Embedding)
		if sim > 0 {
			scored = append(scored, scoredResult{
				item:     item,
				vecScore: sim,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].vecScore > scored[j].vecScore
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	for i := range scored {
		scored[i].vecScore = float64(i + 1)
	}

	log.Printf("[向量检索] 召回 %d 条 (cosine>0), topK=%d", len(scored), topK)
	return scored
}

func (r *KnowledgeRetriever) GetEmbeddingCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.KnowledgeBase{}).
		Where("embedding IS NOT NULL AND JSON_LENGTH(embedding) > 0").
		Count(&count).Error
	return count, err
}

func (r *KnowledgeRetriever) String() string {
	return fmt.Sprintf("KnowledgeRetriever(embedder=%v)", r.embedder != nil)
}