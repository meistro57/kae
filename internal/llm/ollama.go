package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const ollamaDefaultURL = "http://localhost:11434"

type ollamaClient struct {
	baseURL string
	model   string
	http    *http.Client
}

// NewOllamaClient returns a Provider backed by a local Ollama instance.
// It reads OLLAMA_URL from the environment, defaulting to localhost:11434.
func NewOllamaClient(model string) Provider {
	base := os.Getenv("OLLAMA_URL")
	if base == "" {
		base = ollamaDefaultURL
	}
	return &ollamaClient{baseURL: base, model: model, http: &http.Client{}}
}

// NewOllamaClientWithURL returns a Provider backed by Ollama at a custom URL.
func NewOllamaClientWithURL(baseURL, model string) Provider {
	return &ollamaClient{baseURL: baseURL, model: model, http: &http.Client{}}
}

func (c *ollamaClient) ModelName() string { return "ollama:" + c.model }

func (c *ollamaClient) Stream(system string, messages []Message) <-chan Chunk {
	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		c.doStream(system, messages, ch)
	}()
	return ch
}

func (c *ollamaClient) doStream(system string, msgs []Message, ch chan<- Chunk) {
	all := make([]Message, 0, len(msgs)+1)
	if system != "" {
		all = append(all, Message{Role: "system", Content: system})
	}
	all = append(all, msgs...)

	reqBody := map[string]any{
		"model":    c.model,
		"messages": all,
		"stream":   true,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		ch <- Chunk{Type: ChunkError, Err: fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, b)}
		return
	}

	// Ollama streams NDJSON — one complete JSON object per line.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		if event.Message.Content != "" {
			ch <- Chunk{Type: ChunkText, Text: event.Message.Content}
		}

		if event.Done {
			ch <- Chunk{Type: ChunkDone}
			return
		}
	}
	ch <- Chunk{Type: ChunkDone}
}
