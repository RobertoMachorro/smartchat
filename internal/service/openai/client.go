package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 45 * time.Second},
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

func (c *Client) ChatCompletion(ctx context.Context, model string, messages []Message, temperature float64) (Message, Usage, error) {
	if c.BaseURL == "" {
		return Message{}, Usage{}, fmt.Errorf("missing base url")
	}
	endpoint, err := url.JoinPath(c.BaseURL, "chat/completions")
	if err != nil {
		return Message{}, Usage{}, fmt.Errorf("build endpoint: %w", err)
	}
	payload, err := json.Marshal(chatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
	})
	if err != nil {
		return Message{}, Usage{}, fmt.Errorf("marshal request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Message{}, Usage{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.HTTP.Do(request)
	if err != nil {
		return Message{}, Usage{}, fmt.Errorf("execute request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return Message{}, Usage{}, fmt.Errorf("openai request failed: status %d", response.StatusCode)
	}
	var parsed chatResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return Message{}, Usage{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Message{}, Usage{}, fmt.Errorf("no choices returned")
	}
	return parsed.Choices[0].Message, parsed.Usage, nil
}
