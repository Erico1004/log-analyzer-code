package retriever

import (
	"fmt"
	"strings"

	"log-analyzer/model"

	"gorm.io/gorm"
)

type KnowledgeRetriever struct {
	db *gorm.DB
}

func NewKnowledgeRetriever(db *gorm.DB) *KnowledgeRetriever {
	return &KnowledgeRetriever{db: db}
}

// extractKeywords 从日志上下文中提取检索关键词
// 优化版本：直接使用KeyErrors中已提取的精确错误类型
func (r *KnowledgeRetriever) extractKeywords(ctx *model.LogContext) string {
	// 优先使用已提取的关键错误信息（这些是精确的错误类型）
	if len(ctx.KeyErrors) > 0 {
		return strings.Join(ctx.KeyErrors, " ")
	}

	// 从处理后的文本中提取高频异常词汇
	text := ctx.ProcessedText
	patterns := []string{
		"OutOfMemoryError", "Java heap space", "GC overhead",
		"NullPointerException", "ClassCastException",
		"Connection refused", "ConnectException", "Communications link failure",
		"SocketTimeoutException", "Read timed out",
		"Deadlock", "Lock wait timeout",
		"No space left", "Disk full", "quota exceeded",
		"502", "503", "Bad Gateway", "Service Unavailable",
		"Permission denied", "Access denied",
		"CPU", "memory leak", "Metaspace",
		"UnknownHostException", "DNS",
	}

	textUpper := strings.ToUpper(text)
	var found []string
	for _, p := range patterns {
		if strings.Contains(textUpper, strings.ToUpper(p)) {
			found = append(found, p)
		}
	}

	if len(found) == 0 {
		return "运维 故障 排查"
	}

	// 去重
	seen := make(map[string]bool)
	var unique []string
	for _, f := range found {
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}

	return strings.Join(unique, " ")
}

// Retrieve 执行知识检索
func (r *KnowledgeRetriever) Retrieve(ctx *model.LogContext, topK int, threshold float64) ([]model.KnowledgeItem, error) {
	keywords := r.extractKeywords(ctx)

	// 调试日志
	fmt.Printf("[检索关键词] %s\n", keywords)

	var results []struct {
		model.KnowledgeBase
		Score float64 `json:"score"`
	}

	// 使用全文检索
	query := r.db.Model(&model.KnowledgeBase{}).
		Select("*, MATCH(content, keywords) AGAINST(? IN NATURAL LANGUAGE MODE) as score", keywords).
		Where("MATCH(content, keywords) AGAINST(? IN NATURAL LANGUAGE MODE)", keywords).
		Order("score DESC").
		Limit(topK)

	if err := query.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("检索查询失败: %w", err)
	}

	items := make([]model.KnowledgeItem, 0)
	if len(results) == 0 {
		return items, nil
	}

	// 相对于批次最高分进行归一化，避免硬编码除数导致的阈值失效
	maxScore := results[0].Score
	if maxScore <= 0 {
		maxScore = 1
	}
	for _, res := range results {
		normalizedScore := res.Score / maxScore
		if normalizedScore > 1.0 {
			normalizedScore = 1.0
		}

		if normalizedScore >= threshold {
			items = append(items, model.KnowledgeItem{
				ID:              res.ID,
				Title:           res.Title,
				Content:         res.Content,
				Category:        res.Category,
				SimilarityScore: normalizedScore,
			})
		}
	}

	return items, nil
}
