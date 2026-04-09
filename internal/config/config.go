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
	SharedMemory  bool
	// Embeddings — optional OpenAI-compatible endpoint for semantic vectors.
	// If unset, falls back to feature hashing.
	EmbeddingsURL   string
	EmbeddingsKey   string
	EmbeddingsModel string
}

func Load() (*Config, error) {
	_ = godotenv.Load() // load .env if present; ignore error if missing
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not set")
	}
	return &Config{
		OpenRouterKey:   key,
		Model:           "deepseek/deepseek-r1",
		FastModel:       "google/gemini-2.5-flash",
		QdrantURL:       envOr("QDRANT_URL", "http://localhost:6333"),
		EmbeddingsURL:   os.Getenv("EMBEDDINGS_URL"),
		EmbeddingsKey:   os.Getenv("EMBEDDINGS_KEY"),
		EmbeddingsModel: envOr("EMBEDDINGS_MODEL", "text-embedding-3-small"),
	}, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
