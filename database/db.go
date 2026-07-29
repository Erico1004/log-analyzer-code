package database

import (
	"fmt"
	"log"
	"log-analyzer/config"
	"log-analyzer/model"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	cfg := config.AppConfig

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	sqlDB, _ := DB.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// AutoMigrate：自动建表/更新表结构
	if err := DB.AutoMigrate(
		&model.KnowledgeBase{},
		&model.DiagnosisHistory{},
		&model.UserFeedback{},
	); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 创建 FULLTEXT 索引（幂等：检查是否已存在）
	var indexCount int64
	DB.Raw(`SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE table_schema = DATABASE()
			AND table_name = 'knowledge_base'
			AND index_type = 'FULLTEXT'`).Scan(&indexCount)
	if indexCount == 0 {
		if err := DB.Exec(`ALTER TABLE knowledge_base ADD FULLTEXT INDEX ft_content_keywords (content, keywords)`).Error; err != nil {
			log.Printf("⚠️ FULLTEXT 索引创建失败: %v", err)
		} else {
			log.Println("✅ FULLTEXT 索引创建成功")
		}
	}

	log.Println("✅ 数据库连接成功，表结构已同步")
	return nil
}
