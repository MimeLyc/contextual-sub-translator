# Contextual Subtitle Translator

AI-powered subtitle translator with contextual understanding, web search, and a built-in web UI.

## Features

- **Contextual Translation**: Uses media metadata (NFO files) to improve translation accuracy
- **Agent-based Architecture**: Unified AI layer with tool calling support
- **Web Search Integration**: Automatically searches for official character names, place names, and terminology in target language
- **Batch Processing**: Efficient batch translation with configurable batch sizes

## Quick Start

```bash
# Build
make build

# Run service (cron + HTTP API + UI)
./ctxtrans

# With Docker
docker compose up --build
```

After startup, open `http://localhost:8080` for the Jellyfin-style UI.

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
| `HTTP_ADDR` | HTTP listen address | `:8080` |
| `UI_STATIC_DIR` | Built frontend static directory | `/app/web` |
| `UI_ENABLE` | Enable web UI/static hosting | `true` |
| `DATA_DIR` | Persistent data directory (`ctxtrans.db` lives here) | `/app/data` |
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

### Configuration Priority

Configuration is resolved in the following order (highest priority wins):

```
1. Hardcoded defaults (lowest)
2. Environment variables
3. Runtime settings file (highest)
```

On startup, the application:

1. Builds the full `Config` from environment variables (falling back to hardcoded defaults for unset vars)
2. Attempts to load the runtime settings file (`settings.json`, path controlled by `SETTINGS_FILE` env var, default `/app/config/settings.json`)
3. If the file exists, its non-empty fields override the corresponding config values — **overriding both defaults and environment variables**

#### Runtime Settings (dynamic)

The following fields can be changed at runtime via `PUT /api/settings` or by editing the settings file. They take precedence over environment variables:

| Settings Field | Overrides Env Var |
|---------------|-------------------|
| `llm_api_url` | `LLM_API_URL` |
| `llm_api_key` | `LLM_API_KEY` |
| `llm_model` | `LLM_MODEL` |
| `cron_expr` | `CRON_EXPR` |
| `target_language` | (hardcoded `Chinese`) |

All other configuration (media directories, HTTP address, agent parameters, etc.) can **only** be set via environment variables.

Runtime updates via the HTTP API are written to `settings.json` atomically (temp file + rename) and take effect immediately. They persist across restarts.

### Persistence

The service stores queue state and translation progress in SQLite at:

`$DATA_DIR/ctxtrans.db` (default `/app/data/ctxtrans.db`).

Persisting `DATA_DIR` as a volume is required for restart-resume behavior.

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

# Build frontend
cd web && npm install && npm run build

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Lint code
make lint
```

## License

MIT
