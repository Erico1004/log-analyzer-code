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
// 策略：KeyErrors 精确匹配 > 通用模式匹配 > 兜底分类词
func (r *KnowledgeRetriever) extractKeywords(ctx *model.LogContext) []string {
	var found []string

	// 第一优先级：使用预处理阶段已提取的关键错误信息
	if len(ctx.KeyErrors) > 0 {
		found = append(found, ctx.KeyErrors...)
	}

	// 第二优先级：从处理后的文本中匹配运维特征模式
	text := ctx.ProcessedText
	textUpper := strings.ToUpper(text)

	// 运维场景通用的特征模式（大幅扩展，覆盖主流产品和错误码）
	patterns := []string{
		// 错误码
		"502", "503", "500", "504", "401", "403",
		"ORA-01157", "ORA-01110",
		// 内存
		"OOM", "out of memory", "OutOfMemoryError", "memory leak",
		"memory pressure", "memory limit", "memory exhausted",
		"flushing caches", "RADAR_PRE_LEAK",
		// 磁盘
		"no space left", "disk full", "disk usage 100%", "disk space exhausted",
		"quota exceeded", "NTFS", "CHKDSK", "corruption",
		// 网络
		"connection refused", "connection timeout", "connection reset",
		"bad gateway", "service unavailable", "unreachable",
		"DNS", "lookup failed", "SRCADDRFAIL", "unknown host",
		// 认证权限
		"permission denied", "access denied", "authentication failed",
		"unauthorized", "insufficient privileges",
		// 数据库
		"deadlock", "lock wait timeout", "database corruption",
		"cannot identify", "data file",
		// 进程/系统
		"core dump", "COREDUMP", "killed", "oom-killer", "KILL",
		"reboot", "restart", "fatal",
		// 配置
		"configuration file", "invalid parameter", "parse error",
		"config mismatch", "orphaned", "node leases",
		// 具体产品
		"Cisco", "AKS", "Kubernetes", "kube-apiserver",
		"InfluxDB", "Trello", "HAProxy", "haproxy",
		"OpenShift", "OVN", "Oracle", "Apex One",
		"TwistLock", "VG-Manager", "RAID",
		"SDP1", "A-SMGCS",
		// CPU
		"CPU", "cpu credits", "high cpu", "throttled",
		"CPU threshold", "CPU utilization",
		// 硬件
		"cache module", "health indicator", "write-through",
		"emergency maintenance", "emergency stop",
		// 磁盘阵列
		"hot spare", "physical disk",
	}

	for _, p := range patterns {
		if strings.Contains(textUpper, strings.ToUpper(p)) {
			found = append(found, p)
		}
	}

	// 额外：从方括号中提取产品名 [ProductName]
	// 日志格式: 2025-09-27 01:00:00 ERROR [SDP1] Server cache module fault detected.
	remainder := text
	for {
		start := strings.Index(remainder, "[")
		if start == -1 {
			break
		}
		end := strings.Index(remainder[start:], "]")
		if end == -1 {
			break
		}
		productName := remainder[start+1 : start+end]
		// 过滤掉太长的（不是产品名）、纯数字、日志级别等
		if len(productName) >= 2 && len(productName) <= 30 &&
			!strings.ContainsAny(productName, " ") &&
			productName != "ERROR" && productName != "WARN" && productName != "INFO" &&
			productName != "CRITICAL" && productName != "DEBUG" && productName != "TRACE" {
			found = append(found, productName)
		}
		remainder = remainder[start+end+1:]
	}

	// 去重
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

// Retrieve 执行混合知识检索（FULLTEXT + symptoms LIKE）
func (r *KnowledgeRetriever) Retrieve(ctx *model.LogContext, topK int, threshold float64) ([]model.KnowledgeItem, error) {
	keywords := r.extractKeywords(ctx)
	keywordStr := strings.Join(keywords, " ")
	fmt.Printf("[检索关键词] %s\n", keywordStr)

	// 用于去重的集合
	seenIDs := make(map[int]bool)
	type scored struct {
		item  model.KnowledgeBase
		score float64
	}
	var allResults []scored

	// ──── 策略1: FULLTEXT 全文检索（content + keywords 字段）────
	var ftResults []struct {
		model.KnowledgeBase
		Score float64 `json:"score"`
	}
	ftQuery := r.db.Model(&model.KnowledgeBase{}).
		Select("*, MATCH(content, keywords) AGAINST(? IN NATURAL LANGUAGE MODE) as score", keywordStr).
		Where("MATCH(content, keywords) AGAINST(? IN NATURAL LANGUAGE MODE)", keywordStr).
		Order("score DESC").
		Limit(topK)

	if err := ftQuery.Find(&ftResults).Error; err == nil {
		for _, res := range ftResults {
			if !seenIDs[res.ID] {
				seenIDs[res.ID] = true
				allResults = append(allResults, scored{item: res.KnowledgeBase, score: res.Score})
			}
		}
	}

	// ──── 策略2: symptoms 字段 LIKE 匹配（精确特征串，FULLTEXT 的补充）────
	for _, kw := range keywords {
		if len(kw) < 2 {
			continue
		}
		var symResults []model.KnowledgeBase
		likePattern := "%" + kw + "%"
		r.db.Model(&model.KnowledgeBase{}).
			Where("symptoms LIKE ?", likePattern).
			Limit(topK).
			Find(&symResults)
		for _, res := range symResults {
			if !seenIDs[res.ID] {
				seenIDs[res.ID] = true
				// LIKE 命中给固定高分 1.5，确保不低于 FULLTEXT 结果
				allResults = append(allResults, scored{item: res, score: 1.5})
			}
		}
	}

	// ──── 归一化 & 阈值过滤 ────
	if len(allResults) == 0 {
		fmt.Printf("[检索结果] 0 条\n")
		return nil, nil
	}

	// 计算最高分用于归一化
	maxScore := allResults[0].score
	for _, r := range allResults {
		if r.score > maxScore {
			maxScore = r.score
		}
	}
	if maxScore <= 0 {
		maxScore = 1
	}

	items := make([]model.KnowledgeItem, 0)
	for _, r := range allResults {
		ns := r.score / maxScore
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

	// 按分数降序排列，限制 topK
	if len(items) > topK {
		items = items[:topK]
	}

	fmt.Printf("[检索结果] %d 条 (FULLTEXT+LIKE混合)\n", len(items))
	return items, nil
}
