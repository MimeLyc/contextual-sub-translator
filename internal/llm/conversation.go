package llm

import (
	"context"
	"fmt"
	"time"
)

// Conversation represents a conversation with context and history
// Provides an easy way to maintain conversation state and system context
//
// client: The OpenRouter client
// systemPrompt: System prompt for the conversation context
// messages: History of messages in the conversation
// maxHistory: Maximum number of messages to keep in history
type Conversation struct {
	client       *Client
	systemPrompt string
	messages     []Message
	maxHistory   int
	createdAt    time.Time
	updatedAt    time.Time
}

// NewConversation creates a new conversation with the given client and system prompt
//
// client: The OpenRouter client
// systemPrompt: System prompt for the conversation context
// maxHistory: Maximum number of messages to keep in history (default: 100)
//
// Example:
//
//	config, _ := llm.NewConfig()
//	client, _ := llm.NewClient(config)
//	conv := llm.NewConversation(client, "You are a helpful assistant.", 50)
func NewConversation(client *Client, systemPrompt string, maxHistory int) *Conversation {
	if maxHistory <= 0 {
		maxHistory = 100
	}

	return &Conversation{
		client:       client,
		systemPrompt: systemPrompt,
		messages:     make([]Message, 0),
		maxHistory:   maxHistory,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
	}
}

// NewConversationWithOptions creates a new conversation with additional options
//
// client: The OpenRouter client
// options: Additional options for the conversation
type ConversationOption func(*Conversation)

func NewConversationWithOptions(client *Client, opts ...ConversationOption) *Conversation {
	conv := &Conversation{
		client:     client,
		messages:   make([]Message, 0),
		maxHistory: 100,
		createdAt:  time.Now(),
		updatedAt:  time.Now(),
	}

	for _, opt := range opts {
		opt(conv)
	}

	return conv
}

// WithSystemPrompt sets the system prompt for the conversation
func WithSystemPrompt(prompt string) ConversationOption {
	return func(c *Conversation) {
		c.systemPrompt = prompt
	}
}

// WithMaxHistory sets the maximum number of messages to keep in history
func WithMaxHistory(maxHistory int) ConversationOption {
	return func(c *Conversation) {
		c.maxHistory = maxHistory
	}
}

// WithInitialMessages sets initial messages for the conversation
func WithInitialMessages(messages []Message) ConversationOption {
	return func(c *Conversation) {
		c.messages = append(c.messages, messages...)
	}
}

// SendMessage sends a message in the conversation and gets a response
//
// ctx: Context for the request
// content: The message content
// files: Optional files to attach to the message
//
// # Returns the assistant's response content or an error
//
// Example:
//
//	response, err := conv.SendMessage(ctx, "What is Go?", nil)
//	if err != nil {
//		log.Printf("Error: %v", err)
//		return
//	}
//	fmt.Println("Assistant:", response)
func (c *Conversation) SendMessage(ctx context.Context, content string, files []File) (string, error) {
	return c.SendMessageWithOptions(ctx, content, files, nil)
}

