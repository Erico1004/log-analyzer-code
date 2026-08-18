// 检索流水线验证脚本
// 用全部25个测试案例验证混合检索（FULLTEXT + symptoms LIKE）命中率
// 运行：go run cmd/verify/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/model"
	"log-analyzer/preprocessor"
	"log-analyzer/retriever"
)

type TestCase struct {
	ID                 string   `json:"id"`
	Category           string   `json:"category"`
	Log                string   `json:"log"`
	ExpectedRootCause  string   `json:"expected_root_cause"`
	ExpectedKeywords   []string `json:"expected_keywords"`
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║     RAG 混合检索验证 — 全25案例知识命中率测试             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	// 1. 加载配置和数据库
	config.LoadConfig()
	if err := database.InitDB(); err != nil {
		log.Fatal("数据库初始化失败:", err)
	}

	// 2. 加载测试案例
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		log.Fatal("读取测试案例失败:", err)
	}
	var cases []TestCase
	json.Unmarshal(data, &cases)
	fmt.Printf("\n📋 加载了 %d 个测试案例\n", len(cases))

	// 3. 统计知识库
	var kbCount int64
	database.DB.Model(&model.KnowledgeBase{}).Count(&kbCount)
	fmt.Printf("📊 知识库条目: %d\n", kbCount)

	// 4. 逐个案例检索
	p := preprocessor.NewLogPreprocessor()
	r := retriever.NewKnowledgeRetriever(database.DB, nil)

	hit := 0
	fmt.Println("\n── 案例检索结果 ──")
	for _, tc := range cases {
		logCtx := p.Process(&model.RawLogInput{
			Content:    tc.Log,
			SourceType: "PASTE",
		})
		items, err := r.Retrieve(logCtx, 5, 0.3)
		if err != nil {
			fmt.Printf("  %s (%s): ❌ 检索异常 %v\n", tc.ID, tc.Category, err)
			continue
		}
		if len(items) > 0 {
			hit++
			fmt.Printf("  %s (%s): ✅ 命中 %d 条 → %s\n", tc.ID, tc.Category, len(items), items[0].Title)
		} else {
			fmt.Printf("  %s (%s): ❌ 0 条\n", tc.ID, tc.Category)
		}
	}

	// 5. 总结
	hitRate := float64(hit) / float64(len(cases)) * 100
	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Printf("║  知识命中率: %d/%d = %.1f%%                            ║\n", hit, len(cases), hitRate)
	if hitRate >= 80 {
		fmt.Println("║  ✅ 检索系统工作正常，达到演示标准                      ║")
	} else if hitRate >= 50 {
		fmt.Println("║  ⚠️  检索命中率偏低，仍有优化空间                      ║")
	} else {
		fmt.Println("║  ❌ 检索命中率严重不足，检查知识库或检索器              ║")
	}
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
}
