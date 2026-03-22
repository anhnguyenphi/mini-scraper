package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scraper-ai/scraper-ai/scraper"
	"github.com/scraper-ai/scraper-ai/summarizer"
	"github.com/spf13/cobra"
)

var (
	cdpURL              string
	provider            string
	ollamaURL           string
	ollamaModel         string
	geminiModel         string
	geminiBaseURL       string
	geminiAPIKey        string
	openrouterModel     string
	openrouterAPIKey    string
	timeout             int
	raw                 bool
	backend             string
	pythonPath          string
	crawlScriptPath     string
	crawl4aiServiceURL  string
	crawl4aiAPIKey      string
	crawl4aiCDPURL      string
	crawl4aiBrowserMode string
)

var rootCmd = &cobra.Command{
	Use:   "scraper-ai <url>",
	Short: "Scrape a web page and summarize with Ollama or Gemini",
	Long: `A CLI tool that uses a headless browser to fetch web pages,
converts them to markdown, and optionally summarizes with AI.

Backend:
  lightpanda  - Chrome DevTools Protocol (requires Lightpanda/Chrome)
  crawl4ai    - Python crawl4ai library (requires: pip install crawl4ai)

Mode:
  raw       - Output raw HTML only
  markdown  - Convert HTML to markdown (default)`,
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVar(&backend, "backend", "lightpanda", "Scraper backend: lightpanda or crawl4ai")
	rootCmd.Flags().StringVar(&cdpURL, "cdp", "ws://127.0.0.1:9222", "Lightpanda CDP WebSocket URL")
	rootCmd.Flags().StringVar(&pythonPath, "python", "python3", "Python path for crawl4ai backend")
	rootCmd.Flags().StringVar(&crawlScriptPath, "crawl-script", "scripts/crawl4ai_runner.py", "Path to crawl4ai runner script")
	rootCmd.Flags().StringVar(&crawl4aiServiceURL, "crawl4ai-service", "", "Crawl4AI HTTP server base URL (optional; persistent browser). Env: CRAWL4AI_SERVICE_URL")
	rootCmd.Flags().StringVar(&crawl4aiAPIKey, "crawl4ai-api-key", "", "X-API-Key for crawl4ai HTTP server. Env: CRAWL4AI_API_KEY")
	rootCmd.Flags().StringVar(&crawl4aiCDPURL, "crawl4ai-cdp-url", "", "WebSocket CDP URL for crawl4ai (attach to existing browser). Env: CRAWL4AI_CDP_URL")
	rootCmd.Flags().StringVar(&crawl4aiBrowserMode, "crawl4ai-browser-mode", "", "browser_mode when using CDP (default: custom). Env: CRAWL4AI_BROWSER_MODE")
	rootCmd.Flags().StringVar(&provider, "provider", summarizer.ProviderOllama, "Summarization provider: ollama, gemini, or openrouter")
	rootCmd.Flags().StringVar(&ollamaURL, "ollama", "http://127.0.0.1:11434", "Ollama API base URL")
	rootCmd.Flags().StringVar(&ollamaModel, "model", "qwen3.5:0.8b", "Ollama model to use for summarization")
	rootCmd.Flags().StringVar(&geminiModel, "gemini-model", "gemini-1.5-flash", "Gemini model to use for summarization")
	rootCmd.Flags().StringVar(&geminiBaseURL, "gemini-base-url", "https://generativelanguage.googleapis.com/v1beta", "Gemini API base URL")
	rootCmd.Flags().StringVar(&openrouterModel, "openrouter-model", "google/gemini-2.0-flash-001", "OpenRouter model to use for summarization")
	rootCmd.Flags().IntVar(&timeout, "timeout", 30, "Timeout in seconds for page loading")
	rootCmd.Flags().BoolVar(&raw, "raw", false, "Output raw HTML instead of markdown")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	url := args[0]
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Using backend: %s\n", backend)

	// Create scraper based on backend
	s, err := createScraper()
	if err != nil {
		return fmt.Errorf("create scraper: %w", err)
	}
	defer s.Close()

	// Determine mode
	mode := scraper.ModeMarkdown
	if raw {
		mode = scraper.ModeRaw
	}

	fmt.Fprintf(os.Stderr, "Fetching %s ...\n", url)

	result, err := s.Scrape(ctx, url, mode)
	if err != nil {
		return fmt.Errorf("scrape: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Fetched %d bytes of HTML\n", len(result.HTML))

	// If raw mode, just output HTML
	if raw {
		fmt.Println(result.HTML)
		return nil
	}

	markdown := result.Markdown
	if markdown == "" {
		fmt.Fprintf(os.Stderr, "Warning: no markdown returned\n")
		return nil
	}

	// Truncate markdown if too long (Ollama context limits)
	const maxChars = 50000
	if len(markdown) > maxChars {
		markdown = markdown[:maxChars]
		fmt.Fprintf(os.Stderr, "Truncated markdown to %d chars\n", maxChars)
	}

	switch provider {
	case summarizer.ProviderOllama:
		fmt.Fprintf(os.Stderr, "Summarizing with Ollama model %s ...\n", ollamaModel)
	case summarizer.ProviderGemini:
		geminiAPIKey = os.Getenv("GEMINI_API_KEY")
		if strings.TrimSpace(geminiAPIKey) == "" {
			return fmt.Errorf("provider gemini requires GEMINI_API_KEY environment variable")
		}
		fmt.Fprintf(os.Stderr, "Summarizing with Gemini model %s ...\n", geminiModel)
	case summarizer.ProviderOpenRouter:
		openrouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
		if strings.TrimSpace(openrouterAPIKey) == "" {
			return fmt.Errorf("provider openrouter requires OPENROUTER_API_KEY environment variable")
		}
		fmt.Fprintf(os.Stderr, "Summarizing with OpenRouter model %s ...\n", openrouterModel)
	default:
		return fmt.Errorf("invalid provider %q (supported: %s, %s, %s)", provider, summarizer.ProviderOllama, summarizer.ProviderGemini, summarizer.ProviderOpenRouter)
	}

	summarizerCfg := summarizer.Config{
		Provider:         provider,
		BaseURL:          ollamaURL,
		Model:            ollamaModel,
		GeminiAPIKey:     geminiAPIKey,
		GeminiBaseURL:    geminiBaseURL,
		OpenRouterAPIKey: openrouterAPIKey,
		OpenRouterModel:  openrouterModel,
	}

	if provider == summarizer.ProviderGemini {
		summarizerCfg.Model = geminiModel
	}

	summ := summarizer.New(summarizerCfg)

	summary, err := summ.Summarize(ctx, markdown)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}

	fmt.Println(strings.TrimSpace(summary))
	return nil
}

