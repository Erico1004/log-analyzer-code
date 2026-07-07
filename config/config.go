package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DeepSeekAPIKey string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
}

var AppConfig *Config

func LoadConfig() error {
	_ = godotenv.Load()

	AppConfig = &Config{
		DeepSeekAPIKey: getEnv("DEEPSEEK_API_KEY", ""),
		DBHost:         getEnv("DB_HOST", "127.0.0.1"),
		DBPort:         getEnv("DB_PORT", "3306"),
		DBUser:         getEnv("DB_USER", "root"),
		DBPassword:     getEnv("DB_PASSWORD", ""),
		DBName:         getEnv("DB_NAME", "log_analysis"),
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
