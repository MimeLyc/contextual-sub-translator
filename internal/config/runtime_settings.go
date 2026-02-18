package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
	"golang.org/x/text/language"
)

const DefaultRuntimeSettingsFile = "/app/config/settings.json"

type RuntimeSettings struct {
	LLMAPIURL      string `json:"llm_api_url"`
	LLMAPIKey      string `json:"llm_api_key"`
	LLMModel       string `json:"llm_model"`
	CronExpr       string `json:"cron_expr"`
	TargetLanguage string `json:"target_language"`
}

func RuntimeSettingsFilePath() string {
	return getEnvString("SETTINGS_FILE", DefaultRuntimeSettingsFile)
}

func (s RuntimeSettings) Validate() error {
	if strings.TrimSpace(s.LLMAPIURL) == "" {
		return fmt.Errorf("llm_api_url is required")
	}
	if strings.TrimSpace(s.LLMAPIKey) == "" {
		return fmt.Errorf("llm_api_key is required")
	}
	if strings.TrimSpace(s.LLMModel) == "" {
		return fmt.Errorf("llm_model is required")
	}
	if strings.TrimSpace(s.CronExpr) == "" {
		return fmt.Errorf("cron_expr is required")
	}
	if _, err := cron.ParseStandard(s.CronExpr); err != nil {
		return fmt.Errorf("invalid cron_expr: %w", err)
	}
	if strings.TrimSpace(s.TargetLanguage) == "" {
		return fmt.Errorf("target_language is required")
	}
	if _, err := language.Parse(s.TargetLanguage); err != nil {
		return fmt.Errorf("invalid target_language: %w", err)
	}
	return nil
}

func (c *Config) RuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		LLMAPIURL:      c.LLM.APIURL,
		LLMAPIKey:      c.LLM.APIKey,
		LLMModel:       c.LLM.Model,
		CronExpr:       c.Translate.CronExpr,
		TargetLanguage: c.Translate.TargetLanguage.String(),
	}
}

func WithRuntimeSettings(settings RuntimeSettings) Option {
	return func(c *Config) {
		if strings.TrimSpace(settings.LLMAPIURL) != "" {
			c.LLM.APIURL = settings.LLMAPIURL
		}
		if strings.TrimSpace(settings.LLMAPIKey) != "" {
			c.LLM.APIKey = settings.LLMAPIKey
		}
		if strings.TrimSpace(settings.LLMModel) != "" {
			c.LLM.Model = settings.LLMModel
		}
		if strings.TrimSpace(settings.CronExpr) != "" {
			c.Translate.CronExpr = settings.CronExpr
		}
		if tag, err := language.Parse(settings.TargetLanguage); err == nil {
			c.Translate.TargetLanguage = tag
		}
	}
}

func LoadRuntimeSettingsFile(path string) (RuntimeSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeSettings{}, err
	}
	var settings RuntimeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return RuntimeSettings{}, fmt.Errorf("invalid settings file: %w", err)
	}
	return settings, nil
}

func WriteRuntimeSettingsFile(path string, settings RuntimeSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

type RuntimeSettingsStore struct {
	path string

	mu      sync.RWMutex
	current RuntimeSettings
}

func NewRuntimeSettingsStore(path string, initial RuntimeSettings) (*RuntimeSettingsStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("settings file path is required")
	}
	if err := initial.Validate(); err != nil {
		return nil, err
	}
	return &RuntimeSettingsStore{
		path:    path,
		current: initial,
	}, nil
}

func (s *RuntimeSettingsStore) GetRuntimeSettings() (RuntimeSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current, nil
}

func (s *RuntimeSettingsStore) UpdateRuntimeSettings(next RuntimeSettings) (RuntimeSettings, error) {
	if err := next.Validate(); err != nil {
		return RuntimeSettings{}, err
	}
	if err := WriteRuntimeSettingsFile(s.path, next); err != nil {
		return RuntimeSettings{}, err
	}

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()
	return next, nil
}
