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

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

type geminiClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewGeminiClient returns a Provider backed by the native Google Gemini API.
func NewGeminiClient(apiKey, model string) Provider {
	return &geminiClient{apiKey: apiKey, model: model, http: &http.Client{}}
}

func (c *geminiClient) ModelName() string { return c.model }

func (c *geminiClient) Stream(system string, messages []Message) <-chan Chunk {
	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		c.doStream(system, messages, ch)
	}()
	return ch
}

func (c *geminiClient) doStream(system string, msgs []Message, ch chan<- Chunk) {
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var contents []content
	for _, m := range msgs {
		if m.Role == "system" {
			continue // handled via systemInstruction
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, content{Role: role, Parts: []part{{Text: m.Content}}})
	}

	reqBody := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"maxOutputTokens": 8192,
		},
	}
	if system != "" {
		reqBody["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": system}},
		}
	}

	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s",
		geminiBaseURL, c.model, c.apiKey)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
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
		ch <- Chunk{Type: ChunkError, Err: fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, b)}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		var event struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text    string `json:"text"`
						Thought bool   `json:"thought"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil || len(event.Candidates) == 0 {
			continue
		}

		cand := event.Candidates[0]
		for _, p := range cand.Content.Parts {
			if p.Text == "" {
				continue
			}
			if p.Thought {
				ch <- Chunk{Type: ChunkThink, Text: p.Text}
			} else {
				ch <- Chunk{Type: ChunkText, Text: p.Text}
			}
		}

		switch cand.FinishReason {
		case "STOP", "MAX_TOKENS", "SAFETY":
			ch <- Chunk{Type: ChunkDone}
			return
		}
	}
	ch <- Chunk{Type: ChunkDone}
}
