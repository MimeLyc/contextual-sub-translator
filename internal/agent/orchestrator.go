package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
	"github.com/MimeLyc/contextual-sub-translator/internal/tools"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

// Orchestrator manages the agent loop for tool calling
type Orchestrator struct {
	client        *llm.Client
	registry      *tools.Registry
	maxIterations int
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator(client *llm.Client, registry *tools.Registry, maxIterations int) *Orchestrator {
	return &Orchestrator{
		client:        client,
		registry:      registry,
		maxIterations: maxIterations,
	}
}

// Run executes the agent loop
func (o *Orchestrator) Run(ctx context.Context, req AgentRequest) (*AgentResult, error) {
	result := &AgentResult{
		ToolCalls:  make([]ToolCallRecord, 0),
		Iterations: 0,
	}

	// Build initial messages
	messages := []llm.Message{
		{Role: "user", Content: req.UserMessage},
	}

	// Get available tools
	toolDefs := o.registry.ToOpenAIFormat()

	opts := llm.NewChatCompletionOptions().
		WithSystemPrompt(req.SystemPrompt)

	// Agent loop
	for i := 0; i < o.maxIterations; i++ {
		result.Iterations++

		var resp *llm.ChatResponse
		var err error

		// Call LLM with or without tools
		if len(toolDefs) > 0 {
			resp, err = o.client.ChatCompletionWithTools(ctx, messages, toolDefs, opts)
		} else {
			resp, err = o.client.ChatCompletion(ctx, messages, opts)
		}

		if err != nil {
			return nil, fmt.Errorf("LLM call failed at iteration %d: %w", i+1, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no choices in response at iteration %d", i+1)
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		// Check finish reason
		switch choice.FinishReason {
		case "stop":
			// Agent is done, return the content
			result.Content = assistantMsg.Content
			return result, nil

		case "tool_calls":
			// Process tool calls
			if len(assistantMsg.ToolCalls) == 0 {
				// No tool calls but finish reason says tool_calls - treat as done
				result.Content = assistantMsg.Content
				return result, nil
			}

			// Add assistant message to conversation
			messages = append(messages, assistantMsg)

			// Execute each tool call
			for _, toolCall := range assistantMsg.ToolCalls {
				toolResult := o.executeTool(ctx, toolCall)
				result.ToolCalls = append(result.ToolCalls, toolResult)

				// Add tool result to messages
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    toolResult.Result,
					ToolCallID: toolCall.ID,
				})

				log.Info("Tool %s executed: error=%v", toolCall.Function.Name, toolResult.IsError)
			}

			// Clear system prompt for subsequent iterations (already in context)
			opts = llm.NewChatCompletionOptions()

		default:
			// Unknown finish reason, treat content as final response
			result.Content = assistantMsg.Content
			return result, nil
		}
	}

	// Max iterations reached
	return nil, fmt.Errorf("max iterations (%d) reached without completion", o.maxIterations)
}

func (o *Orchestrator) executeTool(ctx context.Context, toolCall llm.ToolCall) ToolCallRecord {
	record := ToolCallRecord{
		ToolName:  toolCall.Function.Name,
		Arguments: toolCall.Function.Arguments,
	}

	tool, exists := o.registry.Get(toolCall.Function.Name)
	if !exists {
		record.Result = fmt.Sprintf("Tool %q not found", toolCall.Function.Name)
		record.IsError = true
		return record
	}

	result, err := tool.Execute(ctx, json.RawMessage(toolCall.Function.Arguments))
	if err != nil {
		record.Result = fmt.Sprintf("Tool execution error: %v", err)
		record.IsError = true
		return record
	}

	record.Result = result.Content
	record.IsError = result.IsError
	return record
}
