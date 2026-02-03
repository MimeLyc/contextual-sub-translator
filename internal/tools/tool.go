package tools

import (
	"context"
	"encoding/json"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// Tool defines the interface for tools that can be called by the agent
type Tool interface {
	// Name returns the unique name of the tool
	Name() string

	// Description returns a description of what the tool does
	Description() string

	// Parameters returns the JSON Schema for the tool's parameters
	Parameters() json.RawMessage

	// Execute runs the tool with the given arguments and returns the result
	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}
