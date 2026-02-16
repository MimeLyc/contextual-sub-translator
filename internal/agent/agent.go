package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/MimeLyc/agent-core-go/pkg/agent"
	coretools "github.com/MimeLyc/agent-core-go/pkg/tools"
	projecttools "github.com/MimeLyc/contextual-sub-translator/internal/tools"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

// LLMConfig configures the underlying LLM provider for the agent.
// Temperature is kept for backward compatibility.
// The current agent-core-go API path does not expose these knobs yet.
type LLMConfig struct {
	APIKey      string
	APIURL      string
	Model       string
	MaxTokens   int
	Temperature float64
	Timeout     int
}

// Validate validates the configuration.
func (c LLMConfig) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(c.APIURL) == "" {
		return fmt.Errorf("API URL is required")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if c.Timeout < 0 {
		return fmt.Errorf("timeout must be greater than or equal to 0")
	}
	if c.MaxTokens < 0 {
		return fmt.Errorf("max tokens must be greater than or equal to 0")
	}
	return nil
}

// Agent defines the interface for an agent that can execute tasks.
type Agent interface {
	// Execute runs the agent with the given request.
	Execute(ctx context.Context, req AgentRequest) (*AgentResult, error)

	// Close releases any resources held by the agent.
	Close() error
}

// LLMAgent implements the Agent interface using agent-core-go.
type LLMAgent struct {
	agent         coreagent.Agent
	maxIterations int
}

// NewLLMAgent creates a new LLM-based agent backed by agent-core-go.
func NewLLMAgent(cfg LLMConfig, registry *projecttools.Registry, maxIterations int) (*LLMAgent, error) {
	if maxIterations <= 0 {
		maxIterations = 10
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid LLM config: %w", err)
	}

	for _, field := range unsupportedConfigNotes(cfg) {
		log.Warn("LLM config field %s is currently ignored by agent-core-go adapter", field)
	}

	coreRegistry, err := toCoreRegistry(registry)
	if err != nil {
		return nil, err
	}

	timeout := 5 * time.Minute
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}

	coreCfg := coreagent.AgentConfig{
		Type: coreagent.AgentTypeAPI,
		API: &coreagent.APIConfig{
			ProviderType:  coreagent.ProviderTypeOpenAI,
			BaseURL:       normalizeBaseURL(cfg.APIURL),
			APIKey:        cfg.APIKey,
			Model:         cfg.Model,
			MaxTokens:     cfg.MaxTokens,
			Timeout:       timeout,
			MaxAttempts:   5,
			MaxIterations: maxIterations,
			MaxMessages:   50,
		},
		Registry: coreRegistry,
	}

	coreAgent, err := coreagent.NewAgent(coreCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent-core-go agent: %w", err)
	}

	return &LLMAgent{
		agent:         coreAgent,
		maxIterations: maxIterations,
	}, nil
}

// Execute runs the agent with the given request.
func (a *LLMAgent) Execute(ctx context.Context, req AgentRequest) (*AgentResult, error) {
	if a == nil || a.agent == nil {
		return nil, fmt.Errorf("agent is not initialized")
	}

	maxIterations := a.getMaxIterations(req)
	result, err := a.executeWith(a.agent, ctx, req, maxIterations)
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	return convertAgentResult(result), nil
}

func (a *LLMAgent) executeWith(agentInstance coreagent.Agent, ctx context.Context, req AgentRequest, maxIterations int) (coreagent.AgentResult, error) {
	return agentInstance.Execute(ctx, coreagent.AgentRequest{
		Task:         req.UserMessage,
		SystemPrompt: req.SystemPrompt,
		Options: coreagent.AgentOptions{
			MaxIterations: maxIterations,
		},
	})
}

func convertAgentResult(result coreagent.AgentResult) *AgentResult {
	converted := &AgentResult{
		Content:    result.Message,
		ToolCalls:  convertToolCalls(result.ToolCalls),
		Iterations: result.Usage.TotalIterations,
	}
	if strings.TrimSpace(converted.Content) == "" {
		converted.Content = result.Summary
	}
	return converted
}

// Close releases any resources held by the agent.
func (a *LLMAgent) Close() error {
	if a == nil || a.agent == nil {
		return nil
	}
	return a.agent.Close()
}

func (a *LLMAgent) getMaxIterations(req AgentRequest) int {
	if req.MaxIterations > 0 {
		return req.MaxIterations
	}
	return a.maxIterations
}

func normalizeBaseURL(baseURL string) string {
	normalized := strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if strings.HasSuffix(normalized, "/chat/completions") {
		normalized = strings.TrimSuffix(normalized, "/chat/completions")
	}
	if strings.HasSuffix(normalized, "/v1") {
		normalized = strings.TrimSuffix(normalized, "/v1")
	}
	return normalized
}

func unsupportedConfigNotes(cfg LLMConfig) []string {
	var notes []string
	if cfg.Temperature != 0 {
		notes = append(notes, "temperature")
	}
	return notes
}

func toCoreRegistry(registry *projecttools.Registry) (*coretools.Registry, error) {
	coreRegistry := coretools.NewRegistry()
	if registry == nil {
		return coreRegistry, nil
	}

	for _, name := range registry.List() {
		tool, ok := registry.Get(name)
		if !ok {
			continue
		}
		if err := coreRegistry.Register(toolAdapter{tool: tool}); err != nil {
			return nil, fmt.Errorf("register tool %q: %w", name, err)
		}
	}

	return coreRegistry, nil
}

func convertToolCalls(coreCalls []coreagent.ToolCallRecord) []ToolCallRecord {
	if len(coreCalls) == 0 {
		return nil
	}

	result := make([]ToolCallRecord, 0, len(coreCalls))
	for _, call := range coreCalls {
		arguments, err := json.Marshal(call.Input)
		if err != nil {
			arguments = []byte("{}")
		}

		result = append(result, ToolCallRecord{
			ToolName:  call.Name,
			Arguments: string(arguments),
			Result:    call.Output,
			IsError:   call.IsError,
		})
	}

	return result
}

type toolAdapter struct {
	tool projecttools.Tool
}

func (a toolAdapter) Name() string {
	return a.tool.Name()
}

func (a toolAdapter) Description() string {
	return a.tool.Description()
}

func (a toolAdapter) InputSchema() map[string]any {
	var schema map[string]any
	if err := json.Unmarshal(a.tool.Parameters(), &schema); err != nil || schema == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return schema
}

func (a toolAdapter) Execute(ctx context.Context, _ *coretools.ToolContext, input map[string]any) (coretools.ToolResult, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return coretools.NewErrorResultf("failed to marshal tool input: %v", err), nil
	}

	toolResult, execErr := a.tool.Execute(ctx, json.RawMessage(payload))
	if execErr != nil {
		return coretools.NewErrorResult(execErr), nil
	}

	return coretools.ToolResult{
		Content: toolResult.Content,
		IsError: toolResult.IsError,
	}, nil
}
