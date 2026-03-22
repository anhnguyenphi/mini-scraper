# Scraper AI

Scrape web pages, convert to markdown, and summarize with AI.

## Features

- **Multiple scraping backends:**
  - **Lightpanda** - Chrome DevTools Protocol (CDP) based scraping
  - **Crawl4AI** - Python-based intelligent crawler with built-in markdown conversion
- HTML to Markdown conversion via [MarkItDown](https://github.com/microsoft/markitdown)
- Summarization with **Ollama**, **Gemini**, or **OpenRouter**
- HTTP API server with built-in web UI
- Custom system/user prompts per request
- Docker Compose for one-command setup

## Prerequisites

- **Go 1.26+**
- **Python 3.10+** with dependencies:
  ```bash
  # Required for both backends
  pip install "markitdown[all]"

  # Optional: for Crawl4AI backend
  pip install crawl4ai
  crawl4ai-setup
  ```
- A headless browser:
  - **Lightpanda backend**: Lightpanda or Chrome with CDP
  - **Crawl4AI backend**: Managed automatically via Playwright
- One summarization provider (Ollama, Gemini API key, or OpenRouter API key)

## Quick Start (Docker)

```bash
cp .env.example .env
# Edit .env to set your API keys
docker compose up -d
# Open http://localhost:8080
```

## Scraper Backends

| Backend | Setup | Best For |
|---------|-------|----------|
| `lightpanda` | Requires external CDP server | Fast, lightweight, production use |
| `crawl4ai` | Self-contained (Playwright) | Easy setup, no external dependencies |

## CLI Usage

### Using Lightpanda (default)

1. Start a headless browser:

```bash
# Lightpanda (binary)
./lightpanda serve --host 127.0.0.1 --port 9222

# Lightpanda (Docker)
docker run -d --name lightpanda -p 9222:9222 lightpanda/browser:nightly

# Chromedp (Docker)
docker run -d -p 9222:9222 --rm --name headless-shell chromedp/headless-shell
```

2. Run:

```bash
# Get AI summary (default: Ollama, lightpanda backend)
go run . https://example.com

# Output raw HTML
go run . --raw https://example.com
```

### Using Crawl4AI

1. Install crawl4ai:

```bash
pip install crawl4ai
crawl4ai-setup
```

2. Run:

```bash
# Use crawl4ai backend
go run . --backend crawl4ai https://example.com

# Output raw HTML with crawl4ai
go run . --backend crawl4ai --raw https://example.com
```

### Common Examples

```bash
# Use a different Ollama model
go run . --model llama3.1 https://example.com

# Use Gemini
GEMINI_API_KEY=your_key go run . --provider gemini https://example.com

# Use OpenRouter
OPENROUTER_API_KEY=sk-or-... go run . --provider openrouter https://example.com

# Use crawl4ai with Gemini
go run . --backend crawl4ai --provider gemini https://example.com
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--backend` | `lightpanda` | Scraper backend: `lightpanda` or `crawl4ai` |
| `--cdp` | `ws://127.0.0.1:9222` | Lightpanda CDP WebSocket URL |
| `--python` | `python3` | Python path for crawl4ai backend |
| `--crawl-script` | `scripts/crawl4ai.py` | Path to crawl4ai.py script |
| `--provider` | `ollama` | Provider: `ollama`, `gemini`, `openrouter` |
| `--ollama` | `http://127.0.0.1:11434` | Ollama API base URL |
| `--model` | `qwen3.5:0.8b` | Ollama model name |
| `--gemini-model` | `gemini-1.5-flash` | Gemini model name |
| `--gemini-base-url` | `https://generativelanguage.googleapis.com/v1beta` | Gemini API base URL |
| `--openrouter-model` | `google/gemini-2.0-flash-001` | OpenRouter model ID |
| `--timeout` | `30` | Page load timeout (seconds) |
| `--raw` | `false` | Output raw HTML instead of markdown |

### CLI Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GEMINI_API_KEY` | When using `gemini` | Google Gemini API key |
| `OPENROUTER_API_KEY` | When using `openrouter` | OpenRouter API key |

## HTTP API Server

```bash
go run . serve
# or
go run . serve --listen :3000
```

### Server Flags / Env Vars

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--listen` | `LISTEN` | `:8080` | Listen address |
| `--cdp` | `CDP_URL` | `ws://127.0.0.1:9222` | CDP WebSocket URL |
| `--python` | `PYTHON_PATH` | `python3` | Python path for crawl4ai |
| `--crawl-script` | `CRAWL_SCRIPT_PATH` | `scripts/crawl4ai.py` | crawl4ai script path |
| `--ollama` | `OLLAMA_URL` | `http://127.0.0.1:11434` | Ollama API URL |
| `--model` | `OLLAMA_MODEL` | `qwen3.5:0.8b` | Ollama model |
| `--gemini-model` | `GEMINI_MODEL` | `gemini-1.5-flash` | Gemini model |
| `--gemini-base-url` | `GEMINI_BASE_URL` | `https://...googleapis.com/v1beta` | Gemini base URL |
| `--openrouter-model` | `OPENROUTER_MODEL` | `google/gemini-2.0-flash-001` | OpenRouter model |

Priority: CLI flag > `.env` / environment variable > default.

### API Endpoints

**`POST /api/scrape`** — Scrape a URL and optionally summarize.

```bash
# Using lightpanda backend (default)
curl -X POST http://localhost:8080/api/scrape \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com", "summarize": true}'

# Using crawl4ai backend
curl -X POST http://localhost:8080/api/scrape \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com", "backend": "crawl4ai", "mode": "markdown"}'
```

Request body fields:

| Field | Type | Description |
|-------|------|-------------|
| `url` | string | **Required.** URL to scrape |
| `backend` | string | `lightpanda` (default) or `crawl4ai` |
| `mode` | string | `markdown` (default) or `raw` |
| `summarize` | bool | Generate AI summary |
| `provider` | string | `ollama`, `gemini`, or `openrouter` |
| `ollama_model` | string | Override Ollama model |
| `gemini_model` | string | Override Gemini model |
| `gemini_base_url` | string | Override Gemini base URL |
| `openrouter_model` | string | Override OpenRouter model |
| `system_prompt` | string | Custom system prompt |
| `user_prompt` | string | Custom user prompt template (use `{{content}}` for page markdown) |

API keys are passed via headers:

| Header | Provider |
|--------|----------|
| `X-Gemini-API-Key` | gemini |
| `X-OpenRouter-API-Key` | openrouter |

Response:

```json
{
  "url": "https://example.com",
  "backend": "lightpanda",
  "html": "<html>...",
  "markdown": "# Example...",
  "summary": "This page is about..."
}
```

**`GET /api/health`** — Health check.

**`GET /`** — Web UI.

## Custom Prompts

Both the CLI (`--system-prompt`, `--user-prompt` via env) and the API support custom prompts. Leave empty to use the built-in defaults.

The user prompt template uses `{{content}}` as the placeholder for page markdown:

```
Extract all product names and prices from:

{{content}}
```

## Docker Compose

Runs three services:

| Service | Description | Port |
|---------|-------------|------|
| `app` | Scraper AI API + Web UI | 8080 |
| `lightpanda` | Headless browser (CDP) | 9222 (internal) |
| `ollama` | Local LLM inference | 11434 (internal) |

```bash
cp .env.example .env
docker compose up -d
docker compose logs -f app
```

After first start, pull an Ollama model:

```bash
docker compose exec ollama ollama pull qwen3.5:0.8b
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Scraper AI                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  CLI / HTTP API                                             │
│       │                                                     │
│       ▼                                                     │
│  ┌─────────────────┐                                        │
│  │ Scraper Factory │                                        │
│  └────────┬────────┘                                        │
│           │                                                 │
│     ┌─────┴─────┐                                           │
│     ▼           ▼                                           │
│ ┌───────────┐ ┌───────────┐                                 │
│ │Lightpanda │ │ Crawl4AI  │   ◄── Backend selection         │
│ │  (CDP)    │ │ (Python)  │                                  │
│ └─────┬─────┘ └─────┬─────┘                                 │
│       │             │                                       │
│       └──────┬──────┘                                       │
│              ▼                                              │
│         HTML / Markdown                                     │
│              │                                              │
│              ▼                                              │
│    ┌─────────────────┐                                      │
│    │   Summarizer    │   ◄── Optional AI summarization      │
│    │ (Ollama/Gemini) │                                       │
│    └─────────────────┘                                      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```
