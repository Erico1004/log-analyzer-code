package experiment

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log-analyzer/database"
	"log-analyzer/llm"
	"log-analyzer/model"
	"log-analyzer/preprocessor"
	"log-analyzer/prompt"
	"log-analyzer/retriever"
)

// TestCase 测试案例结构
type TestCase struct {
	ID                string   `json:"id"`                  // 案例编号
	Category          string   `json:"category"`            // 故障分类（10类典型场景）
	Log               string   `json:"log"`                 // 日志内容
	ExpectedRootCause string   `json:"expected_root_cause"` // 期望的根因
	ExpectedKeywords  []string `json:"expected_keywords"`   // 期望的关键词（用于评估）
}

// ExperimentResult 单次实验结果
type ExperimentResult struct {
	CaseID         string  `json:"case_id"`
	Category       string  `json:"category"`
	LogSnippet     string  `json:"log_snippet"`
	Mode           string  `json:"mode"`
	Strategy       string  `json:"strategy"`
	PredictedCause string  `json:"predicted_cause"`
	ExpectedCause  string  `json:"expected_cause"`
	IsCorrect      bool    `json:"is_correct"`
	Confidence     float64 `json:"confidence"`
	LatencyMs      int64   `json:"latency_ms"`
	KnowledgeUsed  int     `json:"knowledge_used"`
	TokensUsed     int     `json:"tokens_used"`
	Error          string  `json:"error,omitempty"`
}

type ExperimentRunner struct {
	preprocessor *preprocessor.LogPreprocessor
	retriever    *retriever.KnowledgeRetriever
	assembler    *prompt.PromptAssembler
	llmAdapter   *llm.DeepSeekAdapter
	useRAG       bool
}

func NewExperimentRunner(llmAdapter *llm.DeepSeekAdapter, useRAG bool, embeddingAdapter *llm.EmbeddingAdapter) *ExperimentRunner {
	return &ExperimentRunner{
		preprocessor: preprocessor.NewLogPreprocessor(),
		retriever:    retriever.NewKnowledgeRetriever(database.DB, embeddingAdapter),
		assembler:    prompt.NewPromptAssembler(),
		llmAdapter:   llmAdapter,
		useRAG:       useRAG,
	}
}

// LoadTestCases 加载测试案例
// 从JSON文件读取25个真实运维故障案例
// 自动尝试多个候选路径以适应不同运行场景
func LoadTestCases(path string) ([]TestCase, error) {
	candidates := []string{path}
	if path != "testdata/cases.json" {
		candidates = append(candidates, "testdata/cases.json")
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "testdata", "cases.json"),
			filepath.Join(exeDir, "..", "testdata", "cases.json"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "testdata", "cases.json"),
		)
	}

	var lastErr error
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		var cases []TestCase
		if err := json.Unmarshal(data, &cases); err != nil {
			lastErr = fmt.Errorf("解析JSON失败: %w", err)
			continue
		}
		log.Printf("[LoadTestCases] 成功加载: %s (%d 个用例)", candidate, len(cases))
		return cases, nil
	}
	return nil, fmt.Errorf("加载测试案例失败（已尝试路径 %v）: %w", candidates, lastErr)
}

func (r *ExperimentRunner) RunSingleCase(tc TestCase, strategy prompt.PromptStrategy) *ExperimentResult {
	startTime := time.Now()

	snippet := tc.Log
	if len(snippet) > 120 {
		snippet = snippet[:120] + "..."
	}

	result := &ExperimentResult{
		CaseID:        tc.ID,
		Category:      tc.Category,
		LogSnippet:    snippet,
		ExpectedCause: tc.ExpectedRootCause,
		Strategy:      string(strategy),
	}

	if r.useRAG {
		result.Mode = "RAG"
	} else {
		result.Mode = "DIRECT"
	}

	logCtx := r.preprocessor.Process(&model.RawLogInput{
		Content:    tc.Log,
		SourceType: "PASTE",
	})

	var knowledgeItems []model.KnowledgeItem
	if r.useRAG && r.retriever != nil {
		items, err := r.retriever.Retrieve(logCtx, 5, 0.3)
		if err != nil {
			log.Printf("  [warn] 知识检索失败: %v", err)
		} else {
			knowledgeItems = items
		}
	}
	result.KnowledgeUsed = len(knowledgeItems)

	promptObj := r.assembler.Assemble(logCtx, knowledgeItems, strategy)

	llmResp, err := r.llmAdapter.Invoke(promptObj, "deepseek-chat", 0.1, 2048)
	if err != nil {
		result.Error = "LLM调用失败: " + err.Error()
		result.LatencyMs = time.Since(startTime).Milliseconds()
		return result
	}

	if v, ok := llmResp.ParsedJSON["root_cause"].(string); ok {
		result.PredictedCause = v
	}
	if v, ok := llmResp.ParsedJSON["confidence"].(float64); ok {
		result.Confidence = v
	}
	result.TokensUsed = llmResp.TotalTokens
	result.LatencyMs = time.Since(startTime).Milliseconds()

	result.IsCorrect = EvaluateCorrectness(result.PredictedCause, tc.ExpectedRootCause, tc.ExpectedKeywords)

	return result
}

