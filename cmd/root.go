package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scraper-ai/scraper-ai/converter"
	"github.com/scraper-ai/scraper-ai/scraper"
	"github.com/scraper-ai/scraper-ai/summarizer"
	"github.com/spf13/cobra"
)

var (
	cdpURL        string
	provider      string
	ollamaURL     string
	ollamaModel   string
	geminiModel   string
	geminiBaseURL string
	geminiAPIKey  string
	timeout       int
	raw           bool
)

var rootCmd = &cobra.Command{
	Use:   "scraper-ai <url>",
	Short: "Scrape a web page and summarize with Ollama or Gemini",
	Long:  "A CLI tool that uses Lightpanda (headless browser) to fetch web pages, converts them to markdown, and summarizes with Ollama or Gemini.",
	Args:  cobra.ExactArgs(1),
	RunE:  run,
}

func init() {
	rootCmd.Flags().StringVar(&cdpURL, "cdp", "ws://127.0.0.1:9222", "Lightpanda CDP WebSocket URL")
	rootCmd.Flags().StringVar(&provider, "provider", summarizer.ProviderOllama, "Summarization provider: ollama or gemini")
	rootCmd.Flags().StringVar(&ollamaURL, "ollama", "http://127.0.0.1:11434", "Ollama API base URL")
	rootCmd.Flags().StringVar(&ollamaModel, "model", "qwen3.5:0.8b", "Ollama model to use for summarization")
	rootCmd.Flags().StringVar(&geminiModel, "gemini-model", "gemini-1.5-flash", "Gemini model to use for summarization")
	rootCmd.Flags().StringVar(&geminiBaseURL, "gemini-base-url", "https://generativelanguage.googleapis.com/v1beta", "Gemini API base URL")
	rootCmd.Flags().IntVar(&timeout, "timeout", 30, "Timeout in seconds for page loading")
	rootCmd.Flags().BoolVar(&raw, "raw", false, "Output raw markdown instead of summary")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	url := args[0]
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Fetching %s ...\n", url)

	// Step 1: Fetch page via Lightpanda CDP
	scraperCfg := scraper.Config{
		CDPURL:  cdpURL,
		Timeout: time.Duration(timeout) * time.Second,
	}
	s := scraper.New(scraperCfg)

	html, err := s.Fetch(ctx, url)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Fetched %d bytes of HTML\n", len(html))

	// Step 2: Convert to markdown
	markdown, err := converter.HTMLToMarkdown(html)
	if err != nil {
		return fmt.Errorf("convert to markdown: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Converted to %d chars of markdown\n", len(markdown))

	// If --raw flag, just output markdown
	if raw {
		fmt.Println(markdown)
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
	default:
		return fmt.Errorf("invalid provider %q (supported: %s, %s)", provider, summarizer.ProviderOllama, summarizer.ProviderGemini)
	}

	summarizerCfg := summarizer.Config{
		Provider:      provider,
		BaseURL:       ollamaURL,
		Model:         ollamaModel,
		GeminiAPIKey:  geminiAPIKey,
		GeminiBaseURL: geminiBaseURL,
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
