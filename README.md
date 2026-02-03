# Contextual Subtitle Translator

AI-powered subtitle translator with contextual understanding and web search capabilities.

## Features

- **Contextual Translation**: Uses media metadata (NFO files) to improve translation accuracy
- **Agent-based Architecture**: Unified AI layer with tool calling support
- **Web Search Integration**: Automatically searches for official character names, place names, and terminology in target language
- **Batch Processing**: Efficient batch translation with configurable batch sizes

## Quick Start

```bash
# Build
make build

# Run translation
./ctxtrans --input video.srt --target-lang Chinese

# With Docker
docker-compose up
```

## Configuration

### Required Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `LLM_API_KEY` | API key for LLM provider | `sk-or-v1-xxxxx` |
| `LLM_API_URL` | LLM API endpoint | `https://openrouter.ai/api/v1` |
| `LLM_MODEL` | Model to use | `google/gemini-2.5-flash` |

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SEARCH_API_KEY` | Tavily API key for web search | (empty - disables search) |
| `SEARCH_API_URL` | Search API endpoint | `https://api.tavily.com/search` |
| `AGENT_MAX_ITERATIONS` | Max tool calling iterations | `10` |
| `LLM_MAX_TOKENS` | Max tokens per request | `5000` |
| `LLM_TEMPERATURE` | Sampling temperature | `0.7` |
| `LLM_TIMEOUT` | Request timeout (seconds) | `120` |

### Web Search (Tavily API)

To enable automatic terminology lookup:

1. Get a Tavily API key from https://tavily.com
2. Set `SEARCH_API_KEY` environment variable

When enabled, the agent will:
- Search for official character names in target language
- Find localized place names and terminology
- Verify translations against authoritative sources

## Architecture

The translator uses an agent-based architecture:

```
internal/
├── agent/           # Agent orchestration with tool calling
│   ├── agent.go     # LLMAgent implementation
│   ├── orchestrator.go  # Agent loop logic
│   └── types.go     # AgentRequest, AgentResult types
├── tools/           # Tool implementations
│   ├── tool.go      # Tool interface
│   ├── registry.go  # Tool registry
│   └── web_search.go    # Web search tool (Tavily API)
├── llm/             # LLM client with tool calling support
│   ├── client.go    # HTTP client with ChatCompletionWithTools
│   └── types.go     # ToolCall, ToolDefinition types
├── translator/      # Translation logic using agent
│   └── llm.go       # agentTranslator implementation
└── config/          # Configuration management
    └── config.go    # SearchConfig, AgentConfig
```

### How It Works

1. **Agent Layer** (`internal/agent/`) - Orchestrates LLM interactions with tool calling loop
2. **Tools Layer** (`internal/tools/`) - Implements tools like `web_search` for terminology lookup
3. **LLM Layer** (`internal/llm/`) - Low-level LLM API communication with OpenAI-compatible tool calling
4. **Translator Layer** (`internal/translator/`) - Translation logic using the agent

### Agent Loop

The agent follows this flow:

1. Send messages + tools to LLM
2. If `FinishReason == "tool_calls"`:
   - Execute each tool via registry
   - Append tool results as `role: "tool"` messages
   - Loop back to step 1
3. If `FinishReason == "stop"`: return final content
4. Guard against infinite loops with MaxIterations

## Development

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Lint code
make lint
```

## License

MIT
