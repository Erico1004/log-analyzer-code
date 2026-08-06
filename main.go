package main

import (
	"context"
	"io"
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

type limitedReader struct {
	r     io.ReadCloser
	limit int64
	read  int64
}

func (lr *limitedReader) Read(p []byte) (n int, err error) {
	if lr.read >= lr.limit {
		return 0, ErrTooLarge
	}
	n, err = lr.r.Read(p)
	lr.read += int64(n)
	if lr.read > lr.limit {
		return 0, ErrTooLarge
	}
	return
}

func (lr *limitedReader) Close() error {
	return lr.r.Close()
}

var ErrTooLarge = &errTooLarge{}

type errTooLarge struct{}

func (e *errTooLarge) Error() string { return "请求体过大" }

func bodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = &limitedReader{c.Request.Body, maxBytes, 0}
		c.Next()
	}
}

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	if err := database.InitDB(); err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	llmAdapter := llm.NewDeepSeekAdapter(config.AppConfig.DeepSeekAPIKey)

	diagnoseHandler := handler.NewDiagnoseHandler(llmAdapter)
	feedbackHandler := handler.NewFeedbackHandler()
	knowledgeHandler := handler.NewKnowledgeHandler()
	historyHandler := handler.NewHistoryHandler()
	healthHandler := handler.NewHealthHandler()
	experimentHandler := handler.NewExperimentHandler(llmAdapter)

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	r.GET("/health", healthHandler.Liveness)
	r.GET("/ready", healthHandler.Readiness)

	api := r.Group("/api")
	{
		api.POST("/diagnose", bodyLimitMiddleware(1<<20), diagnoseHandler.Handle)
		api.POST("/feedback", feedbackHandler.Handle)

		kb := api.Group("/knowledge")
		{
			kb.GET("", knowledgeHandler.List)
			kb.GET("/:id", knowledgeHandler.Get)
			kb.POST("", knowledgeHandler.Create)
			kb.PUT("/:id", knowledgeHandler.Update)
			kb.DELETE("/:id", knowledgeHandler.Delete)
		}

		api.GET("/history", historyHandler.List)
		api.GET("/stats", historyHandler.Stats)

		api.GET("/experiment/cases", experimentHandler.ListTestCases)
		api.POST("/experiment/run", experimentHandler.Run)
	}

	port := config.AppConfig.Port

	log.Printf("服务器启动: http://localhost:%s", port)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("强制关闭: %v", err)
	}

	if database.DB != nil {
		if sqlDB, err := database.DB.DB(); err == nil {
			sqlDB.Close()
		}
	}

	log.Println("服务已停止")
}