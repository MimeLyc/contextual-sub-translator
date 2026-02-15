package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
	"golang.org/x/text/language"
)

// Config holds all application configuration
// Includes LLM configuration and media directory configuration
// Supports environment variables with sensible defaults
//
// Environment Variables:
// LLM Configuration:
// - LLM_API_KEY: API key for the LLM provider (required)
// - LLM_API_URL: API endpoint URL (default: https://openrouter.ai/api/v1)
// - LLM_MODEL: Model name to use (default: openai/gpt-3.5-turbo)
// - LLM_MAX_TOKENS: Maximum tokens for responses (default: 8000)
// - LLM_TEMPERATURE: Temperature for responses (default: 0.7)
// - LLM_TIMEOUT: Request timeout in seconds (default: 30)
// - LLM_SITE_URL: Site URL for HTTP referer header (optional)
// - LLM_APP_NAME: Application name for X-Title header (optional)
//
// Media Directory Configuration:
// - MOVIE_DIR: Movie directory (default: /movies)
// - ANIMATION_DIR: Animation directory (default: /animations)
// - TELEPLAY_DIR: Teleplay directory (default: /teleplays)
// - SHOW_DIR: Show directory (default: /shows)
// - DOCUMENTARY_DIR: Documentary directory (default: /documentaries)
//
// System Configuration:
// - PUID: User ID (default: 1000)
// - PGID: Group ID (default: 1000)
// - TZ: Timezone (default: UTC)
// - ZONE: Zone information (default: local)

type Config struct {
	// LLM Configuration
	LLM LLMConfig `json:"llm"`

	// Media Directory Configuration
	Media MediaConfig `json:"media"`

	// System Configuration
	System SystemConfig `json:"system"`

	// Translate Configuration
	Translate TranslateConfig `json:"translate"`

	// Search Configuration (for web search tool)
	Search SearchConfig `json:"search"`

	// Agent Configuration
	Agent AgentConfig `json:"agent"`
}

type TranslateConfig struct {
	TargetLanguage language.Tag `json:"target_language"`
	CronExpr       string       `json:"cron_expr"`
}

// SearchConfig holds the configuration for web search tool
type SearchConfig struct {
	APIKey string `json:"api_key"` // Tavily API key
	APIURL string `json:"api_url"` // Tavily API URL
}

// AgentConfig holds the configuration for the agent
type AgentConfig struct {
	MaxIterations     int `json:"max_iterations"`     // Max tool calling iterations
	BundleConcurrency int `json:"bundle_concurrency"` // Parallel bundle processing workers
}

// LLMConfig holds the configuration for LLM client
// Supports any LLM provider (OpenRouter, OpenAI, Anthropic, etc.)
type LLMConfig struct {
	APIKey      string  `json:"api_key"`
	APIURL      string  `json:"api_url"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Timeout     int     `json:"timeout"`
	SiteURL     string  `json:"site_url"`
	AppName     string  `json:"app_name"`
}

// MediaConfig holds the configuration for media directories
type MediaConfig struct {
	MovieDir       string `json:"movie_dir"`
	AnimationDir   string `json:"animation_dir"`
	TeleplayDir    string `json:"teleplay_dir"`
	ShowDir        string `json:"show_dir"`
	DocumentaryDir string `json:"documentary_dir"`
}

func (c MediaConfig) MediaPaths() []string {
	ret := make([]string, 0)
	if c.MovieDir != "" {
		ret = append(ret, c.MovieDir)
	}
	if c.AnimationDir != "" {
		ret = append(ret, c.AnimationDir)
	}
	if c.TeleplayDir != "" {
		ret = append(ret, c.TeleplayDir)
	}
	if c.ShowDir != "" {
		ret = append(ret, c.ShowDir)
	}
	if c.DocumentaryDir != "" {
		ret = append(ret, c.DocumentaryDir)
	}
	return ret
}

// SystemConfig holds the system configuration
type SystemConfig struct {
	PUID int    `json:"puid"`
	PGID int    `json:"pgid"`
	TZ   string `json:"tz"`
	Zone string `json:"zone"`
}

// Option is a function type for configuring Config
type Option func(*Config)

// NewFromEnv creates a new Config instance with values from environment variables and options
func NewFromEnv(opts ...Option) (*Config, error) {
	config := &Config{
		LLM: LLMConfig{
			APIKey:      getEnvString("LLM_API_KEY", ""),
			APIURL:      getEnvString("LLM_API_URL", "https://openrouter.ai/api/v1"),
			Model:       getEnvString("LLM_MODEL", "openai/gpt-3.5-turbo"),
			MaxTokens:   getEnvInt("LLM_MAX_TOKENS", 8000),
			Temperature: getEnvFloat("LLM_TEMPERATURE", 0.7),
			Timeout:     getEnvInt("LLM_TIMEOUT", 30),
			SiteURL:     getEnvString("LLM_SITE_URL", ""),
			AppName:     getEnvString("LLM_APP_NAME", ""),
		},
		Media: MediaConfig{
			MovieDir:       getEnvString("MOVIE_DIR", "/movies"),
			AnimationDir:   getEnvString("ANIMATION_DIR", "/animations"),
			TeleplayDir:    getEnvString("TELEPLAY_DIR", "/teleplays"),
			ShowDir:        getEnvString("SHOW_DIR", "/shows"),
			DocumentaryDir: getEnvString("DOCUMENTARY_DIR", "/documentaries"),
		},
		System: SystemConfig{
			PUID: getEnvInt("PUID", 1000),
			PGID: getEnvInt("PGID", 1000),
			TZ:   getEnvString("TZ", "UTC"),
			Zone: getEnvString("ZONE", "local"),
		},
		Translate: TranslateConfig{
			//TODO: get from env
			TargetLanguage: language.Chinese,
			CronExpr:       getEnvString("CRON_EXPR", "0 0 * * *"),
		},
		Search: SearchConfig{
			APIKey: getEnvString("SEARCH_API_KEY", ""),
			APIURL: getEnvString("SEARCH_API_URL", "https://api.tavily.com/search"),
		},
		Agent: AgentConfig{
			MaxIterations:     getEnvInt("AGENT_MAX_ITERATIONS", 10),
			BundleConcurrency: getEnvInt("AGENT_BUNDLE_CONCURRENCY", 1),
		},
	}

	log.Info("Config: %v", config)

	// Apply custom options
	for _, opt := range opts {
		opt(config)
	}

	// Validate required configuration
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// validate checks if all required configuration is properly set
func (c *Config) validate() error {
	if c.LLM.APIKey == "" {
		return fmt.Errorf("LLM_API_KEY is required")
	}
	return nil
}

// getEnvString gets a string value from environment variables with default
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an integer value from environment variables with default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvFloat gets a float value from environment variables with default
func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}
