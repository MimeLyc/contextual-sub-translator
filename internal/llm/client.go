package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client represents a generic LLM API client
// Provides methods for chat completions and file handling
// Thread-safe for concurrent use
//
// config: Configuration for the LLM API
// httpClient: HTTP client for API requests
// baseURL: Base URL for the LLM API
type Client struct {
	config     *Config
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new LLM client with the given configuration
//
// config: Configuration for the LLM API
//
// Returns a new Client instance or an error if configuration is invalid
// Example:
//
//	config, err := llm.NewLLMConfig()
//	if err != nil {
//		log.Fatal(err)
//	}
//	client, err := llm.NewClient(config)
//	if err != nil {
//		log.Fatal(err)
//	}
func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	client := &Client{
		config:  config,
		baseURL: config.APIURL,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}

	return client, nil
}

// ChatCompletion creates a chat completion request to the configured LLM API
//
// ctx: Context for the request
// messages: Array of messages in the conversation
// options: Optional configuration for the request
//
// # Returns the chat completion response or an error
//
// Example:
//
//	messages := []llm.Message{
//		{Role: "user", Content: "Hello, how are you?"},
//	}
//	response, err := client.ChatCompletion(ctx, messages, nil)
func (c *Client) ChatCompletion(ctx context.Context, messages []Message, opts *ChatCompletionOptions) (*ChatResponse, error) {
	if opts == nil {
		opts = NewChatCompletionOptions()
	}

	// Add system prompt if provided
	if opts.SystemPrompt != "" {
		systemMessage := Message{
			Role:    "system",
			Content: opts.SystemPrompt,
		}
		messages = append([]Message{systemMessage}, messages...)
	}

	// Add file content to messages if provided
	if len(opts.Files) > 0 {
		fileMessages := c.processFiles(opts.Files)
		messages = append(messages, fileMessages...)
	}

	request := ChatRequest{
		Model:       c.getModel(opts),
		Messages:    messages,
		MaxTokens:   c.getMaxTokens(opts),
		Temperature: c.getTemperature(opts),
		Stream:      opts.Stream,
	}

	response, err := c.makeRequest(ctx, "POST", "/chat/completions", request)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	return response, nil
}

// SimpleChat provides a simple interface for chat completion
//
// ctx: Context for the request
// prompt: The user prompt
// systemPrompt: Optional system prompt for context
//
// # Returns the assistant's response content or an error
//
// Example:
//
//	response, err := client.SimpleChat(ctx, "What is Go?", "You are a helpful assistant.")
func (c *Client) SimpleChat(ctx context.Context, prompt string, systemPrompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	opts := NewChatCompletionOptions()
	if systemPrompt != "" {
		opts = opts.WithSystemPrompt(systemPrompt)
	}

	response, err := c.ChatCompletion(ctx, messages, opts)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return response.Choices[0].Message.Content, nil
}

// ChatWithFiles provides a chat interface with file attachments
//
// ctx: Context for the request
// prompt: The user prompt
// files: Files to attach to the conversation
// systemPrompt: Optional system prompt for context
//
// # Returns the assistant's response content or an error
//
// Example:
//
//	file, err := llm.NewFileFromPath("document.txt")
//	if err != nil {
//		log.Fatal(err)
//	}
//	response, err := client.ChatWithFiles(ctx, "Summarize this document", []llm.File{*file}, "You are a helpful assistant.")
func (c *Client) ChatWithFiles(ctx context.Context, prompt string, files []File, systemPrompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: prompt, FilePaths: []string{}},
	}

	for _, file := range files {
		if file.Content != nil {
			fileContent, err := file.ToMessage()
			if err != nil {
				return "", fmt.Errorf("failed to process file %s: %w", file.Name, err)
			}
			messages = append(messages, Message{
				Role:    "user",
				Content: fileContent,
			})
		}
	}

	return c.SimpleChat(ctx, prompt, systemPrompt)
}

// makeRequest makes a raw HTTP request to the configured LLM API
func (c *Client) makeRequest(ctx context.Context, method, path string, payload interface{}) (*ChatResponse, error) {
	url := c.baseURL + path

	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	headers := c.config.GetHeaders()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if os.IsTimeout(err) {
			return nil, fmt.Errorf("request timed out: %w", err)
		}
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var chatResponse ChatResponse
	if err := json.Unmarshal(responseBody, &chatResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for API errors
	if chatResponse.Error != nil && chatResponse.Error.Message != "" {
		return &chatResponse, chatResponse.Error
	}

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &chatResponse, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	return &chatResponse, nil
}

// processFiles processes file attachments and creates appropriate messages
func (c *Client) processFiles(files []File) []Message {
	messages := make([]Message, 0, len(files))

	for _, file := range files {
		fileContent, err := file.ToMessage()
		if err != nil {
			// Skip files that can't be processed
			continue
		}

		messages = append(messages, Message{
			Role:    "user",
			Content: fileContent,
		})
	}

	return messages
}

// getModel returns the model to use for the request
func (c *Client) getModel(opts *ChatCompletionOptions) string {
	return c.config.Model
}

// getMaxTokens returns the max tokens to use for the request
func (c *Client) getMaxTokens(opts *ChatCompletionOptions) int {
	if opts.MaxTokens > 0 {
		return opts.MaxTokens
	}
	return c.config.MaxTokens
}

// getTemperature returns the temperature to use for the request
func (c *Client) getTemperature(opts *ChatCompletionOptions) float64 {
	if opts.Temperature >= 0 && opts.Temperature <= 2 {
		return opts.Temperature
	}
	return c.config.Temperature
}

// GetModels returns a list of available models from the configured LLM provider
//
// ctx: Context for the request
//
// # Returns an array of model information or an error
//
// Example:
//
//	models, err := client.GetModels(ctx)
//	if err != nil {
//		log.Printf("Failed to get models: %v", err)
//	}
func (c *Client) GetModels(ctx context.Context) ([]ModelInfo, error) {
	_, err := c.makeRequest(ctx, "GET", "/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %w", err)
	}

	// This is a simplified representation for the currently configured model
	return []ModelInfo{
		{ID: c.config.Model, Name: c.config.Model},
	}, nil
}

// StreamChatCompletion provides streaming chat completion
//
// This is a placeholder for streaming implementation
// The actual implementation would require handling SSE (Server-Sent Events)
func (c *Client) StreamChatCompletion(ctx context.Context, messages []Message, opts *ChatCompletionOptions) (<-chan ChatResponse, error) {
	// Placeholder for streaming implementation
	return nil, fmt.Errorf("streaming not implemented yet")
}

// ModelInfo represents basic model information
//
// ID: Model identifier
// Name: Human-readable model name
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
