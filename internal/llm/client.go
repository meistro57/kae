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

const baseURL = "https://openrouter.ai/api/v1/chat/completions"

type ChunkType int

const (
	ChunkText  ChunkType = iota // normal assistant output
	ChunkThink                  // inside R1 <think>...</think>
	ChunkDone                   // stream finished cleanly
	ChunkError                  // something went wrong
)

type Chunk struct {
	Type ChunkType
	Text string
	Err  error
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

func NewClient(apiKey, model string) *Client {
	return &Client{apiKey: apiKey, model: model, http: &http.Client{}}
}

// Stream sends messages and returns a channel of chunks.
// Channel is closed after ChunkDone or ChunkError.
func (c *Client) Stream(system string, messages []Message) <-chan Chunk {
	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		c.doStream(system, messages, ch)
	}()
	return ch
}

func (c *Client) doStream(system string, msgs []Message, ch chan<- Chunk) {
	all := make([]Message, 0, len(msgs)+1)
	if system != "" {
		all = append(all, Message{Role: "system", Content: system})
	}
	all = append(all, msgs...)

	body, _ := json.Marshal(map[string]any{
		"model":    c.model,
		"messages": all,
		"stream":   true,
	})

	req, err := http.NewRequest("POST", baseURL, bytes.NewReader(body))
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/meistro57/kae")
	req.Header.Set("X-Title", "Knowledge Archaeology Engine")

	resp, err := c.http.Do(req)
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		ch <- Chunk{Type: ChunkError, Err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	inThink := false
	buf := &strings.Builder{}

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		t := ChunkText
		if inThink {
			t = ChunkThink
		}
		ch <- Chunk{Type: t, Text: buf.String()}
		buf.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			flush()
			ch <- Chunk{Type: ChunkDone}
			return
		}

		var event struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil || len(event.Choices) == 0 {
			continue
		}

		// R1 via OpenRouter sends thinking in the `reasoning` field
		if r := event.Choices[0].Delta.Reasoning; r != "" {
			ch <- Chunk{Type: ChunkThink, Text: r}
		}

		text := event.Choices[0].Delta.Content
		if text == "" {
			continue
		}

		// Walk text handling <think> tag boundaries (fallback for models that inline tags)
		for len(text) > 0 {
			if !inThink {
				idx := strings.Index(text, "<think>")
				if idx < 0 {
					buf.WriteString(text)
					break
				}
				buf.WriteString(text[:idx])
				flush()
				inThink = true
				text = text[idx+len("<think>"):]
			} else {
				idx := strings.Index(text, "</think>")
				if idx < 0 {
					buf.WriteString(text)
					break
				}
				buf.WriteString(text[:idx])
				flush()
				inThink = false
				text = text[idx+len("</think>"):]
			}
		}
	}

	flush()
}
