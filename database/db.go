package database

import (
	"fmt"
	"log"
	"log-analyzer/config"
	"log-analyzer/model"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	cfg := config.AppConfig

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	sqlDB, _ := DB.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 启用 pgvector 扩展
	if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return fmt.Errorf("pgvector 扩展启用失败: %w", err)
	}

	// AutoMigrate：自动建表/更新表结构
	if err := DB.AutoMigrate(
		&model.KnowledgeBase{},
		&model.DiagnosisHistory{},
		&model.UserFeedback{},
	); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 创建 HNSW 向量索引（幂等）
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_kb_embedding_hnsw ON knowledge_base USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64)")

	// 创建 GIN 全文检索索引（PostgreSQL 原生 FTS）
	DB.Exec("CREATE INDEX IF NOT EXISTS idx_kb_fts ON knowledge_base USING gin (to_tsvector('simple', coalesce(content, '') || ' ' || coalesce(keywords, '')))")

	log.Println("数据库连接成功 (PostgreSQL + pgvector)，表结构已同步")
	return nil
}
