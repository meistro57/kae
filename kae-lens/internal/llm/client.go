package llm

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// Client wraps the OpenAI-compatible client pointed at OpenRouter.
type Client struct {
	inner            *openai.Client
	embeddingClient  *openai.Client
	reasoningModel   string
	fastModel        string
	embeddingModel   string
}

// Config holds LLM + embedding configuration.
type Config struct {
	OpenRouterAPIKey  string
	OpenRouterBaseURL string
	ReasoningModel    string
	FastModel         string
	OpenAIAPIKey      string
	EmbeddingModel    string
}

// New creates a new LLM client.
func New(cfg Config) *Client {
	// OpenRouter client (chat completions)
	orConfig := openai.DefaultConfig(cfg.OpenRouterAPIKey)
	orConfig.BaseURL = cfg.OpenRouterBaseURL
	orClient := openai.NewClientWithConfig(orConfig)

	// OpenAI client (embeddings)
	oaiClient := openai.NewClient(cfg.OpenAIAPIKey)

	return &Client{
		inner:           orClient,
		embeddingClient: oaiClient,
		reasoningModel:  cfg.ReasoningModel,
		fastModel:       cfg.FastModel,
		embeddingModel:  cfg.EmbeddingModel,
	}
}

// ChatResponse is the result of a chat completion call.
type ChatResponse struct {
	Content string
	Model   string
	Tokens  int
}

// Reason calls the reasoning model (DeepSeek R1) for deep analysis.
func (c *Client) Reason(ctx context.Context, systemPrompt, userPrompt string) (*ChatResponse, error) {
	return c.chat(ctx, c.reasoningModel, systemPrompt, userPrompt)
}

// FastChat calls the fast model (Gemini Flash) for lighter tasks.
func (c *Client) FastChat(ctx context.Context, systemPrompt, userPrompt string) (*ChatResponse, error) {
	return c.chat(ctx, c.fastModel, systemPrompt, userPrompt)
}

func (c *Client) chat(ctx context.Context, model, systemPrompt, userPrompt string) (*ChatResponse, error) {
	resp, err := c.inner.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature: 0.3, // lower = more deterministic reasoning
	})
	if err != nil {
		return nil, fmt.Errorf("chat completion with model %q: %w", model, err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from model %q", model)
	}

	return &ChatResponse{
		Content: resp.Choices[0].Message.Content,
		Model:   resp.Model,
		Tokens:  resp.Usage.TotalTokens,
	}, nil
}

// Embed generates a vector embedding for the given text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.embeddingClient.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Model: openai.EmbeddingModel(c.embeddingModel),
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("embedding text: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}

	floats := make([]float32, len(resp.Data[0].Embedding))
	for i, f := range resp.Data[0].Embedding {
		floats[i] = float32(f)
	}
	return floats, nil
}

// EmbedBatch generates embeddings for multiple texts in one API call.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := c.embeddingClient.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Model: openai.EmbeddingModel(c.embeddingModel),
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("batch embedding %d texts: %w", len(texts), err)
	}

	results := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		floats := make([]float32, len(d.Embedding))
		for j, f := range d.Embedding {
			floats[j] = float32(f)
		}
		results[i] = floats
	}
	return results, nil
}
