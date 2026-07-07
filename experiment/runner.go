package experiment

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	CaseID         string  `json:"case_id"`         // 案例ID
	Category       string  `json:"category"`        // 故障分类
	Mode           string  `json:"mode"`            // RAG 或 DIRECT
	Strategy       string  `json:"strategy"`        // ZERO_SHOT/FEW_SHOT/COT
	PredictedCause string  `json:"predicted_cause"` // 预测根因
	ExpectedCause  string  `json:"expected_cause"`  // 期望根因
	IsCorrect      bool    `json:"is_correct"`      // 是否正确
	Confidence     float64 `json:"confidence"`      // 置信度
	LatencyMs      int64   `json:"latency_ms"`      // 响应时间(ms)
	KnowledgeUsed  int     `json:"knowledge_used"`  // 使用的知识条目数
	TokensUsed     int     `json:"tokens_used"`     // 消耗的Token数
}

// ExperimentRunner 实验运行器
// 对应论文6.3节：系统整体性能测试
type ExperimentRunner struct {
	preprocessor *preprocessor.LogPreprocessor
	retriever    *retriever.KnowledgeRetriever
	assembler    *prompt.PromptAssembler
	llmAdapter   *llm.DeepSeekAdapter
	useRAG       bool // true=RAG模式，false=直接LLM模式
}

// NewExperimentRunner 创建实验运行器实例
// useRAG: true表示使用RAG检索增强，false表示直接调用LLM（基线方法）
func NewExperimentRunner(llmAdapter *llm.DeepSeekAdapter, useRAG bool) *ExperimentRunner {
	return &ExperimentRunner{
		preprocessor: preprocessor.NewLogPreprocessor(),
		retriever:    retriever.NewKnowledgeRetriever(database.DB),
		assembler:    prompt.NewPromptAssembler(),
		llmAdapter:   llmAdapter,
		useRAG:       useRAG,
	}
}

// LoadTestCases 加载测试案例
// 从JSON文件读取25个真实运维故障案例
func LoadTestCases(path string) ([]TestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取测试案例文件失败: %w", err)
	}

	var cases []TestCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("解析测试案例JSON失败: %w", err)
	}

	return cases, nil
}

// RunSingleCase 运行单个案例
// 对应论文4.4节：系统流程设计
func (r *ExperimentRunner) RunSingleCase(tc TestCase, strategy prompt.PromptStrategy) (*ExperimentResult, error) {
	startTime := time.Now()

	// 步骤1：日志预处理（对应论文4.2.1节）
	logCtx := r.preprocessor.Process(&model.RawLogInput{
		Content:    tc.Log,
		SourceType: "PASTE",
	})

	// 步骤2：知识检索（对应论文4.2.2节）
	var knowledgeItems []model.KnowledgeItem
	if r.useRAG && r.retriever != nil {
		items, err := r.retriever.Retrieve(logCtx, 5, 0.3)
		if err != nil {
			log.Printf("  ⚠️ 知识检索失败: %v", err)
		} else {
			knowledgeItems = items
		}
	}

	// 步骤3：提示组装（对应论文4.2.3节）
	promptObj := r.assembler.Assemble(logCtx, knowledgeItems, strategy)

	// 步骤4：大模型调用（对应论文4.2.4节）
	llmResp, err := r.llmAdapter.Invoke(promptObj, "deepseek-chat", 0.1, 2048)
	if err != nil {
		return nil, fmt.Errorf("LLM调用失败: %w", err)
	}

	// 提取诊断结果
	predictedCause := ""
	if v, ok := llmResp.ParsedJSON["root_cause"].(string); ok {
		predictedCause = v
	}

	confidence := 0.0
	if v, ok := llmResp.ParsedJSON["confidence"].(float64); ok {
		confidence = v
	}

	// 评估正确性（对应论文6.2节：评价指标设计）
	isCorrect := evaluateCorrectness(predictedCause, tc.ExpectedRootCause, tc.ExpectedKeywords)

	mode := "DIRECT"
	if r.useRAG {
		mode = "RAG"
	}

	latency := time.Since(startTime).Milliseconds()

	return &ExperimentResult{
		CaseID:         tc.ID,
		Category:       tc.Category,
		Mode:           mode,
		Strategy:       string(strategy),
		PredictedCause: predictedCause,
		ExpectedCause:  tc.ExpectedRootCause,
		IsCorrect:      isCorrect,
		Confidence:     confidence,
		LatencyMs:      latency,
		KnowledgeUsed:  len(knowledgeItems),
		TokensUsed:     llmResp.TotalTokens,
	}, nil
}

