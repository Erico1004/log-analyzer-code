package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// 初始化LLM适配器
	llmAdapter := llm.NewDeepSeekAdapter(config.AppConfig.DeepSeekAPIKey)

	// 初始化处理器
	diagnoseHandler := handler.NewDiagnoseHandler(llmAdapter)
	feedbackHandler := handler.NewFeedbackHandler()
	knowledgeHandler := handler.NewKnowledgeHandler()
	historyHandler := handler.NewHistoryHandler()
	healthHandler := handler.NewHealthHandler()
	experimentHandler := handler.NewExperimentHandler(llmAdapter)

	// 创建Gin引擎
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	// 首页
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	// 健康检查（无需认证）
	r.GET("/health", healthHandler.Liveness)
	r.GET("/ready", healthHandler.Readiness)

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

		// 实验评估
		api.GET("/experiment/cases", experimentHandler.ListTestCases)
		api.POST("/experiment/run", experimentHandler.Run)
	}

	port := config.AppConfig.Port

	log.Println("========================================")
	log.Println("🚀 运维日志智能分析系统启动成功")
	log.Printf("   访问地址: http://localhost:%s", port)
	log.Println("========================================")

	// 优雅退出
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("收到退出信号，开始优雅关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️ 强制关闭: %v", err)
	}

	// 关闭数据库连接
	if sqlDB, err := database.DB.DB(); err == nil {
		sqlDB.Close()
	}

	log.Println("✅ 服务已关闭")
}
