package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// Core
	OpenRouterKey   string
	Model           string // primary thinking model
	FastModel       string // cheap bulk model
	QdrantURL       string
	MaxCycles       int
	Seed            string
	SharedMemory    bool
	ResumeGraphPath string
	SaveGraphPath   string

	// Embeddings — optional OpenAI-compatible endpoint
	EmbeddingsURL   string
	EmbeddingsKey   string
	EmbeddingsModel string

	// Additional provider keys
	AnthropicKey string
	OpenAIKey    string
	GeminiKey    string
	OllamaURL    string

	// Ensemble mode (Tier 1.1)
	EnsembleMode   bool
	EnsembleModels []string // "provider:model" strings

	// Run controller (Tier 1.2)
	NoveltyThreshold float64 // new_nodes/total below this → stagnant
	StagnationWindow int     // consecutive stagnant cycles before stop
	BranchThreshold  float64 // anomaly score above this → branch
	MaxBranches      int     // 0 = unlimited

	// Meta-analysis (Tier 1.3)
	RunAnalysis    bool
	MinAnalysisRuns int
}

func Load() (*Config, error) {
	_ = godotenv.Load() // load .env if present; ignore error if missing

	key := os.Getenv("OPENROUTER_API_KEY")
	// OpenRouter key is required unless the user plans to use other providers
	// exclusively — we still load it but will only fail at stream time.
	return &Config{
		OpenRouterKey:   key,
		AnthropicKey:    os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:       os.Getenv("OPENAI_API_KEY"),
		GeminiKey:       os.Getenv("GEMINI_API_KEY"),
		OllamaURL:       os.Getenv("OLLAMA_URL"),
		Model:           "deepseek/deepseek-r1",
		FastModel:       "google/gemini-2.5-flash",
		QdrantURL:       envOr("QDRANT_URL", "http://localhost:6333"),
		EmbeddingsURL:   os.Getenv("EMBEDDINGS_URL"),
		EmbeddingsKey:   os.Getenv("EMBEDDINGS_KEY"),
		EmbeddingsModel: envOr("EMBEDDINGS_MODEL", "text-embedding-3-small"),
		// Run controller defaults
		NoveltyThreshold: 0.05,
		StagnationWindow: 3,
		BranchThreshold:  0.7,
		MaxBranches:      4,
		// Meta-analysis defaults
		MinAnalysisRuns: 2,
	}, nil
}

// ProviderKeys returns the subset of Config needed by llm.NewProvider.
// Importing this avoids a circular dependency between config and llm.
func (c *Config) ProviderKeys() interface{} {
	// Returned as any to avoid importing llm here; callers cast to llm.ProviderKeys.
	return struct {
		OpenRouterKey string
		AnthropicKey  string
		OpenAIKey     string
		GeminiKey     string
		OllamaURL     string
	}{
		OpenRouterKey: c.OpenRouterKey,
		AnthropicKey:  c.AnthropicKey,
		OpenAIKey:     c.OpenAIKey,
		GeminiKey:     c.GeminiKey,
		OllamaURL:     c.OllamaURL,
	}
}

// Validate checks that at least one provider key is set.
func (c *Config) Validate() error {
	if c.OpenRouterKey == "" &&
		c.AnthropicKey == "" &&
		c.OpenAIKey == "" &&
		c.GeminiKey == "" &&
		!strings.Contains(c.Model, "ollama:") {
		return fmt.Errorf("no API key set — provide at least one of: " +
			"OPENROUTER_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY")
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
