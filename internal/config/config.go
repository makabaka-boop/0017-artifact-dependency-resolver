package config

import (
	"os"
)

// Config 保存来自环境变量的服务配置。
type Config struct {
	ListenAddr string
	DBPath     string
	LogLevel   string
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		ListenAddr: getenvDefault("LISTEN_ADDR", ":8080"),
		DBPath:     getenvDefault("DB_PATH", "data.db"),
		LogLevel:   getenvDefault("LOG_LEVEL", "info"),
	}
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
