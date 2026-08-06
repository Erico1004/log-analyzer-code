package main

import (
	"flag"
	"log"

	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/experiment"
	"log-analyzer/llm"
	"log-analyzer/prompt"
)

func main() {
	testCasesPath := flag.String("cases", "testdata/cases.json", "测试案例JSON文件路径")
	ragOutput := flag.String("rag-output", "results/experiment_rag_results.csv", "RAG实验结果输出文件")
	directOutput := flag.String("direct-output", "results/experiment_direct_results.csv", "直接LLM实验结果输出文件")
	skipRAG := flag.Bool("skip-rag", false, "跳过RAG实验")
	skipDirect := flag.Bool("skip-direct", false, "跳过直接LLM实验")
	flag.Parse()

	if err := config.LoadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	if err := database.InitDB(); err != nil {
		log.Printf("数据库连接失败: %v（将使用纯LLM模式）", err)
	}

	if config.AppConfig.DeepSeekAPIKey == "" {
		log.Fatal("未设置 DEEPSEEK_API_KEY 环境变量")
	}
	llmAdapter := llm.NewDeepSeekAdapter(config.AppConfig.DeepSeekAPIKey)

	cases, err := experiment.LoadTestCases(*testCasesPath)
	if err != nil {
		log.Fatal("加载测试案例失败:", err)
	}
	log.Printf("加载 %d 个测试案例, %d 类故障", len(cases), countCategories(cases))

	strategies := []prompt.PromptStrategy{
		prompt.ZeroShot,
		prompt.FewShot,
		prompt.CoT,
	}

	var ragResults, directResults []experiment.ExperimentResult

	if !*skipRAG {
		log.Println("--- RAG 增强模式实验 ---")
		ragRunner := experiment.NewExperimentRunner(llmAdapter, true)
		ragResults, err = ragRunner.RunExperiment(cases, strategies)
		if err != nil {
			log.Printf("RAG实验出错: %v", err)
		} else {
			log.Printf("RAG实验完成, 共 %d 次测试", len(ragResults))
			experiment.ExportToCSV(ragResults, *ragOutput)
			experiment.ExportToJSON(ragResults, "results/experiment_rag_results.json")
		}
	}

	if !*skipDirect {
		log.Println("--- 直接LLM模式实验 ---")
		directRunner := experiment.NewExperimentRunner(llmAdapter, false)
		directResults, err = directRunner.RunExperiment(cases, strategies)
		if err != nil {
			log.Printf("直接LLM实验出错: %v", err)
		} else {
			log.Printf("直接LLM实验完成, 共 %d 次测试", len(directResults))
			experiment.ExportToCSV(directResults, *directOutput)
			experiment.ExportToJSON(directResults, "results/experiment_direct_results.json")
		}
	}

	if len(ragResults) > 0 && len(directResults) > 0 {
		experiment.PrintReport(ragResults, directResults)
	}

	log.Println("实验完成")
}

func countCategories(cases []experiment.TestCase) int {
	categories := make(map[string]bool)
	for _, c := range cases {
		categories[c.Category] = true
	}
	return len(categories)
}