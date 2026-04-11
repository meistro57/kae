package llm

import "net/http"

const openAIURL = "https://api.openai.com/v1/chat/completions"

// openAIClient calls the native OpenAI API.
// The wire format is identical to OpenRouter so it reuses streamOpenAICompat.
type openAIClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewOpenAIClient returns a Provider backed by the native OpenAI API.
func NewOpenAIClient(apiKey, model string) Provider {
	return &openAIClient{apiKey: apiKey, model: model, http: &http.Client{}}
}

func (c *openAIClient) ModelName() string { return c.model }

func (c *openAIClient) Stream(system string, messages []Message) <-chan Chunk {
	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		streamOpenAICompat(openAIURL, c.apiKey, c.model, system, messages, ch, nil)
	}()
	return ch
}
