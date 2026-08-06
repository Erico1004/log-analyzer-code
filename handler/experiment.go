package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"log-analyzer/database"
	"log-analyzer/experiment"
	"log-analyzer/llm"
	"log-analyzer/model"
	"log-analyzer/preprocessor"
	"log-analyzer/prompt"
	"log-analyzer/retriever"
)

// ExperimentHandler 实验运行处理器
type ExperimentHandler struct {
	llmAdapter *llm.DeepSeekAdapter
}

// NewExperimentHandler 创建实验处理器
func NewExperimentHandler(llmAdapter *llm.DeepSeekAdapter) *ExperimentHandler {
	return &ExperimentHandler{llmAdapter: llmAdapter}
}

// ExperimentRunRequest 实验运行请求
type ExperimentRunRequest struct {
	Strategy string `json:"strategy"` // ZERO_SHOT / FEW_SHOT / COT / ALL
	UseRAG   bool   `json:"use_rag"`  // true=RAG增强, false=Direct LLM
	Limit    int    `json:"limit"`    // 限制运行用例数, 0=全部
}

// CaseResult 单个用例的实验结果
type CaseResult struct {
	CaseID         string  `json:"case_id"`
	Category       string  `json:"category"`
	LogSnippet     string  `json:"log_snippet"`
	ExpectedCause  string  `json:"expected_cause"`
	PredictedCause string  `json:"predicted_cause"`
	IsCorrect      bool    `json:"is_correct"`
	Confidence     float64 `json:"confidence"`
	LatencyMs      int64   `json:"latency_ms"`
	TokensUsed     int     `json:"tokens_used"`
	KnowledgeUsed  int     `json:"knowledge_used"`
	Strategy       string  `json:"strategy"`
	Mode           string  `json:"mode"`
	Error          string  `json:"error,omitempty"`
}

// ExperimentRunResponse 实验运行响应
type ExperimentRunResponse struct {
	Results    []CaseResult                      `json:"results"`
	Summary    map[string]interface{}            `json:"summary"`
	ByStrategy map[string]map[string]interface{} `json:"by_strategy,omitempty"`
}

// ListTestCases 返回测试用例列表（供前端选择）
func (h *ExperimentHandler) ListTestCases(c *gin.Context) {
	cases, err := experiment.LoadTestCases("testdata/cases.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载测试用例失败: " + err.Error()})
		return
	}
	// 只返回摘要信息，不返回完整日志
	type CaseSummary struct {
		ID         string `json:"id"`
		Category   string `json:"category"`
		LogSnippet string `json:"log_snippet"`
	}
	var summaries []CaseSummary
	for _, tc := range cases {
		snippet := tc.Log
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		summaries = append(summaries, CaseSummary{
			ID:         tc.ID,
			Category:   tc.Category,
			LogSnippet: snippet,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"items": summaries,
		"total": len(summaries),
	})
}

