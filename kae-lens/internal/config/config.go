package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// LensConfig is the top-level config for KAE Lens.
type LensConfig struct {
	Qdrant    QdrantConfig    `yaml:"qdrant"`
	Watcher   WatcherConfig   `yaml:"watcher"`
	Density   DensityConfig   `yaml:"density"`
	LLM       LLMConfig       `yaml:"llm"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Web       WebConfig       `yaml:"web"`
	TUI       TUIConfig       `yaml:"tui"`
}

type QdrantConfig struct {
	Host                string `yaml:"host"`
	Port                int    `yaml:"port"`
	APIKey              string `yaml:"api_key"`
	KnowledgeCollection string `yaml:"knowledge_collection"`
	FindingsCollection  string `yaml:"findings_collection"`
}

type WatcherConfig struct {
	PollIntervalSeconds      int `yaml:"poll_interval_seconds"`
	BatchSize                int `yaml:"batch_size"`
	MaxConcurrentBatches     int `yaml:"max_concurrent_batches"`
	IdlePollsBeforeReprocess int `yaml:"idle_polls_before_reprocess"`
}

type DensityThresholds struct {
	VerySparseWidth int `yaml:"very_sparse_width"`
	SparseWidth     int `yaml:"sparse_width"`
	MediumWidth     int `yaml:"medium_width"`
	DenseWidth      int `yaml:"dense_width"`
	VeryDenseWidth  int `yaml:"very_dense_width"`
}

type DensityScoreThresholds struct {
	Sparse float32 `yaml:"sparse"`
	Dense  float32 `yaml:"dense"`
}

type DensityBuckets struct {
	VerySparseMax int `yaml:"very_sparse_max"`
	SparseMax     int `yaml:"sparse_max"`
	MediumMax     int `yaml:"medium_max"`
	DenseMax      int `yaml:"dense_max"`
}

type DensityConfig struct {
	Thresholds      DensityThresholds      `yaml:"thresholds"`
	ScoreThresholds DensityScoreThresholds `yaml:"score_thresholds"`
	DensityBuckets  DensityBuckets         `yaml:"density_buckets"`
}

type LLMConfig struct {
	ReasoningModel     string  `yaml:"reasoning_model"`
	FastModel          string  `yaml:"fast_model"`
	OpenRouterBaseURL  string  `yaml:"openrouter_base_url"`
	OpenRouterAPIKey   string  `yaml:"openrouter_api_key"`
	MinConfidence      float64 `yaml:"min_confidence"`
	FastBatchThreshold int     `yaml:"fast_batch_threshold"`
	LLMTimeoutSeconds  int     `yaml:"llm_timeout_seconds"`
}

type EmbeddingConfig struct {
	Model        string `yaml:"model"`
	Dimensions   int    `yaml:"dimensions"`
	OpenAIAPIKey string `yaml:"openai_api_key"`
}

type WebConfig struct {
	Port       int    `yaml:"port"`
	SSEPath    string `yaml:"sse_path"`
	CORSOrigin string `yaml:"cors_origin"`
}

type TUIConfig struct {
	Enabled      bool `yaml:"enabled"`
	RefreshMs    int  `yaml:"refresh_ms"`
	MaxFeedItems int  `yaml:"max_feed_items"`
}

// Load reads a lens config from the given YAML file path.
// Environment variables override yaml values for sensitive fields:
//
//	LENS_QDRANT_API_KEY, LENS_OPENROUTER_API_KEY, LENS_OPENAI_API_KEY
func Load(path string) (*LensConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg LensConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Environment variable overrides for secrets.
	// LENS_* variants take priority; fall back to the shared KAE env vars.
	if v := os.Getenv("LENS_QDRANT_HOST"); v != "" {
		cfg.Qdrant.Host = v
	}
	if v := os.Getenv("LENS_QDRANT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Qdrant.Port = p
		}
	}
	if v := os.Getenv("LENS_QDRANT_API_KEY"); v != "" {
		cfg.Qdrant.APIKey = v
	}
	if v := firstEnv("LENS_OPENROUTER_API_KEY", "OPENROUTER_API_KEY"); v != "" {
		cfg.LLM.OpenRouterAPIKey = v
	}
	if v := firstEnv("LENS_OPENAI_API_KEY", "OPENAI_API_KEY"); v != "" {
		cfg.Embedding.OpenAIAPIKey = v
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// firstEnv returns the value of the first non-empty environment variable.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func (c *LensConfig) validate() error {
	if c.Qdrant.Host == "" {
		return fmt.Errorf("qdrant.host is required")
	}
	if c.Qdrant.Port == 0 {
		return fmt.Errorf("qdrant.port is required")
	}
	if c.Watcher.BatchSize <= 0 {
		return fmt.Errorf("watcher.batch_size must be > 0")
	}
	if c.Watcher.IdlePollsBeforeReprocess <= 0 {
		c.Watcher.IdlePollsBeforeReprocess = 3 // default: reprocess after 3 idle polls
	}
	if c.LLM.MinConfidence <= 0 || c.LLM.MinConfidence > 1 {
		return fmt.Errorf("llm.min_confidence must be between 0 and 1")
	}
	return nil
}
