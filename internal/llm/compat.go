package llm

// streamOpenAICompat streams from any OpenAI-compatible chat/completions
// endpoint (OpenRouter, OpenAI, Azure, etc.).
// extraHeaders is optional and is merged into the request.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func streamOpenAICompat(
	baseURL, apiKey, model, system string,
	msgs []Message,
	ch chan<- Chunk,
	extraHeaders map[string]string,
) {
	all := make([]Message, 0, len(msgs)+1)
	if system != "" {
		all = append(all, Message{Role: "system", Content: system})
	}
	all = append(all, msgs...)

	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": all,
		"stream":   true,
	})

	req, err := http.NewRequest("POST", baseURL, bytes.NewReader(body))
	if err != nil {
		ch <- Chunk{Type: ChunkError, Err: err}
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := (&http.Client{}).Do(req)
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

		// DeepSeek R1 / some models send reasoning in the `reasoning` field.
		if r := event.Choices[0].Delta.Reasoning; r != "" {
			ch <- Chunk{Type: ChunkThink, Text: r}
		}

		text := event.Choices[0].Delta.Content
		if text == "" {
			continue
		}

		// Walk text handling inline <think>…</think> tags (fallback for models
		// that embed thinking markers in the content field).
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
