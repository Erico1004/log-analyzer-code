package main

import (
	"fmt"
	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/model"
)

func main() {
	config.LoadConfig()
	if err := database.InitDB(); err != nil {
		fmt.Println("DB init error:", err)
		return
	}

	var items []model.KnowledgeBase
	database.DB.Find(&items)
	fmt.Printf("知识库共 %d 条记录:\n\n", len(items))
	for _, item := range items {
		content := item.Content
		fmt.Printf("[%d] 标题: %s\n", item.ID, item.Title)
		fmt.Printf("    分类: %s\n", item.Category)
		fmt.Printf("    关键词: %s\n", item.Keywords)
		fmt.Printf("    内容: %s\n\n", content)
	}
}