// Run 执行实验
func (h *ExperimentHandler) Run(c *gin.Context) {
	var req ExperimentRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	// 加载测试用例
	cases, err := experiment.LoadTestCases("testdata/cases.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载测试用例失败: " + err.Error()})
		return
	}

	// 限制用例数
	if req.Limit > 0 && req.Limit < len(cases) {
		cases = cases[:req.Limit]
	}

	// 确定策略列表
	var strategies []prompt.PromptStrategy
	switch strings.ToUpper(req.Strategy) {
	case "ZERO_SHOT":
		strategies = []prompt.PromptStrategy{prompt.ZeroShot}
	case "FEW_SHOT":
		strategies = []prompt.PromptStrategy{prompt.FewShot}
	case "COT":
		strategies = []prompt.PromptStrategy{prompt.CoT}
	case "ALL", "":
		strategies = []prompt.PromptStrategy{prompt.ZeroShot, prompt.FewShot, prompt.CoT}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的策略: " + req.Strategy})
		return
	}

	// 创建实验运行器
	proc := preprocessor.NewLogPreprocessor()
	var ret *retriever.KnowledgeRetriever
	if database.DB != nil {
		ret = retriever.NewKnowledgeRetriever(database.DB)
	}
	asm := prompt.NewPromptAssembler()

	mode := "DIRECT"
	if req.UseRAG {
		mode = "RAG"
	}

	var allResults []CaseResult
	totalTests := len(cases) * len(strategies)
	completed := 0

	log.Printf("[实验] 开始运行: 模式=%s, 策略=%v, 用例数=%d, 总测试=%d",
		mode, strategies, len(cases), totalTests)

	for _, tc := range cases {
		for _, strategy := range strategies {
			completed++
			cr := CaseResult{
				CaseID:        tc.ID,
				Category:      tc.Category,
				ExpectedCause: tc.ExpectedRootCause,
				Strategy:      string(strategy),
				Mode:          mode,
			}

			snippet := tc.Log
			if len(snippet) > 120 {
				snippet = snippet[:120] + "..."
			}
			cr.LogSnippet = snippet

			startTime := time.Now()

			// 步骤1：预处理
			logCtx := proc.Process(&model.RawLogInput{
				Content:    tc.Log,
				SourceType: "PASTE",
			})

			// 步骤2：知识检索
			var knowledgeItems []model.KnowledgeItem
			if req.UseRAG && ret != nil {
				items, err := ret.Retrieve(logCtx, 5, 0.3)
				if err != nil {
					log.Printf("  [检索失败] %v", err)
				} else {
					knowledgeItems = items
				}
			}
			cr.KnowledgeUsed = len(knowledgeItems)

			// 步骤3：提示组装
			promptObj := asm.Assemble(logCtx, knowledgeItems, strategy)

			// 步骤4：LLM调用
			llmResp, err := h.llmAdapter.Invoke(promptObj, "deepseek-chat", 0.1, 2048)
			if err != nil {
				cr.Error = "LLM调用失败: " + err.Error()
				cr.LatencyMs = time.Since(startTime).Milliseconds()
				allResults = append(allResults, cr)
				log.Printf("  [%d/%d] %s %s 失败: %v", completed, totalTests, tc.ID, strategy, err)
				continue
			}

			// 解析结果
			if v, ok := llmResp.ParsedJSON["root_cause"].(string); ok {
				cr.PredictedCause = v
			}
			if v, ok := llmResp.ParsedJSON["confidence"].(float64); ok {
				cr.Confidence = v
			}
			cr.TokensUsed = llmResp.TotalTokens
			cr.LatencyMs = time.Since(startTime).Milliseconds()

			// 评估正确性
			cr.IsCorrect = evaluateCorrectness(cr.PredictedCause, tc.ExpectedRootCause, tc.ExpectedKeywords)

			allResults = append(allResults, cr)

			status := "✗"
			if cr.IsCorrect {
				status = "✓"
			}
			log.Printf("  [%d/%d] %s %s %s conf=%.2f%% lat=%dms tok=%d",
				completed, totalTests, tc.ID, strategy, status, cr.Confidence*100, cr.LatencyMs, cr.TokensUsed)

			// 避免API限流
			time.Sleep(300 * time.Millisecond)
		}
	}

	// 计算汇总
	summary := calcSummary(allResults)
	byStrategy := calcByStrategy(allResults)

	log.Printf("[实验] 完成: 准确率=%.1f%%, 正确=%v/%v, 平均耗时=%vms",
		summary["accuracy"], summary["correct"], summary["total"], summary["avg_latency_ms"])

	c.JSON(http.StatusOK, ExperimentRunResponse{
		Results:    allResults,
		Summary:    summary,
		ByStrategy: byStrategy,
	})
}

func evaluateCorrectness(predicted, expected string, keywords []string) bool {
	predictedLower := strings.ToLower(predicted)
	expectedLower := strings.ToLower(expected)

	matchCount := 0
	for _, kw := range keywords {
		if strings.Contains(predictedLower, strings.ToLower(kw)) {
			matchCount++
		}
	}
	if matchCount >= len(keywords)/2 {
		return true
	}

	expectedWords := strings.Fields(expectedLower)
	for _, word := range expectedWords {
		if len(word) > 3 && strings.Contains(predictedLower, word) {
			return true
		}
	}
	return false
}

func calcSummary(results []CaseResult) map[string]interface{} {
	total := 0
	correct := 0
	var totalLatency int64 = 0
	var totalConfidence float64 = 0
	var totalTokens int = 0
	var totalKnowledge int = 0

	for _, r := range results {
		if r.Error != "" {
			continue
		}
		total++
		if r.IsCorrect {
			correct++
		}
		totalLatency += r.LatencyMs
		totalConfidence += r.Confidence
		totalTokens += r.TokensUsed
		totalKnowledge += r.KnowledgeUsed
	}

	accuracy := 0.0
	avgLatency := int64(0)
	avgConfidence := 0.0
	avgTokens := 0
	avgKnowledge := 0.0

	if total > 0 {
		accuracy = float64(correct) / float64(total) * 100
		avgLatency = totalLatency / int64(total)
		avgConfidence = totalConfidence / float64(total)
		avgTokens = totalTokens / total
		avgKnowledge = float64(totalKnowledge) / float64(total)
	}

	return map[string]interface{}{
		"total":          total,
		"correct":        correct,
		"accuracy":       accuracy,
		"avg_latency_ms": avgLatency,
		"avg_confidence": avgConfidence,
		"avg_tokens":     avgTokens,
		"avg_knowledge":  avgKnowledge,
	}
}

func calcByStrategy(results []CaseResult) map[string]map[string]interface{} {
	strategyMap := make(map[string][]CaseResult)
	for _, r := range results {
		strategyMap[r.Strategy] = append(strategyMap[r.Strategy], r)
	}
	stats := make(map[string]map[string]interface{})
	for s, rs := range strategyMap {
		stats[s] = calcSummary(rs)
	}
	return stats
}
