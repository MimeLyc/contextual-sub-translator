package llm

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// Message represents a chat message
// Supports both text and file content
//
// Role: "system", "user", or "assistant"
// Content: Text content of the message
// FilePaths: Optional list of file paths to include as attachments
// FileURLs: Optional list of URLs to include as attachments
type Message struct {
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	FilePaths []string `json:"-"` // Not serialized to JSON, handled separately
	FileURLs  []string `json:"-"` // Not serialized to JSON, handled separately
}

// ChatRequest represents a chat completion request
// Compatible with OpenAI API format
//
// Model: The model to use for completion
// Messages: Array of conversation messages
// MaxTokens: Maximum number of tokens to generate
// Temperature: Sampling temperature (0-2)
// Stream: Whether to stream the response
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse represents a chat completion response
// Compatible with OpenAI API format
//
// ID: Unique identifier for the response
// Object: Always "chat.completion"
// Created: Unix timestamp
// Model: Model used for the response
// Choices: Array of completion choices
// Usage: Token usage statistics
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
	Error   *Error   `json:"error,omitempty"`
}

// Choice represents a completion choice
//
// Index: Index of the choice
// Message: The message content
// FinishReason: Reason for completion
//
// FinishReason values: "stop", "length", "content_filter", "tool_calls", "function_call"
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage statistics
//
// PromptTokens: Number of tokens in the prompt
// CompletionTokens: Number of tokens in the completion
// TotalTokens: Total number of tokens used
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Error represents an API error
//
// Message: Error message
// Type: Error type
// Param: Parameter that caused the error
// Code: Error code
type Error struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

// File represents a file attachment
//
// Name: Original file name
// ContentType: MIME type of the file
// Content: File content as bytes
// URL: Optional URL for the file
type File struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"content"`
	URL         string `json:"url,omitempty"`
}

// ChatCompletionOptions represents options for chat completion
//
// SystemPrompt: System prompt to set context
// MaxTokens: Maximum tokens for the response
// Temperature: Temperature for the response
// Files: Files to include in the message
// Stream: Whether to stream the response
type ChatCompletionOptions struct {
	SystemPrompt string
	MaxTokens    int
	Temperature  float64
	Files        []File
	Stream       bool
}

// NewChatCompletionOptions creates a new chat completion options with defaults
func NewChatCompletionOptions() *ChatCompletionOptions {
	return &ChatCompletionOptions{
		SystemPrompt: "",
		MaxTokens:    0, // Use model default
		Temperature:  0.7,
		Files:        []File{},
		Stream:       false,
	}
}

// WithSystemPrompt sets the system prompt
func (o *ChatCompletionOptions) WithSystemPrompt(prompt string) *ChatCompletionOptions {
	o.SystemPrompt = prompt
	return o
}

// WithMaxTokens sets the max tokens
func (o *ChatCompletionOptions) WithMaxTokens(maxTokens int) *ChatCompletionOptions {
	o.MaxTokens = maxTokens
	return o
}

// WithTemperature sets the temperature
func (o *ChatCompletionOptions) WithTemperature(temperature float64) *ChatCompletionOptions {
	o.Temperature = temperature
	return o
}

// WithFiles adds files to the message
func (o *ChatCompletionOptions) WithFiles(files ...File) *ChatCompletionOptions {
	o.Files = append(o.Files, files...)
	return o
}

// WithStream enables streaming response
func (o *ChatCompletionOptions) WithStream(stream bool) *ChatCompletionOptions {
	o.Stream = stream
	return o
}

// NewFileFromPath creates a new file from a file path
// Automatically detects content type based on file extension
func NewFileFromPath(filePath string) (*File, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	contentType := getContentTypeFromExtension(filePath)

	return &File{
		Name:        filepath.Base(filePath),
		ContentType: contentType,
		Content:     content,
	}, nil
}

// NewFileFromURL creates a new file from a URL
// Note: This is a placeholder for URL-based file handling
// Actual implementation would require downloading the file
func NewFileFromURL(url string) (*File, error) {
	return &File{
		Name: filepath.Base(url),
		URL:  url,
	}, nil
}

// ToMessage converts the file to a message with file content
// For text files, includes content directly
// For binary files, includes file info and content type
func (f *File) ToMessage() (string, error) {
	if f.URL != "" {
		return fmt.Sprintf("File URL: %s\nFile name: %s", f.URL, f.Name), nil
	}

	if isTextFile(f.ContentType) {
		return fmt.Sprintf("File: %s\nContent:\n%s", f.Name, string(f.Content)), nil
	}

	return fmt.Sprintf("File: %s\nType: %s\nSize: %d bytes", f.Name, f.ContentType, len(f.Content)), nil
}

// ToMultipart converts the file to a multipart form field
func (f *File) ToMultipart(writer *multipart.Writer, fieldName string) error {
	part, err := writer.CreateFormFile(fieldName, f.Name)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = part.Write(f.Content)
	return err
}

// Helper functions
func getContentTypeFromExtension(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func isTextFile(contentType string) bool {
	textTypes := []string{
		"text/plain",
		"text/markdown",
		"text/csv",
		"text/html",
		"text/css",
		"application/json",
		"application/xml",
		"application/javascript",
	}

	for _, t := range textTypes {
		if contentType == t || strings.HasPrefix(contentType, "text/") {
			return true
		}
	}
	return false
}

func (e *Error) Error() string {
	return fmt.Sprintf("LLM API Error: %s (type: %s, code: %s)", e.Message, e.Type, e.Code)
}

// MarshalJSON custom JSON marshaling for Message to handle file uploads
func (m Message) MarshalJSON() ([]byte, error) {
	// This is a simplified version - in real implementation, files would be handled differently
	// For OpenRouter, files would typically be handled as URLs or base64 encoded content
	return json.Marshal(&struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{
		Role:    m.Role,
		Content: m.Content,
	})
}
