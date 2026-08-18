package handler

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"log-analyzer/experiment"
	"log-analyzer/llm"
	"log-analyzer/prompt"
)

type ExperimentHandler struct {
	llmAdapter       *llm.DeepSeekAdapter
	embeddingAdapter *llm.EmbeddingAdapter
	runMu            sync.Mutex
	running          bool
}

func NewExperimentHandler(llmAdapter *llm.DeepSeekAdapter, embeddingAdapter *llm.EmbeddingAdapter) *ExperimentHandler {
	return &ExperimentHandler{llmAdapter: llmAdapter, embeddingAdapter: embeddingAdapter}
}

type ExperimentRunRequest struct {
	Strategy string `json:"strategy"`
	UseRAG   bool   `json:"use_rag"`
	Limit    int    `json:"limit"`
}

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

type ExperimentRunResponse struct {
	Results    []CaseResult                      `json:"results"`
	Summary    map[string]interface{}            `json:"summary"`
	ByStrategy map[string]map[string]interface{} `json:"by_strategy,omitempty"`
}

func (h *ExperimentHandler) ListTestCases(c *gin.Context) {
	cases, err := experiment.LoadTestCases("testdata/cases.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type caseSummary struct {
		ID         string `json:"id"`
		Category   string `json:"category"`
		LogSnippet string `json:"log_snippet"`
	}
	summaries := make([]caseSummary, 0, len(cases))
	for _, tc := range cases {
		snippet := tc.Log
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		summaries = append(summaries, caseSummary{
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

func (h *ExperimentHandler) Run(c *gin.Context) {
	h.runMu.Lock()
	if h.running {
		h.runMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "实验正在运行中，请等待完成后再试"})
		return
	}
	h.running = true
	h.runMu.Unlock()

	defer func() {
		h.runMu.Lock()
		h.running = false
		h.runMu.Unlock()
	}()

	var req ExperimentRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	cases, err := experiment.LoadTestCases("testdata/cases.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载测试用例失败: " + err.Error()})
		return
	}

	if req.Limit > 0 && req.Limit < len(cases) {
		cases = cases[:req.Limit]
	}

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

	runner := experiment.NewExperimentRunner(h.llmAdapter, req.UseRAG, h.embeddingAdapter)

	rawResults, err := runner.RunExperiment(cases, strategies)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "实验运行失败: " + err.Error()})
		return
	}

	results := make([]CaseResult, 0, len(rawResults))
	for _, r := range rawResults {
		results = append(results, CaseResult{
			CaseID:         r.CaseID,
			Category:       r.Category,
			LogSnippet:     r.LogSnippet,
			ExpectedCause:  r.ExpectedCause,
			PredictedCause: r.PredictedCause,
			IsCorrect:      r.IsCorrect,
			Confidence:     r.Confidence,
			LatencyMs:      r.LatencyMs,
			TokensUsed:     r.TokensUsed,
			KnowledgeUsed:  r.KnowledgeUsed,
			Strategy:       r.Strategy,
			Mode:           r.Mode,
			Error:          r.Error,
		})
	}

	summary := experiment.CalculateAccuracy(rawResults)
	byStrategy := experiment.GroupByStrategy(rawResults)

	c.JSON(http.StatusOK, ExperimentRunResponse{
		Results:    results,
		Summary:    summary,
		ByStrategy: byStrategy,
	})
}
