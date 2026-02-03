package agent

import (
	"context"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/tools"
)

// Agent defines the interface for an agent that can execute tasks
type Agent interface {
	// Execute runs the agent with the given request
	Execute(ctx context.Context, req AgentRequest) (*AgentResult, error)

	// Close releases any resources held by the agent
	Close() error
}

// LLMAgent implements the Agent interface using an LLM with tool calling
type LLMAgent struct {
	client        *llm.Client
	registry      *tools.Registry
	maxIterations int
}

// NewLLMAgent creates a new LLM-based agent
func NewLLMAgent(client *llm.Client, registry *tools.Registry, maxIterations int) *LLMAgent {
	if maxIterations <= 0 {
		maxIterations = 10
	}
	return &LLMAgent{
		client:        client,
		registry:      registry,
		maxIterations: maxIterations,
	}
}

// Execute runs the agent with the given request
func (a *LLMAgent) Execute(ctx context.Context, req AgentRequest) (*AgentResult, error) {
	orchestrator := NewOrchestrator(a.client, a.registry, a.getMaxIterations(req))
	return orchestrator.Run(ctx, req)
}

// Close releases any resources held by the agent
func (a *LLMAgent) Close() error {
	// No resources to release currently
	return nil
}

func (a *LLMAgent) getMaxIterations(req AgentRequest) int {
	if req.MaxIterations > 0 {
		return req.MaxIterations
	}
	return a.maxIterations
}
