package internal

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port     string
	ProxyURL string
}

var Cfg *Config

func LoadConfig() {
	godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "7990"
	}

	Cfg = &Config{
		Port:     port,
		ProxyURL: os.Getenv("PROXY_URL"),
	}
}
