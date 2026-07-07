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

	// AutoMigrate：自动建表/更新表结构，demo可复现
	if err := DB.AutoMigrate(
		&model.KnowledgeBase{},
		&model.DiagnosisResult{},
		&model.DiagnosisHistory{},
		&model.UserFeedback{},
	); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	log.Println("✅ 数据库连接成功，表结构已同步")
	return nil
}
