package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/handler"
	"log-analyzer/llm"
)

func main() {
	// 加载配置
	if err := config.LoadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 初始化数据库
	if err := database.InitDB(); err != nil {
		log.Printf("⚠️ 数据库连接失败: %v", err)
	}

	// 检查API Key
	if config.AppConfig.DeepSeekAPIKey == "" {
		log.Println("⚠️ 警告: 未设置 DEEPSEEK_API_KEY")
	}

	// 初始化LLM适配器
	llmAdapter := llm.NewDeepSeekAdapter(config.AppConfig.DeepSeekAPIKey)

	// 初始化处理器
	diagnoseHandler := handler.NewDiagnoseHandler(llmAdapter)
	feedbackHandler := handler.NewFeedbackHandler()
	knowledgeHandler := handler.NewKnowledgeHandler()
	historyHandler := handler.NewHistoryHandler()

	// 创建Gin引擎
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	// 首页
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	// API路由
	api := r.Group("/api")
	{
		api.POST("/diagnose", diagnoseHandler.Handle)
		api.POST("/feedback", feedbackHandler.Handle)

		// 知识库管理
		kb := api.Group("/knowledge")
		{
			kb.GET("", knowledgeHandler.List)
			kb.GET("/:id", knowledgeHandler.Get)
			kb.POST("", knowledgeHandler.Create)
			kb.PUT("/:id", knowledgeHandler.Update)
			kb.DELETE("/:id", knowledgeHandler.Delete)
		}

		// 诊断历史
		api.GET("/history", historyHandler.List)
		api.GET("/stats", historyHandler.Stats)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("========================================")
	log.Println("🚀 运维日志智能分析系统启动成功")
	log.Printf("   访问地址: http://localhost:%s", port)
	log.Println("========================================")

	r.Run(":" + port)
}
