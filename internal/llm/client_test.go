package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	config := &Config{
		APIKey:      "test-key",
		APIURL:      "https://api.example.com",
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, config, client.config)
	assert.Equal(t, config.APIURL, client.baseURL)
	assert.NotNil(t, client.httpClient)

	// Test with invalid config
	invalidConfig := &Config{} // Missing API key
	_, err = NewClient(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestClientWithMockServer(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Verify headers
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Mock successful response
		response := `{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello! This is a test response."
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	// Create client with mock server URL
	config := &Config{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	messages := []Message{
		{Role: "user", Content: "Hello, how are you?"},
	}

	response, err := client.ChatCompletion(ctx, messages, nil)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test-id", response.ID)
	assert.Equal(t, "test-model", response.Model)
	assert.Len(t, response.Choices, 1)
	assert.Equal(t, "Hello! This is a test response.", response.Choices[0].Message.Content)
	assert.Equal(t, 30, response.Usage.TotalTokens)
}

func TestClientErrorHandling(t *testing.T) {
	// Test with server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)

		response := `{
			"error": {
				"message": "Invalid API key",
				"type": "authentication_error",
				"code": "401"
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	config := &Config{
		APIKey:      "invalid-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	response, err := client.ChatCompletion(ctx, messages, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	if response != nil && response.Error != nil {
		assert.Equal(t, "Invalid API key", response.Error.Message)
	}
}

func TestSimpleChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := `{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Simple chat response"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 5,
				"completion_tokens": 10,
				"total_tokens": 15
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	config := &Config{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	response, err := client.SimpleChat(ctx, "Hello", "You are a helpful assistant")

	require.NoError(t, err)
	assert.Equal(t, "Simple chat response", response)
}

func TestChatWithFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := `{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "I can see your file content"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 15,
				"completion_tokens": 8,
				"total_tokens": 23
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	config := &Config{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	// Create test file
	file := File{
		Name:        "test.txt",
		ContentType: "text/plain",
		Content:     []byte("This is test file content"),
	}

	ctx := context.Background()
	response, err := client.ChatWithFiles(ctx, "Summarize this", []File{file}, "You are a helpful assistant")

	require.NoError(t, err)
	assert.Equal(t, "I can see your file content", response)
}

func TestClientGetModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "/models", r.URL.Path)

		response := `{
			"data": [
				{"id": "test-model-1", "name": "Test Model 1"},
				{"id": "test-model-2", "name": "Test Model 2"}
			]
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	config := &Config{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	models, err := client.GetModels(ctx)

	require.NoError(t, err)
	assert.Len(t, models, 1) // Simplified response
	assert.Equal(t, "test-model", models[0].ID)
}

func TestClientConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Response"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 5,
				"completion_tokens": 5,
				"total_tokens": 10
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	config := &Config{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	// Test concurrent requests
	ctx := context.Background()
	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.ChatCompletion(ctx, messages, nil)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestInvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	config := &Config{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   1000,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	_, err = client.ChatCompletion(ctx, messages, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

const (
	defaultAPIURL = "https://openrouter.ai/api/v1"
	defaultModel  = "google/gemini-2.5-flash"
)

// TestOpenRouterIntegration tests actual connection to OpenRouter API
// This test is skipped by default and requires OPENROUTER_API_KEY environment variable
func TestOpenRouterIntegration(t *testing.T) {

	_ = godotenv.Load("./.env")
	// This test will be skipped if OPENROUTER_API_KEY is not set
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		t.Skip("Set LLM_API_KEY environment variable to run this test")
	}

	config := &Config{
		APIKey:      apiKey,
		APIURL:      defaultAPIURL,
		Model:       defaultModel,
		MaxTokens:   100,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Test SimpleChat
	t.Run("SimpleChat", func(t *testing.T) {
		response, err := client.SimpleChat(ctx, "Hello, can you hear me?", "You are a helpful assistant. Reply briefly.")
		assert.NoError(t, err)
		assert.NotEmpty(t, response)
		assert.Contains(t, strings.ToLower(response), "hello")
		assert.Contains(t, strings.ToLower(response), "yes")
	})

	// Test ChatCompletion
	t.Run("ChatCompletion", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "What is 2+2?"},
		}

		response, err := client.ChatCompletion(ctx, messages, nil)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.Choices, 1)
		assert.NotEmpty(t, response.Choices[0].Message.Content)
		assert.Contains(t, response.Choices[0].Message.Content, "4")
	})

	// Test GetModels
	t.Run("GetModels", func(t *testing.T) {
		models, err := client.GetModels(ctx)
		assert.NoError(t, err)
		assert.NotEmpty(t, models)
		// Look for Claude model in response
		found := false
		for _, model := range models {
			if model.ID == "anthropic/claude-3-haiku-20240307" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected to find Claude-3-haiku model")
	})

	// Test with sample file
	t.Run("ChatWithFiles", func(t *testing.T) {
		file := File{
			Name:        "test.txt",
			ContentType: "text/plain",
			Content:     []byte("This is a simple test file with sample content."),
		}

		response, err := client.ChatWithFiles(ctx, "What is this file about? Summarize in one sentence.", []File{file}, "You are a helpful assistant.")
		assert.NoError(t, err)
		assert.NotEmpty(t, response)
		assert.Contains(t, strings.ToLower(response), "test")
	})
}

// TestOpenRouterIntegrationWithEnv reads API key from environment
func TestOpenRouterIntegrationWithEnv(t *testing.T) {
	_ = godotenv.Load("./.env")
	// This test will be skipped if OPENROUTER_API_KEY is not set
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		t.Skip("LLM_API_KEY environment variable not set, skipping integration test")
	}

	config := &Config{
		APIKey:      apiKey,
		APIURL:      defaultAPIURL,
		Model:       defaultModel,
		MaxTokens:   50,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Test basic chat functionality
	messages := []Message{
		{Role: "user", Content: "Say 'test passed' if you can see this message"},
	}

	response, err := client.ChatCompletion(ctx, messages, nil)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Choices, 1)
	assert.NotEmpty(t, response.Choices[0].Message.Content)
	assert.NotEmpty(t, response.Usage.TotalTokens)
	assert.Greater(t, response.Usage.TotalTokens, 0)
}

// TestOpenRouterIntegrationQuestionAnswer tests actual Q&A with OpenRouter API
func TestOpenRouterIntegrationQuestionAnswer(t *testing.T) {
	_ = godotenv.Load("./.env")
	// This test demonstrates actual Q&A with OpenRouter API
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		t.Skip("LLM_API_KEY environment variable not set, skipping Q&A integration test")
	}

	config := &Config{
		APIKey:      apiKey,
		APIURL:      defaultAPIURL,
		Model:       defaultModel,
		MaxTokens:   200,
		Temperature: 0.7,
		Timeout:     30,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("SimpleQuestion", func(t *testing.T) {
		question := "What is the capital of France?"
		response, err := client.SimpleChat(ctx, question, "Answer briefly and accurately")

		assert.NoError(t, err)
		assert.NotEmpty(t, response)
		assert.Contains(t, strings.ToLower(response), "paris")
	})

	t.Run("ComplexQuestion", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "Explain the difference between machine learning and artificial intelligence in one sentence."},
		}

		response, err := client.ChatCompletion(ctx, messages, nil)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Len(t, response.Choices, 1)
		assert.NotEmpty(t, response.Choices[0].Message.Content)

		content := strings.ToLower(response.Choices[0].Message.Content)
		assert.True(t, strings.Contains(content, "ai") || strings.Contains(content, "artificial intelligence"))
		assert.True(t, strings.Contains(content, "machine learning") || strings.Contains(content, "ml"))
	})

	t.Run("CodeQuestion", func(t *testing.T) {
		question := "Write a simple Go function to add two numbers"
		response, err := client.SimpleChat(ctx, question, "You are a Go programming expert. Provide only the code without explanation.")

		assert.NoError(t, err)
		assert.NotEmpty(t, response)
		assert.Contains(t, response, "func")
		assert.Contains(t, response, "int")
	})
}
