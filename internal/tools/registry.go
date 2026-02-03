package tools

import (
	"fmt"
	"sync"

	"github.com/MimeLyc/contextual-sub-translator/internal/llm"
)

// Registry manages available tools for the agent
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
// Returns an error if a tool with the same name already exists
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}

	r.tools[name] = tool
	return nil
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all registered tool names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// ToOpenAIFormat converts all registered tools to OpenAI tool definition format
func (r *Registry) ToOpenAIFormat() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, llm.ToolDefinition{
			Type: "function",
			Function: llm.Function{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		})
	}
	return definitions
}
