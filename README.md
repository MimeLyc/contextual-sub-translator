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

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_API_URL` | LLM API endpoint | `https://openrouter.ai/api/v1` |
| `LLM_MODEL` | Model to use | `openai/gpt-3.5-turbo` |
| `LLM_MAX_TOKENS` | Max tokens per request | `8000` |
| `LLM_TEMPERATURE` | Sampling temperature | `0.7` |
| `LLM_TIMEOUT` | Request timeout (seconds) | `30` |
| `SEARCH_API_KEY` | Tavily API key for web search | (empty - disables search) |
| `SEARCH_API_URL` | Search API endpoint | `https://api.tavily.com/search` |
| `AGENT_MAX_ITERATIONS` | Max tool calling iterations | `10` |
| `AGENT_BUNDLE_CONCURRENCY` | Parallel bundle workers | `1` |
| `LOG_LEVEL` | Log level (`DEBUG/INFO/WARN/ERROR/FATAL`) | `INFO` |
| `CRON_EXPR` | Cron expression for scheduled translation | `0 0 * * *` |
| `MOVIE_DIR` | Movie root directory | `/movies` |
| `ANIMATION_DIR` | Animation root directory | `/animations` |
| `TELEPLAY_DIR` | Teleplay root directory | `/teleplays` |
| `SHOW_DIR` | Show root directory | `/shows` |
| `DOCUMENTARY_DIR` | Documentary root directory | `/documentaries` |
| `PUID` | Container user id | `1000` |
| `PGID` | Container group id | `1000` |
| `TZ` | Timezone | `UTC` |
| `ZONE` | Zone info | `local` |

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
├── agent/           # Local adapter around agent-core-go
│   ├── agent.go     # LLMAgent wrapper + tool adapters
│   └── types.go     # AgentRequest, AgentResult types
├── tools/           # Tool implementations
│   ├── tool.go      # Tool interface
│   ├── registry.go  # Tool registry
│   └── web_search.go    # Web search tool (Tavily API)
├── translator/      # Translation logic using agent
│   └── llm.go       # agentTranslator implementation
├── termmap/         # Term map generation/extraction
└── config/          # Configuration management
    └── config.go    # SearchConfig, AgentConfig
```

### How It Works

1. **Agent Layer** (`internal/agent/`) - Wraps `agent-core-go` and adapts project tool interfaces
2. **Tools Layer** (`internal/tools/`) - Implements tools like `web_search` for terminology lookup
3. **agent-core-go** (`github.com/MimeLyc/agent-core-go`) - Provides orchestration loop and OpenAI-compatible provider implementation
4. **Translator Layer** (`internal/translator/`) - Translation logic using the agent

### Agent Loop

The runtime loop is provided by `agent-core-go`.

`internal/agent/agent.go` only maps project requests/results and adapts `internal/tools` into `agent-core-go` tool interfaces, while `MaxIterations` remains configurable via `AGENT_MAX_ITERATIONS`.

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
