package nim

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http: &http.Client{
			Timeout: 0, // Streaming requests are bounded by context, not a short client timeout.
			Transport: &http.Transport{
				ResponseHeaderTimeout: 90 * time.Second,
				IdleConnTimeout:       90 * time.Second,
			},
		},
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func (c *Client) Complete(ctx context.Context, messages []Message, onDelta func(string)) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model, Messages: messages, Stream: true, Temperature: 0.2, MaxTokens: 8192,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("NIM request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("NIM returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		var decoded chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			return "", fmt.Errorf("decode NIM response (%s): %w", contentType, err)
		}
		if len(decoded.Choices) == 0 {
			return "", fmt.Errorf("NIM response contained no choices")
		}
		if onDelta != nil {
			onDelta(decoded.Choices[0].Message.Content)
		}
		return decoded.Choices[0].Message.Content, nil
	}

	var result strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return result.String(), fmt.Errorf("decode NIM stream chunk: %w", err)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		result.WriteString(delta)
		if onDelta != nil {
			onDelta(delta)
		}
	}
	if err := scanner.Err(); err != nil {
		return result.String(), fmt.Errorf("read NIM stream: %w", err)
	}
	if result.Len() == 0 {
		return "", fmt.Errorf("NIM stream completed without text")
	}
	return result.String(), nil
}