func EvaluateCorrectness(predicted, expected string, keywords []string) bool {
	predictedLower := strings.ToLower(predicted)
	expectedLower := strings.ToLower(expected)

	// 方法1：关键词匹配
	matchCount := 0
	for _, kw := range keywords {
		if strings.Contains(predictedLower, strings.ToLower(kw)) {
			matchCount++
		}
	}

	// 如果匹配了至少一半的关键词，判定为正确
	if matchCount >= len(keywords)/2 {
		return true
	}

	// 方法2：语义相似度（简化版：检查共同单词）
	expectedWords := strings.Fields(expectedLower)
	for _, word := range expectedWords {
		if len(word) > 3 && strings.Contains(predictedLower, word) {
			return true
		}
	}

	return false
}

func (r *ExperimentRunner) RunExperiment(cases []TestCase, strategies []prompt.PromptStrategy) ([]ExperimentResult, error) {
	var allResults []ExperimentResult

	modeName := "DIRECT"
	if r.useRAG {
		modeName = "RAG"
	}

	log.Printf("[experiment] start: mode=%s, cases=%d, strategies=%d", modeName, len(cases), len(strategies))

	totalTests := len(cases) * len(strategies)
	completed := 0

	for i, tc := range cases {
		for _, strategy := range strategies {
			completed++

			result := r.RunSingleCase(tc, strategy)
			allResults = append(allResults, *result)

			status := "ok"
			if result.Error != "" {
				status = "err"
			} else if !result.IsCorrect {
				status = "fail"
			}
			log.Printf("  [%d/%d] %s %s %s conf=%.2f%% lat=%dms",
				completed, totalTests, tc.ID, strategy, status, result.Confidence*100, result.LatencyMs)

			if i < len(cases)-1 || strategy != strategies[len(strategies)-1] {
				time.Sleep(300 * time.Millisecond)
			}
		}
	}

	return allResults, nil
}