// evaluateCorrectness 评估诊断是否正确
// 采用关键词匹配+语义相似度双重判断
func evaluateCorrectness(predicted, expected string, keywords []string) bool {
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

// RunExperiment 运行完整实验
// 对应论文6.4节：对比实验分析
func (r *ExperimentRunner) RunExperiment(cases []TestCase, strategies []prompt.PromptStrategy) ([]ExperimentResult, error) {
	var allResults []ExperimentResult

	modeName := "直接LLM模式"
	if r.useRAG {
		modeName = "RAG增强模式"
	}

	log.Printf("========================================")
	log.Printf("开始实验：%s", modeName)
	log.Printf("测试案例数：%d，策略数：%d", len(cases), len(strategies))
	log.Printf("========================================")

	totalTests := len(cases) * len(strategies)
	completed := 0

	for i, tc := range cases {
		for _, strategy := range strategies {
			completed++
			log.Printf("[%d/%d] 案例 %s (%s)，策略 %s",
				completed, totalTests, tc.ID, tc.Category, strategy)

			result, err := r.RunSingleCase(tc, strategy)
			if err != nil {
				log.Printf("  ❌ 失败: %v", err)
				continue
			}

			allResults = append(allResults, *result)

			status := "❌"
			if result.IsCorrect {
				status = "✅"
			}
			log.Printf("  %s 正确，置信度: %.2f%%，耗时: %dms，知识条目: %d",
				status, result.Confidence*100, result.LatencyMs, result.KnowledgeUsed)

			// 避免API限流
			if i < len(cases)-1 || strategy != strategies[len(strategies)-1] {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	return allResults, nil
}

// CalculateAccuracy 计算准确率
// 对应论文6.2节：评价指标设计
func CalculateAccuracy(results []ExperimentResult) map[string]interface{} {
	total := 0
	correct := 0
	var totalLatency int64 = 0
	var totalConfidence float64 = 0
	var totalTokens int = 0

	for _, r := range results {
		total++
		if r.IsCorrect {
			correct++
		}
		totalLatency += r.LatencyMs
		totalConfidence += r.Confidence
		totalTokens += r.TokensUsed
	}

	accuracy := 0.0
	avgLatency := int64(0)
	avgConfidence := 0.0
	avgTokens := 0

	if total > 0 {
		accuracy = float64(correct) / float64(total) * 100
		avgLatency = totalLatency / int64(total)
		avgConfidence = totalConfidence / float64(total)
		avgTokens = totalTokens / total
	}

	return map[string]interface{}{
		"total":          total,
		"correct":        correct,
		"accuracy":       accuracy,
		"avg_latency_ms": avgLatency,
		"avg_confidence": avgConfidence,
		"avg_tokens":     avgTokens,
	}
}

// GroupByStrategy 按策略分组统计
// 对应论文6.5节：提示策略实验分析
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

// GroupByCategory 按故障分类分组统计
// 用于分析不同故障类型的诊断效果
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

// PrintReport 打印实验报告
// 对应论文6.4-6.5节的实验结果展示
func PrintReport(ragResults, directResults []ExperimentResult) {
	ragStats := CalculateAccuracy(ragResults)
	directStats := CalculateAccuracy(directResults)

	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    实验报告                                   ║")
	fmt.Println("║        基于大语言模型的运维日志分析系统                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	// 整体对比
	fmt.Println("\n【6.4 对比实验分析：RAG vs 直接LLM】")
	fmt.Println("┌─────────────────┬──────────┬──────────┬────────────┬────────────┐")
	fmt.Println("│ 方法            │ 准确率   │ 正确/总数 │ 平均响应时间│ 平均置信度 │")
	fmt.Println("├─────────────────┼──────────┼──────────┼────────────┼────────────┤")
	fmt.Printf("│ RAG增强模式     │ %.2f%%   │ %d/%d     │ %dms       │ %.2f%%     │\n",
		ragStats["accuracy"],
		ragStats["correct"], ragStats["total"],
		ragStats["avg_latency_ms"],
		ragStats["avg_confidence"].(float64)*100)
	fmt.Printf("│ 直接LLM模式     │ %.2f%%   │ %d/%d     │ %dms       │ %.2f%%     │\n",
		directStats["accuracy"],
		directStats["correct"], directStats["total"],
		directStats["avg_latency_ms"],
		directStats["avg_confidence"].(float64)*100)
	fmt.Println("├─────────────────┼──────────┼──────────┼────────────┼────────────┤")
	improvement := ragStats["accuracy"].(float64) - directStats["accuracy"].(float64)
	fmt.Printf("│ 提升幅度        │ +%.2f%%  │          │ -%dms      │            │\n",
		improvement,
		directStats["avg_latency_ms"].(int64)-ragStats["avg_latency_ms"].(int64))
	fmt.Println("└─────────────────┴──────────┴──────────┴────────────┴────────────┘")

	// 按策略分组（RAG模式）
	fmt.Println("\n【6.5 提示策略实验分析 - RAG增强模式】")
	ragStrategyStats := GroupByStrategy(ragResults)
	fmt.Println("┌─────────────────┬──────────┬──────────┬────────────┐")
	fmt.Println("│ 提示策略        │ 准确率   │ 正确/总数 │ 平均响应时间│")
	fmt.Println("├─────────────────┼──────────┼──────────┼────────────┤")
	for _, strategy := range []string{"ZERO_SHOT", "FEW_SHOT", "COT"} {
		if stat, ok := ragStrategyStats[strategy]; ok {
			fmt.Printf("│ %-15s │ %.2f%%   │ %d/%d     │ %dms       │\n",
				strategy,
				stat["accuracy"],
				stat["correct"], stat["total"],
				stat["avg_latency_ms"])
		}
	}
	fmt.Println("└─────────────────┴──────────┴──────────┴────────────┘")

	// 按策略分组（直接LLM模式）
	fmt.Println("\n【6.5 提示策略实验分析 - 直接LLM模式】")
	directStrategyStats := GroupByStrategy(directResults)
	fmt.Println("┌─────────────────┬──────────┬──────────┬────────────┐")
	fmt.Println("│ 提示策略        │ 准确率   │ 正确/总数 │ 平均响应时间│")
	fmt.Println("├─────────────────┼──────────┼──────────┼────────────┤")
	for _, strategy := range []string{"ZERO_SHOT", "FEW_SHOT", "COT"} {
		if stat, ok := directStrategyStats[strategy]; ok {
			fmt.Printf("│ %-15s │ %.2f%%   │ %d/%d     │ %dms       │\n",
				strategy,
				stat["accuracy"],
				stat["correct"], stat["total"],
				stat["avg_latency_ms"])
		}
	}
	fmt.Println("└─────────────────┴──────────┴──────────┴────────────┘")

	// 结论
	fmt.Println("\n【实验结论】")
	fmt.Printf("1. RAG检索增强生成使诊断准确率从 %.2f%% 提升至 %.2f%%，提升了 %.2f 个百分点。\n",
		directStats["accuracy"], ragStats["accuracy"], improvement)
	fmt.Printf("2. 平均响应时间从 %dms 降至 %dms，缩短了 %dms。\n",
		directStats["avg_latency_ms"], ragStats["avg_latency_ms"],
		directStats["avg_latency_ms"].(int64)-ragStats["avg_latency_ms"].(int64))
	// 动态找出最优策略
	bestStrategy := "ZERO_SHOT"
	bestAccuracy := 0.0
	for _, s := range []string{"ZERO_SHOT", "FEW_SHOT", "COT"} {
		if stat, ok := ragStrategyStats[s]; ok {
			if acc := stat["accuracy"].(float64); acc > bestAccuracy {
				bestAccuracy = acc
				bestStrategy = s
			}
		}
	}
	fmt.Printf("3. %s 策略在本任务中表现最优，准确率 %.2f%%。\n", bestStrategy, bestAccuracy)
	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    实验报告结束                               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
}
