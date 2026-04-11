package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

type anthropicClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewAnthropicClient returns a Provider backed by the native Anthropic API.
func NewAnthropicClient(apiKey, model string) Provider {
	return &anthropicClient{apiKey: apiKey, model: model, http: &http.Client{}}
}

func (c *anthropicClient) ModelName() string { return c.model }

func (c *anthropicClient) Stream(system string, messages []Message) <-chan Chunk {
	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		c.doStream(system, messages, ch)
	}()
	return ch
}

func (c *anthropicClient) doStream(system string, msgs []Message, ch chan<- Chunk) {
	// Build Anthropic-format messages (system goes in top-level field, not messages).
	type antMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	antMsgs := make([]antMsg, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		antMsgs = append(antMsgs, antMsg{Role: m.Role, Content: m.Content})
	}

	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": 16000,
		"stream":     true,
		"messages":   antMsgs,
	}
	if system != "" {
		reqBody["system"] = system
	}
	// Enable adaptive thinking for modern Claude 4.x models; budget-based
	// thinking for older models that support it.
	if thinking := anthropicThinkingConfig(c.model); thinking != nil {
		reqBody["thinking"] = thinking
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", anthropicURL, bytes.NewReader(body))
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		ch <- Chunk{Type: ChunkError, Err: fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, b)}
		return
	}

	// Anthropic SSE: parse data: lines, inspect .type and .delta.type
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				if event.Delta.Text != "" {
					ch <- Chunk{Type: ChunkText, Text: event.Delta.Text}
				}
			case "thinking_delta":
				if event.Delta.Thinking != "" {
					ch <- Chunk{Type: ChunkThink, Text: event.Delta.Thinking}
				}
			}
		case "message_stop":
			ch <- Chunk{Type: ChunkDone}
			return
		}
	}
	ch <- Chunk{Type: ChunkDone}
}

// anthropicThinkingConfig returns the thinking config for a given model,
// or nil if thinking is not applicable.
func anthropicThinkingConfig(model string) map[string]any {
	m := strings.ToLower(model)
	// claude-opus-4-6 and claude-sonnet-4-6 support adaptive thinking.
	if strings.Contains(m, "claude-opus-4") || strings.Contains(m, "claude-sonnet-4-6") {
		return map[string]any{"type": "adaptive"}
	}
	// Older models with budget-based thinking.
	if strings.Contains(m, "claude-3-7") || strings.Contains(m, "claude-3-5-sonnet") {
		return map[string]any{"type": "enabled", "budget_tokens": 8000}
	}
	return nil
}