func CalculateAccuracy(results []ExperimentResult) map[string]interface{} {
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

// GroupByStrategy groups results by prompt strategy and computes accuracy per group.
func GroupByStrategy(results []ExperimentResult) map[string]map[string]interface{} {
	stats := make(map[string]map[string]interface{})

	strategies := []string{"ZERO_SHOT", "FEW_SHOT", "COT"}
	for _, strategy := range strategies {
		var strategyResults []ExperimentResult
		for _, r := range results {
			if r.Strategy == strategy {
				strategyResults = append(strategyResults, r)
			}
		}
		if len(strategyResults) > 0 {
			stats[strategy] = CalculateAccuracy(strategyResults)
		}
	}

	return stats
}

// GroupByCategory groups results by fault category.
func GroupByCategory(results []ExperimentResult) map[string]map[string]interface{} {
	stats := make(map[string]map[string]interface{})

	categoryMap := make(map[string][]ExperimentResult)
	for _, r := range results {
		categoryMap[r.Category] = append(categoryMap[r.Category], r)
	}

	for category, catResults := range categoryMap {
		stats[category] = CalculateAccuracy(catResults)
	}

	return stats
}

// ExportToCSV 导出结果到CSV文件
// 便于后续数据分析和可视化
func ExportToCSV(results []ExperimentResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建CSV文件失败: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	headers := []string{
		"case_id", "category", "mode", "strategy",
		"predicted_cause", "expected_cause", "is_correct",
		"confidence", "latency_ms", "knowledge_used", "tokens_used",
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// 写入数据
	for _, r := range results {
		row := []string{
			r.CaseID,
			r.Category,
			r.Mode,
			r.Strategy,
			r.PredictedCause,
			r.ExpectedCause,
			fmt.Sprintf("%v", r.IsCorrect),
			fmt.Sprintf("%.4f", r.Confidence),
			fmt.Sprintf("%d", r.LatencyMs),
			fmt.Sprintf("%d", r.KnowledgeUsed),
			fmt.Sprintf("%d", r.TokensUsed),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// ExportToJSON 导出结果到JSON文件
func ExportToJSON(results []ExperimentResult, filename string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func floatStat(stats map[string]interface{}, key string) float64 {
	v, ok := stats[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	}
	return 0
}

func intStat(stats map[string]interface{}, key string) int {
	v, ok := stats[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}

func int64Stat(stats map[string]interface{}, key string) int64 {
	v, ok := stats[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	}
	return 0
}

// PrintReport prints a comparison report between RAG and DIRECT modes.
func PrintReport(ragResults, directResults []ExperimentResult) {
	ragStats := CalculateAccuracy(ragResults)
	directStats := CalculateAccuracy(directResults)

	ragAcc := floatStat(ragStats, "accuracy")
	directAcc := floatStat(directStats, "accuracy")
	ragCorrect := intStat(ragStats, "correct")
	ragTotal := intStat(ragStats, "total")
	directCorrect := intStat(directStats, "correct")
	directTotal := intStat(directStats, "total")
	ragLatency := int64Stat(ragStats, "avg_latency_ms")
	directLatency := int64Stat(directStats, "avg_latency_ms")
	ragConf := floatStat(ragStats, "avg_confidence")
	directConf := floatStat(directStats, "avg_confidence")

	fmt.Println("\n========================================")
	fmt.Println("  Experiment Report")
	fmt.Println("========================================")

	fmt.Println("\n--- RAG vs DIRECT ---")
	fmt.Printf("  RAG    : accuracy=%.2f%%  correct=%d/%d  latency=%dms  confidence=%.1f%%\n",
		ragAcc, ragCorrect, ragTotal, ragLatency, ragConf*100)
	fmt.Printf("  DIRECT : accuracy=%.2f%%  correct=%d/%d  latency=%dms  confidence=%.1f%%\n",
		directAcc, directCorrect, directTotal, directLatency, directConf*100)

	improvement := ragAcc - directAcc
	fmt.Printf("  Delta  : %+.2f%% accuracy, %dms latency\n", improvement, directLatency-ragLatency)

	ragStrategyStats := GroupByStrategy(ragResults)
	directStrategyStats := GroupByStrategy(directResults)

	fmt.Println("\n--- By Strategy (RAG) ---")
	for _, strategy := range []string{"ZERO_SHOT", "FEW_SHOT", "COT"} {
		if stat, ok := ragStrategyStats[strategy]; ok {
			fmt.Printf("  %-12s  acc=%.2f%%  correct=%d/%d  latency=%dms\n",
				strategy, floatStat(stat, "accuracy"),
				intStat(stat, "correct"), intStat(stat, "total"),
				int64Stat(stat, "avg_latency_ms"))
		}
	}

	fmt.Println("\n--- By Strategy (DIRECT) ---")
	for _, strategy := range []string{"ZERO_SHOT", "FEW_SHOT", "COT"} {
		if stat, ok := directStrategyStats[strategy]; ok {
			fmt.Printf("  %-12s  acc=%.2f%%  correct=%d/%d  latency=%dms\n",
				strategy, floatStat(stat, "accuracy"),
				intStat(stat, "correct"), intStat(stat, "total"),
				int64Stat(stat, "avg_latency_ms"))
		}
	}

	bestStrategy := "ZERO_SHOT"
	bestAccuracy := 0.0
	for _, s := range []string{"ZERO_SHOT", "FEW_SHOT", "COT"} {
		if stat, ok := ragStrategyStats[s]; ok {
			if acc := floatStat(stat, "accuracy"); acc > bestAccuracy {
				bestAccuracy = acc
				bestStrategy = s
			}
		}
	}

	fmt.Printf("\n  Best strategy: %s (%.2f%%)\n", bestStrategy, bestAccuracy)
	fmt.Println("========================================")
}
