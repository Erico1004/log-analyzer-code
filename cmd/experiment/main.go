// 实验入口程序
// 系统测试与实验分析
// 运行方式：go run cmd/experiment/main.go
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
	// 命令行参数
	testCasesPath := flag.String("cases", "testdata/cases.json", "测试案例JSON文件路径")
	ragOutput := flag.String("rag-output", "results/experiment_rag_results.csv", "RAG实验结果输出文件")
	directOutput := flag.String("direct-output", "results/experiment_direct_results.csv", "直接LLM实验结果输出文件")
	skipRAG := flag.Bool("skip-rag", false, "跳过RAG实验")
	skipDirect := flag.Bool("skip-direct", false, "跳过直接LLM实验")
	flag.Parse()

	log.Println("╔══════════════════════════════════════════════════════════════╗")
	log.Println("║           运维日志分析系统 - 实验运行器                       ║")
	log.Println("║           基于大语言模型的RAG检索增强生成                      ║")
	log.Println("╚══════════════════════════════════════════════════════════════╝")

	// 步骤1：加载配置（对应论文3.3节：可行性分析）
	log.Println("\n[1/5] 加载系统配置...")
	if err := config.LoadConfig(); err != nil {
		log.Fatal("❌ 加载配置失败:", err)
	}
	log.Println("✅ 配置加载成功")

	// 步骤2：初始化数据库（对应论文4.3节：数据库设计）
	log.Println("\n[2/5] 初始化数据库连接...")
	if err := database.InitDB(); err != nil {
		log.Printf("⚠️ 数据库连接失败: %v（将使用纯LLM模式）", err)
	} else {
		log.Println("✅ 数据库连接成功")
	}

	// 步骤3：初始化LLM适配器（对应论文4.2.4节：大模型调用模块）
	log.Println("\n[3/5] 初始化大模型适配器...")
	if config.AppConfig.DeepSeekAPIKey == "" {
		log.Fatal("❌ 未设置 DEEPSEEK_API_KEY 环境变量")
	}
	llmAdapter := llm.NewDeepSeekAdapter(config.AppConfig.DeepSeekAPIKey)
	log.Println("✅ DeepSeek适配器初始化成功")

	// 步骤4：加载测试案例（对应论文6.1节：实验数据来源）
	log.Println("\n[4/5] 加载测试案例...")
	cases, err := experiment.LoadTestCases(*testCasesPath)
	if err != nil {
		log.Fatal("❌ 加载测试案例失败:", err)
	}
	log.Printf("✅ 成功加载 %d 个测试案例，覆盖 %d 类典型故障场景",
		len(cases), countCategories(cases))

	// 步骤5：运行实验（对应论文6.4-6.5节）
	log.Println("\n[5/5] 开始运行实验...")

	// 提示策略列表（对应论文2.5.2节：主要提示策略）
	strategies := []prompt.PromptStrategy{
		prompt.ZeroShot, // 零样本提示
		prompt.FewShot,  // 少样本提示
		prompt.CoT,      // 思维链提示
	}

	var ragResults, directResults []experiment.ExperimentResult

	// 实验1：RAG增强模式
	if !*skipRAG {
		log.Println("\n┌──────────────────────────────────────────────────────────────┐")
		log.Println("│ 实验1：RAG增强模式（所提方法）                               │")
		log.Println("└──────────────────────────────────────────────────────────────┘")
		ragRunner := experiment.NewExperimentRunner(llmAdapter, true)
		ragResults, err = ragRunner.RunExperiment(cases, strategies)
		if err != nil {
			log.Printf("⚠️ RAG实验出错: %v", err)
		} else {
			log.Printf("✅ RAG实验完成，共运行 %d 次测试", len(ragResults))
			experiment.ExportToCSV(ragResults, *ragOutput)
			experiment.ExportToJSON(ragResults, "results/experiment_rag_results.json")
			log.Printf("✅ 结果已导出至 %s 和 results/experiment_rag_results.json", *ragOutput)
		}
	}

	// 实验2：直接LLM模式（基线方法）
	if !*skipDirect {
		log.Println("\n┌──────────────────────────────────────────────────────────────┐")
		log.Println("│ 实验2：直接LLM模式（基线方法）                               │")
		log.Println("└──────────────────────────────────────────────────────────────┘")
		directRunner := experiment.NewExperimentRunner(llmAdapter, false)
		directResults, err = directRunner.RunExperiment(cases, strategies)
		if err != nil {
			log.Printf("⚠️ 直接LLM实验出错: %v", err)
		} else {
			log.Printf("✅ 直接LLM实验完成，共运行 %d 次测试", len(directResults))
			experiment.ExportToCSV(directResults, *directOutput)
			experiment.ExportToJSON(directResults, "results/experiment_direct_results.json")
			log.Printf("✅ 结果已导出至 %s 和 results/experiment_direct_results.json", *directOutput)
		}
	}

	// 打印实验报告
	if len(ragResults) > 0 && len(directResults) > 0 {
		experiment.PrintReport(ragResults, directResults)
	}

	log.Println("\n实验完成！")
}

// countCategories 统计故障分类数量
func countCategories(cases []experiment.TestCase) int {
	categories := make(map[string]bool)
	for _, c := range cases {
		categories[c.Category] = true
	}
	return len(categories)
}
