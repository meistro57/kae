package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenRouterKey string
	Model         string // deep thinking model (R1)
	FastModel     string // cheap bulk model
	QdrantURL     string
	MaxCycles     int
	Seed          string
}

func Load() (*Config, error) {
	_ = godotenv.Load() // load .env if present; ignore error if missing
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not set")
	}
	return &Config{
		OpenRouterKey: key,
		Model:         "deepseek/deepseek-r1",
		FastModel:     "google/gemini-2.5-flash",
		QdrantURL:     envOr("QDRANT_URL", "http://localhost:6333"),
	}, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
