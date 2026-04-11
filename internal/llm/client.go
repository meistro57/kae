package llm

import "net/http"

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// Client is the legacy OpenRouter-backed provider.
// New code should call NewProvider / NewClient via the factory.
type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

func NewClient(apiKey, model string) *Client {
	return &Client{apiKey: apiKey, model: model, http: &http.Client{}}
}

func (c *Client) ModelName() string { return c.model }

// Stream satisfies the Provider interface.
func (c *Client) Stream(system string, messages []Message) <-chan Chunk {
	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		streamOpenAICompat(openRouterURL, c.apiKey, c.model, system, messages, ch, map[string]string{
			"HTTP-Referer": "https://github.com/meistro57/kae",
			"X-Title":      "Knowledge Archaeology Engine",
		})
	}()
	return ch
}
