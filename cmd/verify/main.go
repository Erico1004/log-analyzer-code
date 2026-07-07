// 检索流水线验证脚本
// 仅测试知识检索（不调用LLM），验证修改效果
// 运行：go run cmd/verify/main.go
package main

import (
	"fmt"
	"log"
	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/model"
	"log-analyzer/preprocessor"
	"log-analyzer/retriever"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║         检索流水线验证                                ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")

	// 1. 加载配置
	fmt.Println("\n[1] 加载配置...")
	if err := config.LoadConfig(); err != nil {
		log.Fatal("❌ 配置加载失败:", err)
	}
	fmt.Println("✅ 配置加载成功")

	// 2. 初始化数据库（含AutoMigrate）
	fmt.Println("\n[2] 初始化数据库（含AutoMigrate）...")
	if err := database.InitDB(); err != nil {
		log.Fatal("❌ 数据库初始化失败:", err)
	}
	fmt.Println("✅ 数据库连接成功，表结构已同步")

	// 3. 统计知识库数据量
	fmt.Println("\n[3] 检查知识库数据...")
	var count int64
	database.DB.Model(&model.KnowledgeBase{}).Count(&count)
	fmt.Printf("📊 knowledge_base 表中记录数: %d\n", count)
	if count == 0 {
		fmt.Println("⚠️  知识库为空！需要先导入种子数据")
		return
	}

	// 4. 用典型案例测试检索
	fmt.Println("\n[4] 检索测试...")
	testCases := []struct {
		name string
		log  string
	}{
		{"内存溢出", "2024-01-15 10:23:45 ERROR [main-thread] java.lang.OutOfMemoryError: Java heap space\n\tat com.example.app.parse(App.java:42)\n\tat com.example.app.Main.main(Main.java:15)"},
		{"数据库连接", "2024-01-16 14:30:12 ERROR [pool-1-thread-3] com.zaxxer.hikari.HikariPool$PoolEntry - Communications link failure\ncom.mysql.cj.jdbc.exceptions.CommunicationsException: Communications link failure"},
		{"磁盘满", "2024-01-17 08:15:33 ERROR [file-writer] java.io.IOException: No space left on device\n\tat java.io.FileOutputStream.write(Native Method)"},
	}

	p := preprocessor.NewLogPreprocessor()
	r := retriever.NewKnowledgeRetriever(database.DB)

	allPassed := true
	for _, tc := range testCases {
		fmt.Printf("\n--- 测试: %s ---\n", tc.name)
		fmt.Printf("输入日志片段:\n  %s\n", tc.log[:min(80, len(tc.log))]+"...")

		// 预处理
		logCtx := p.Process(&model.RawLogInput{
			Content:    tc.log,
			SourceType: "PASTE",
		})
		fmt.Printf("提取关键词: %v\n", logCtx.KeyErrors)

		// 检索（使用修改后的归一化逻辑）
		items, err := r.Retrieve(logCtx, 5, 0.3)
		if err != nil {
			fmt.Printf("❌ 检索失败: %v\n", err)
			allPassed = false
			continue
		}

		if len(items) == 0 {
			fmt.Println("⚠️  检索到 0 条（可能是知识库中没有匹配的内容）")
			allPassed = false
		} else {
			fmt.Printf("✅ 检索到 %d 条:\n", len(items))
			for i, item := range items {
				fmt.Printf("  %d. [%.4f] %s (分类: %s)\n", i+1, item.SimilarityScore, item.Title, item.Category)
				if i >= 2 {
					break // 只展示前3条
				}
			}
		}
	}

	// 5. 总结
	fmt.Println("\n╔══════════════════════════════════════════════════════╗")
	if allPassed {
		fmt.Println("║  ✅ 全部验证通过！                                   ║")
	} else {
		fmt.Println("║  ⚠️  部分测试未检索到结果                            ║")
	}
	fmt.Println("╚══════════════════════════════════════════════════════╝")
}
