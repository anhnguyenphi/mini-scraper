# Mini Scraper
Building LLM web scraper with headless browser to read web, convert to markdown, summarize

## Prerequisites:
- Go 1.26+
- Python 3.10+ with MarkItDown CLI installed:
      python3 -m pip install "markitdown[all]"
- One summarization provider:
  - Ollama installed locally, or
  - Gemini API key (`GEMINI_API_KEY`) for cloud inference

## How to use

1. Start Lightpanda (pick one) or Chomedp:
```
Binary
$ ./lightpanda serve --host 127.0.0.1 --port 9222
Or Docker
$ docker run -d --name lightpanda -p 9222:9222 lightpanda/browser:nightly
chromedp
$ docker run -d -p 9222:9222 --rm --name headless-shell chromedp/headless-shell
```
   
2. Verify MarkItDown CLI is available:
```
$ markitdown --help
If this command is missing, install with:
$ python3 -m pip install "markitdown[all]"
```

3. Start Ollama (with a model pulled), if using provider `ollama`:
      ollama pull llama3.2
   ollama serve
   
4. Run the scraper:
```
Get AI summary
$ go run . https://example.com
Output raw markdown instead of summary
go run . --raw https://example.com
Use a different Ollama model
go run . --model llama3.1 https://example.com
Use Gemini provider (requires GEMINI_API_KEY)
GEMINI_API_KEY=your_key go run . --provider gemini --gemini-model gemini-2.5-flash https://example.com
```
   
Flags:
- --cdp — Lightpanda CDP URL (default ws://127.0.0.1:9222)
- --provider — Summarization provider: ollama|gemini (default ollama)
- --ollama — Ollama API URL (default http://127.0.0.1:11434)
- --model — Ollama model (default qwen3.5:0.8b)
- --gemini-model — Gemini model (default gemini-2.5-flash)
- --gemini-base-url — Gemini API base URL (default https://generativelanguage.googleapis.com/v1beta)
- --timeout — Page load timeout in seconds (default 30)
- --raw — Print markdown instead of summarizing

Environment variables:
- GEMINI_API_KEY — required when `--provider gemini`
