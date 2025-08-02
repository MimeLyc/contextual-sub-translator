package llm

import (
	"fmt"
)

// Config holds the configuration for LLM client
// Supports configuration via environment variables with sensible defaults
// Supports any LLM provider (OpenRouter, OpenAI, Anthropic, etc.)
//
// Environment Variables:
// - LLM_API_KEY: API key for the LLM provider (required)
// - LLM_API_URL: API endpoint URL (default: https://openrouter.ai/api/v1)
// - LLM_MODEL: Model name to use (default: openai/gpt-3.5-turbo)
// - LLM_MAX_TOKENS: Maximum tokens for responses (default: 1000)
// - LLM_TEMPERATURE: Temperature for responses (default: 0.7)
// - LLM_TIMEOUT: Request timeout in seconds (default: 30)
// - LLM_SITE_URL: Site URL for HTTP referer header (optional)
// - LLM_APP_NAME: Application name for X-Title header (optional)
type Config struct {
	APIKey      string  `json:"api_key"`
	APIURL      string  `json:"api_url"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Timeout     int     `json:"timeout"`
	SiteURL     string  `json:"site_url"`
	AppName     string  `json:"app_name"`
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	if c.APIURL == "" {
		return fmt.Errorf("API URL is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	if c.MaxTokens < 1 {
		return fmt.Errorf("max tokens must be greater than 0")
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if c.Timeout < 1 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	return nil
}

// GetHeaders returns the headers for the LLM API request
func (c *Config) GetHeaders() map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + c.APIKey,
		"Content-Type":  "application/json",
	}

	if c.SiteURL != "" {
		headers["HTTP-Referer"] = c.SiteURL
	}
	if c.AppName != "" {
		headers["X-Title"] = c.AppName
	}

	return headers
}