func createScraper() (scraper.Scraper, error) {
	backendType := scraper.ParseBackend(backend)

	switch backendType {
	case scraper.BackendCrawl4AI:
		svc := strings.TrimSpace(crawl4aiServiceURL)
		if svc == "" {
			svc = strings.TrimSpace(os.Getenv("CRAWL4AI_SERVICE_URL"))
		}
		key := strings.TrimSpace(crawl4aiAPIKey)
		if key == "" {
			key = strings.TrimSpace(os.Getenv("CRAWL4AI_API_KEY"))
		}
		cdp := strings.TrimSpace(crawl4aiCDPURL)
		if cdp == "" {
			cdp = strings.TrimSpace(os.Getenv("CRAWL4AI_CDP_URL"))
		}
		bm := strings.TrimSpace(crawl4aiBrowserMode)
		if bm == "" {
			bm = strings.TrimSpace(os.Getenv("CRAWL4AI_BROWSER_MODE"))
		}
		cfg := scraper.Crawl4AIConfig{
			PythonPath:  pythonPath,
			ScriptPath:  crawlScriptPath,
			Timeout:     time.Duration(timeout) * time.Second,
			ServiceURL:  svc,
			APIKey:      key,
			CDPURL:      cdp,
			BrowserMode: bm,
		}
		return scraper.NewScraper(scraper.BackendCrawl4AI, cfg)

	case scraper.BackendLightpanda:
		cfg := scraper.LightpandaConfig{
			CDPURL:      cdpURL,
			Timeout:     time.Duration(timeout) * time.Second,
			WaitForIdle: true,
		}
		return scraper.NewScraper(scraper.BackendLightpanda, cfg)

	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}