// SendMessageWithOptions sends a message with additional options
//
// ctx: Context for the request
// content: The message content
// files: Optional files to attach to the message
// opts: Additional options for the request
//
// Returns the assistant's response content or an error
func (c *Conversation) SendMessageWithOptions(ctx context.Context, content string, files []File, opts *ChatCompletionOptions) (string, error) {
	// Create user message
	userMessage := Message{
		Role:      "user",
		Content:   content,
		FilePaths: []string{},
	}

	// Add files to the message
	if len(files) > 0 {
		for _, file := range files {
			if file.Content != nil {
				fileContent, err := file.ToMessage()
				if err != nil {
					return "", fmt.Errorf("failed to process file %s: %w", file.Name, err)
				}
				userMessage.Content += "\n\n" + fileContent
			}
		}
	}

	// Add user message to history
	c.addMessage(userMessage)

	// Get all messages including system prompt
	messages := c.prepareMessages()

	// Create completion options
	completionOpts := NewChatCompletionOptions()
	if opts != nil {
		completionOpts = opts
	}

	// Make API request
	response, err := c.client.ChatCompletion(ctx, messages, completionOpts)
	if err != nil {
		return "", fmt.Errorf("chat completion failed: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	// Extract assistant response
	assistantContent := response.Choices[0].Message.Content

	// Add assistant message to history
	assistantMessage := Message{
		Role:    "assistant",
		Content: assistantContent,
	}
	c.addMessage(assistantMessage)

	c.updatedAt = time.Now()

	return assistantContent, nil
}

// ClearHistory clears the conversation history
// Keeps the system prompt but removes all messages
func (c *Conversation) ClearHistory() {
	c.messages = make([]Message, 0)
	c.updatedAt = time.Now()
}

// GetHistory returns the conversation history
func (c *Conversation) GetHistory() []Message {
	history := make([]Message, len(c.messages))
	copy(history, c.messages)
	return history
}

// GetContext returns the conversation context including system prompt
func (c *Conversation) GetContext() string {
	return c.systemPrompt
}

// SetContext updates the system prompt and clears history
func (c *Conversation) SetContext(newContext string) {
	c.systemPrompt = newContext
	c.ClearHistory()
	c.updatedAt = time.Now()
}

// UpdateContext updates the system prompt without clearing history
func (c *Conversation) UpdateContext(newContext string) {
	c.systemPrompt = newContext
	c.updatedAt = time.Now()
}

// GetMessageCount returns the number of messages in the conversation
func (c *Conversation) GetMessageCount() int {
	return len(c.messages)
}

// GetConversationLength returns the approximate length of the conversation in tokens
// This is a rough estimation for monitoring purposes
func (c *Conversation) GetConversationLength() int {
	length := 0
	for _, msg := range c.messages {
		length += len(msg.Content) / 4 // Rough estimate: 1 token ≈ 4 characters
	}
	if c.systemPrompt != "" {
		length += len(c.systemPrompt) / 4
	}
	return length
}

// IsEmpty returns true if the conversation has no messages
func (c *Conversation) IsEmpty() bool {
	return len(c.messages) == 0
}

// Export returns a serializable representation of the conversation
func (c *Conversation) Export() *ConversationData {
	return &ConversationData{
		SystemPrompt: c.systemPrompt,
		Messages:     c.messages,
		MaxHistory:   c.maxHistory,
		CreatedAt:    c.createdAt,
		UpdatedAt:    c.updatedAt,
	}
}

// Import restores a conversation from exported data
func (c *Conversation) Import(data *ConversationData) {
	c.systemPrompt = data.SystemPrompt
	c.messages = make([]Message, len(data.Messages))
	copy(c.messages, data.Messages)
	c.maxHistory = data.MaxHistory
	c.createdAt = data.CreatedAt
	c.updatedAt = data.UpdatedAt
}

// addMessage adds a message to the history, maintaining max history limit
func (c *Conversation) addMessage(msg Message) {
	c.messages = append(c.messages, msg)

	// Trim history if it exceeds maxHistory
	if len(c.messages) > c.maxHistory {
		// Keep the most recent messages
		excess := len(c.messages) - c.maxHistory
		c.messages = c.messages[excess:]
	}
}

// prepareMessages prepares messages for the API, including system prompt
func (c *Conversation) prepareMessages() []Message {
	messages := make([]Message, 0)

	// Add system prompt if provided
	if c.systemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: c.systemPrompt,
		})
	}

	// Add conversation history
	messages = append(messages, c.messages...)

	return messages
}

// ConversationData represents the serializable conversation state
type ConversationData struct {
	SystemPrompt string    `json:"system_prompt"`
	Messages     []Message `json:"messages"`
	MaxHistory   int       `json:"max_history"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewConversationFromData creates a new conversation from exported data
func NewConversationFromData(client *Client, data *ConversationData) *Conversation {
	conv := &Conversation{
		client:       client,
		systemPrompt: data.SystemPrompt,
		messages:     make([]Message, len(data.Messages)),
		maxHistory:   data.MaxHistory,
		createdAt:    data.CreatedAt,
		updatedAt:    data.UpdatedAt,
	}

	copy(conv.messages, data.Messages)

	return conv
}

// NewConversationFromMessages creates a new conversation with predefined messages
func NewConversationFromMessages(client *Client, systemPrompt string, messages []Message, maxHistory int) *Conversation {
	conv := NewConversation(client, systemPrompt, maxHistory)
	conv.messages = make([]Message, len(messages))
	copy(conv.messages, messages)
	conv.updatedAt = time.Now()
	return conv
}

// ConversationManager manages multiple conversations
// Useful for applications that need to handle multiple concurrent conversations
type ConversationManager struct {
	client        *Client
	conversations map[string]*Conversation
}

// NewConversationManager creates a new conversation manager
func NewConversationManager(client *Client) *ConversationManager {
	return &ConversationManager{
		client:        client,
		conversations: make(map[string]*Conversation),
	}
}

// Create creates a new conversation with a given ID
func (cm *ConversationManager) Create(id string, systemPrompt string, maxHistory int) *Conversation {
	conv := NewConversation(cm.client, systemPrompt, maxHistory)
	cm.conversations[id] = conv
	return conv
}

// Get retrieves a conversation by ID
func (cm *ConversationManager) Get(id string) (*Conversation, bool) {
	conv, exists := cm.conversations[id]
	return conv, exists
}

// Delete removes a conversation by ID
func (cm *ConversationManager) Delete(id string) {
	delete(cm.conversations, id)
}

// List returns all conversation IDs
func (cm *ConversationManager) List() []string {
	ids := make([]string, 0, len(cm.conversations))
	for id := range cm.conversations {
		ids = append(ids, id)
	}
	return ids
}

// Clear removes all conversations
func (cm *ConversationManager) Clear() {
	cm.conversations = make(map[string]*Conversation)
}
