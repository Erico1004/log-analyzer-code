package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/llm"
)

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	if err := database.InitDB(); err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	if config.AppConfig.EmbeddingAPIKey == "" {
		log.Fatal("EMBEDDING_API_KEY 未配置")
	}

	embedder := llm.NewEmbeddingAdapter(config.AppConfig.EmbeddingAPIKey, config.AppConfig.EmbeddingModel)
	repo := database.NewKnowledgeRepo()

	items, err := repo.FindWithoutEmbedding()
	if err != nil {
		log.Fatal("查询无 embedding 条目失败:", err)
	}

	if len(items) == 0 {
		fmt.Println("✅ 所有条目都已有 embedding，无需回填")
		return
	}

	fmt.Printf("发现 %d 条需要回填 embedding 的条目\n", len(items))

	success := 0
	failed := 0
	for i, item := range items {
		embedText := item.Title + "\n" + item.Content
		fmt.Printf("[%d/%d] 处理 ID=%d: %s...", i+1, len(items), item.ID, truncate(item.Title, 30))

		embedding, err := embedder.Embed(embedText)
		if err != nil {
			fmt.Printf(" ❌ 失败: %v\n", err)
			failed++
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if err := repo.UpdateEmbedding(item.ID, embedding); err != nil {
			fmt.Printf(" ❌ 保存失败: %v\n", err)
			failed++
		} else {
			fmt.Printf(" ✅ 成功 (维度: %d)\n", len(embedding))
			success++
		}

		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("回填完成: 成功 %d, 失败 %d\n", success, failed)
	fmt.Printf("========================================\n")

	if failed > 0 {
		os.Exit(1)
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}