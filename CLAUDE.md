# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build and Development
- `make build` - Build the ctxtrans binary
- `make install` - Install to $GOPATH/bin
- `make clean` - Clean build artifacts
- `go build -o ctxtrans ./cmd/main.go` - Direct build command

### Testing and Quality
- `make test` - Run tests for internal/ctxtrans package
- `make test-coverage` - Run tests with coverage report
- `go test ./...` - Run all tests in the project
- `make fmt` - Format code using go fmt
- `make lint` - Run golangci-lint (requires golangci-lint installation)
- `make deps` - Check and tidy module dependencies

### Development Tools
- `make example` - Run example with demo API key
- `make help` - Show available Makefile targets

## Architecture

This is a contextual subtitle translator that uses LLM APIs to translate subtitles with media context information.

### Core Components

**Main Application Flow:**
- `cmd/main.go` - Entry point, initializes configuration
- `internal/config/` - Configuration management with environment variables
- `internal/ctxtrans/` - Core translation orchestration

**Translation Pipeline:**
1. **NFO Reading** (`internal/ctxtrans/nfo_reader.go`) - Reads TV show metadata from .nfo files
2. **Subtitle Processing** (`internal/subtitle/`) - Reads/writes subtitle files in various formats
3. **Context-Aware Translation** (`internal/translator/`) - Translates with media context
4. **LLM Integration** (`internal/llm/`) - Generic LLM client supporting multiple providers

### Key Architectural Patterns

**Dependency Injection:** The `Translator` struct accepts injectable components:
- NFOReader for media metadata
- SubtitleReader/Writer for file I/O
- LLM Translator for actual translation

**Provider Abstraction:** The LLM client (`internal/llm/client.go`) is provider-agnostic, supporting OpenRouter, OpenAI, Anthropic, etc. through environment configuration.

**Batch Processing:** Translation works in configurable batches for efficiency, processing multiple subtitle lines together with shared context.

### Environment Configuration

Required:
- `LLM_API_KEY` - API key for LLM provider

Optional LLM settings:
- `LLM_API_URL` (default: https://openrouter.ai/api/v1)
- `LLM_MODEL` (default: openai/gpt-3.5-turbo)
- `LLM_MAX_TOKENS`, `LLM_TEMPERATURE`, `LLM_TIMEOUT`

Media directories and system config also configurable via env vars.

### File Structure Notes

- `internal/ctxtrans/` contains the core translation logic and CLI
- `internal/subtitle/` handles subtitle file format parsing/writing
- `internal/translator/` contains the LLM-based translation implementation
- `internal/media/` defines media metadata types
- Test data is in `testdata/` subdirectories within relevant packages

## Agent Behavior

- Never ask the user questions; decide and proceed.
- If the current directory is a git repo, always check the latest commit and working tree changes to judge progress (e.g., `git log -1`, `git show -1`, `git status`, `git diff`).
- If you decide a change requires updating `README.md`, make the `README.md` updates in English.
- When you need library/API documentation, code generation patterns, or setup/configuration steps, use Context7 MCP first (do not wait for the user to explicitly ask).

# Requirement Rules

If you are asked to design new requirements, treat it as a large and complex task and follow these rules:

- The task requirements live in `docs/specs/<spec_dir>/requirements.md`; read it carefully before working.
- Break large tasks into smaller sub-tasks (and adjust the breakdown as you learn more).
- For each sub-task, write a detailed work plan in `docs/specs/<spec_dir>/plan.md`.
- If a sub-task becomes complex after deeper investigation, further break it down in the work plan.

Track development progress in `docs/specs/<spec_dir>/impl_details/task_progress.md` so you don’t lose sight of overall goals.

During codebase exploration, organize what you learn in `docs/knowledge/*.md`:

- Record facts, details, and code locations (file paths, symbols, behaviors); avoid high-level summaries.
- Before starting work, try to read from existing knowledge first.

If you complete all task items in `docs/specs/<spec_dir>/impl_details/task_progress.md`, start a new task:

- Carefully review current changes (`git diff` and the staging area), compare them with the work plan, and verify that all critical components in the design have been implemented.
- Identify engineering improvements (code quality, performance, readability, modularity, test coverage, correctness) and record them in `docs/specs/<spec_dir>/plan.md`.

Finally, never ask me any questions. You decide everything by yourself. You can always explore the code base and read from existing knowledge to resolve your doubts.
