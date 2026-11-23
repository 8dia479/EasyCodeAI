package nim

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sort"
	"strings"
	"time"
)

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolSpec struct {
	Type     string           `json:"type"`
	Function ToolFunctionSpec `json:"function"`
}

type ToolFunctionSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
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
		http: &http.Client{Timeout: 15 * time.Minute, Transport: &http.Transport{
			ResponseHeaderTimeout: 90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   4,
		}},
	}
}

type request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Tools       []ToolSpec `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
}

type response struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

type chunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

const maxRequestAttempts = 4

func (c *Client) Complete(ctx context.Context, messages []Message, tools []ToolSpec, onDelta func(string)) (Message, error) {
	body, err := json.Marshal(request{c.model, messages, true, 0.2, 8192, tools, "auto"})
	if err != nil {
		return Message{}, err
	}
	if len(body) > 16<<20 {
		return Message{}, fmt.Errorf("NIM request exceeds the 16 MiB safety limit")
	}

	var resp *http.Response
	for attempt := 0; attempt < maxRequestAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return Message{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		resp, err = c.http.Do(req)
		if err != nil {
			return Message{}, fmt.Errorf("NIM request failed: %w", err)
		}
		if !isRetryableStatus(resp.StatusCode) || attempt == maxRequestAttempts-1 {
			break
		}

		delay, retryAllowed := retryDelay(resp.Header.Get("Retry-After"), attempt)
		if !retryAllowed {
			break
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		case <-time.After(delay):
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return Message{}, fmt.Errorf("NIM HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		data, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
		if err != nil {
			return Message{}, err
		}
		if len(data) > 32<<20 {
			return Message{}, fmt.Errorf("NIM response exceeds the 32 MiB safety limit")
		}
		var value response
		if err := json.Unmarshal(data, &value); err != nil {
			return Message{}, err
		}
		if len(value.Choices) == 0 {
			return Message{}, fmt.Errorf("empty NIM response")
		}
		msg := value.Choices[0].Message
		if msg.Role == "" {
			msg.Role = "assistant"
		}
		if msg.Content != "" && onDelta != nil {
			onDelta(msg.Content)
		}
		return msg, nil
	}

	var out Message
	calls := map[int]*ToolCall{}
	limited := &io.LimitedReader{R: resp.Body, N: (32 << 20) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 65536), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var value chunk
		if err := json.Unmarshal([]byte(payload), &value); err != nil {
			return out, err
		}
		if len(value.Choices) == 0 {
			continue
		}
		delta := value.Choices[0].Delta
		if delta.Content != "" {
			out.Content += delta.Content
			if onDelta != nil {
				onDelta(delta.Content)
			}
		}
		for _, part := range delta.ToolCalls {
			call := calls[part.Index]
			if call == nil {
				call = &ToolCall{Type: "function"}
				calls[part.Index] = call
			}
			if part.ID != "" {
				call.ID = part.ID
			}
			if part.Type != "" {
				call.Type = part.Type
			}
			call.Function.Name += part.Function.Name
			call.Function.Arguments += part.Function.Arguments
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	if limited.N == 0 {
		return out, fmt.Errorf("NIM stream exceeds the 32 MiB safety limit")
	}
	indices := make([]int, 0, len(calls))
	for index := range calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	if len(indices) > 64 {
		return out, fmt.Errorf("NIM response contains more than 64 tool calls")
	}
	for _, index := range indices {
		out.ToolCalls = append(out.ToolCalls, *calls[index])
	}
	out.Role = "assistant"
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return out, fmt.Errorf("empty NIM stream")
	}
	return out, nil
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryDelay(retryAfter string, attempt int) (time.Duration, bool) {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64); err == nil && seconds >= 0 {
		if seconds > 120 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(retryAfter); err == nil {
		delay := time.Until(when)
		if delay < 0 {
			delay = 0
		}
		if delay > 120*time.Second {
			return 0, false
		}
		return delay, true
	}
	delay := time.Duration(1<<attempt) * time.Second
	if delay > 8*time.Second {
		delay = 8 * time.Second
	}
	return delay, true
}
