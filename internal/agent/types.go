package agent

// AgentRequest represents a request to the agent
type AgentRequest struct {
	// SystemPrompt is the system prompt to set context
	SystemPrompt string

	// UserMessage is the user's message/task
	UserMessage string

	// MaxIterations is the maximum number of tool-calling iterations
	// Default: 10
	MaxIterations int
}

// AgentResult represents the result from an agent execution
type AgentResult struct {
	// Content is the final text response from the agent
	Content string

	// ToolCalls contains a record of all tool calls made during execution
	ToolCalls []ToolCallRecord

	// Iterations is the number of LLM calls made
	Iterations int
}

// ToolCallRecord records a single tool call and its result
type ToolCallRecord struct {
	// ToolName is the name of the tool that was called
	ToolName string

	// Arguments is the JSON arguments passed to the tool
	Arguments string

	// Result is the output from the tool
	Result string

	// IsError indicates if the tool execution resulted in an error
	IsError bool
}
