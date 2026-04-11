package llm

import (
	"fmt"
	"strings"
)

// ProviderKeys holds API keys / URLs for every supported backend.
type ProviderKeys struct {
	OpenRouterKey string
	AnthropicKey  string
	OpenAIKey     string
	GeminiKey     string
	OllamaURL     string // overrides OLLAMA_URL env var when non-empty
}

// ParseProviderModel splits a "provider:model" string into its two parts.
// Strings without a colon are treated as bare model names on OpenRouter.
func ParseProviderModel(s string) (provider, model string) {
	if idx := strings.Index(s, ":"); idx >= 0 {
		return strings.ToLower(s[:idx]), s[idx+1:]
	}
	return "openrouter", s
}

// NewProvider constructs the right Provider implementation for the given
// "provider:model" string and key set.  Returns an error if the required
// API key is missing.
func NewProvider(providerModel string, keys ProviderKeys) (Provider, error) {
	prov, model := ParseProviderModel(providerModel)
	switch prov {
	case "openrouter", "or":
		if keys.OpenRouterKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY required for provider %q", prov)
		}
		return NewClient(keys.OpenRouterKey, model), nil

	case "anthropic", "ant":
		if keys.AnthropicKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY required for provider %q", prov)
		}
		return NewAnthropicClient(keys.AnthropicKey, model), nil

	case "openai":
		if keys.OpenAIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY required for provider %q", prov)
		}
		return NewOpenAIClient(keys.OpenAIKey, model), nil

	case "gemini":
		if keys.GeminiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY required for provider %q", prov)
		}
		return NewGeminiClient(keys.GeminiKey, model), nil

	case "ollama":
		if keys.OllamaURL != "" {
			return NewOllamaClientWithURL(keys.OllamaURL, model), nil
		}
		return NewOllamaClient(model), nil

	default:
		return nil, fmt.Errorf("unknown provider %q — use openrouter, anthropic, openai, gemini, or ollama", prov)
	}
}
