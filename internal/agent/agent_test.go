package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type echoAdapterTool struct{}

func (echoAdapterTool) Name() string { return "echo" }

func (echoAdapterTool) Description() string { return "Echo back input arguments." }

func (echoAdapterTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string"}
		},
		"required": ["text"]
	}`)
}

func (echoAdapterTool) Execute(_ context.Context, args json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: string(args)}, nil
}

func TestNewLLMAgent_InvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := NewLLMAgent(LLMConfig{
		APIURL: "https://example.com",
		Model:  "model",
	}, tools.NewRegistry(), 3)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestLLMAgent_Execute_WithToolCalling(t *testing.T) {
	t.Parallel()

	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}

		_, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		switch atomic.AddInt32(&callCount, 1) {
		case 1:
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-1",
				"object":"chat.completion",
				"created":123,
				"model":"test-model",
				"choices":[
					{
						"index":0,
						"finish_reason":"tool_calls",
						"message":{
							"role":"assistant",
							"content":"",
							"tool_calls":[
								{
									"id":"call_1",
									"type":"function",
									"function":{
										"name":"echo",
										"arguments":"{\"text\":\"hello\"}"
									}
								}
							]
						}
					}
				],
				"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
			}`))
		default:
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-2",
				"object":"chat.completion",
				"created":124,
				"model":"test-model",
				"choices":[
					{
						"index":0,
						"finish_reason":"stop",
						"message":{
							"role":"assistant",
							"content":"done"
						}
					}
				],
				"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}
			}`))
		}
	}))
	t.Cleanup(server.Close)

	registry := tools.NewRegistry()
	require.NoError(t, registry.Register(echoAdapterTool{}))

	a, err := NewLLMAgent(LLMConfig{
		APIKey:      "test-key",
		APIURL:      server.URL,
		Model:       "test-model",
		MaxTokens:   256,
		Temperature: 0.2,
		Timeout:     10,
	}, registry, 5)
	require.NoError(t, err)

	result, err := a.Execute(context.Background(), AgentRequest{
		SystemPrompt: "You are helpful",
		UserMessage:  "Say hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "done", result.Content)
	assert.GreaterOrEqual(t, result.Iterations, 1)
	require.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "echo", result.ToolCalls[0].ToolName)
	assert.JSONEq(t, `{"text":"hello"}`, result.ToolCalls[0].Arguments)
	assert.JSONEq(t, `{"text":"hello"}`, result.ToolCalls[0].Result)
}

func TestUnsupportedConfigNotes(t *testing.T) {
	t.Parallel()

	notes := unsupportedConfigNotes(LLMConfig{
		Temperature: 0.2,
		SiteURL:     "https://example.com",
		AppName:     "ctxtrans",
	})

	assert.Contains(t, notes, "temperature")
	assert.Contains(t, notes, "site_url")
	assert.Contains(t, notes, "app_name")
}

func TestLLMAgent_Execute_ReasoningContentErrorReturnsFailure(t *testing.T) {
	t.Parallel()

	var toolPathCalls int32
	var noToolsCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}

		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))

		hasTools := false
		if rawTools, ok := payload["tools"].([]any); ok && len(rawTools) > 0 {
			hasTools = true
		}

		w.Header().Set("Content-Type", "application/json")
		if hasTools {
			switch atomic.AddInt32(&toolPathCalls, 1) {
			case 1:
				_, _ = w.Write([]byte(`{
					"id":"chatcmpl-tool-1",
					"object":"chat.completion",
					"created":123,
					"model":"test-model",
					"choices":[
						{
							"index":0,
							"finish_reason":"tool_calls",
							"message":{
								"role":"assistant",
								"content":"",
								"tool_calls":[
									{
										"id":"call_1",
										"type":"function",
										"function":{
											"name":"echo",
											"arguments":"{\"text\":\"hello\"}"
										}
									}
								]
							}
						}
					],
					"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
				}`))
			default:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{
					"error":{
						"type":"invalid_request_error",
						"message":"thinking is enabled but reasoning_content is missing in assistant tool call message at index 2"
					}
				}`))
			}
			return
		}

		atomic.AddInt32(&noToolsCalls, 1)
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-no-tools",
			"object":"chat.completion",
			"created":124,
			"model":"test-model",
			"choices":[
				{
					"index":0,
					"finish_reason":"stop",
					"message":{
						"role":"assistant",
						"content":"unexpected no-tools path"
					}
				}
			],
			"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}
		}`))
	}))
	t.Cleanup(server.Close)

	registry := tools.NewRegistry()
	require.NoError(t, registry.Register(echoAdapterTool{}))

	a, err := NewLLMAgent(LLMConfig{
		APIKey:    "test-key",
		APIURL:    server.URL,
		Model:     "test-model",
		MaxTokens: 256,
		Timeout:   10,
	}, registry, 5)
	require.NoError(t, err)

	_, err = a.Execute(context.Background(), AgentRequest{
		SystemPrompt: "You are helpful",
		UserMessage:  "Say hello",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reasoning_content")
	assert.GreaterOrEqual(t, atomic.LoadInt32(&toolPathCalls), int32(2))
	assert.Equal(t, int32(0), atomic.LoadInt32(&noToolsCalls))
}
