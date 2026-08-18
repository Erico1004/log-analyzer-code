package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DeepSeekAPIKey  string
	EmbeddingAPIKey string
	EmbeddingModel  string
	DBHost          string
	DBPort          string
	DBUser          string
	DBPassword      string
	DBName          string
	DBSSLMode       string
	Port            string
}

var AppConfig *Config

func LoadConfig() error {
	_ = godotenv.Load()

	AppConfig = &Config{
		DeepSeekAPIKey:  getEnv("DEEPSEEK_API_KEY", ""),
		EmbeddingAPIKey: getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingModel:  getEnv("EMBEDDING_MODEL", "BAAI/bge-m3"),
		DBHost:          getEnv("DB_HOST", "127.0.0.1"),
		DBPort:          getEnv("DB_PORT", "5432"),
		DBUser:          getEnv("DB_USER", "postgres"),
		DBPassword:      getEnv("DB_PASSWORD", ""),
		DBName:          getEnv("DB_NAME", "log_analysis"),
		DBSSLMode:       getEnv("DB_SSLMODE", "disable"),
		Port:            getEnv("PORT", "8080"),
	}

	// 校验必填字段
	var errs []string
	if AppConfig.DeepSeekAPIKey == "" {
		errs = append(errs, "DEEPSEEK_API_KEY 未设置")
	}
	if AppConfig.DBPassword == "" {
		errs = append(errs, "DB_PASSWORD 未设置")
	}
	if len(errs) > 0 {
		return fmt.Errorf("配置校验失败: %v", errs)
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
