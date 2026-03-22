package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/scraper-ai/scraper-ai/api"
	"github.com/scraper-ai/scraper-ai/cache"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP API server",
	Long:  "Starts a REST API server that exposes scrape and summarize functionality over HTTP.",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().String("listen", ":8080", "Address to listen on")
	serveCmd.Flags().String("db", "scraper-ai.db", "SQLite database path for caching")
	serveCmd.Flags().String("cdp", "ws://127.0.0.1:9222", "Lightpanda CDP WebSocket URL")
	serveCmd.Flags().String("python", "python3", "Python path for crawl4ai backend")
	serveCmd.Flags().String("crawl-script", "scripts/crawl4ai_runner.py", "Path to crawl4ai runner script")
	serveCmd.Flags().String("ollama", "http://127.0.0.1:11434", "Ollama API base URL")
	serveCmd.Flags().String("model", "qwen3.5:0.8b", "Ollama model to use")
	serveCmd.Flags().String("gemini-model", "gemini-1.5-flash", "Gemini model to use")
	serveCmd.Flags().String("gemini-base-url", "https://generativelanguage.googleapis.com/v1beta", "Gemini API base URL")
	serveCmd.Flags().String("openrouter-model", "google/gemini-2.0-flash-001", "OpenRouter model to use")
	rootCmd.AddCommand(serveCmd)
}

func flagOrEnv(cmd *cobra.Command, flag, envKey, fallback string) string {
	if cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetString(flag)
		return v
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fallback
}

func runServe(cmd *cobra.Command, args []string) error {
	godotenv.Load()

	cfg := api.Config{
		CDPURL:          flagOrEnv(cmd, "cdp", "CDP_URL", "ws://127.0.0.1:9222"),
		PythonPath:      flagOrEnv(cmd, "python", "PYTHON_PATH", "python3"),
		CrawlScriptPath: flagOrEnv(cmd, "crawl-script", "CRAWL_SCRIPT_PATH", "scripts/crawl4ai_runner.py"),
		OllamaURL:       flagOrEnv(cmd, "ollama", "OLLAMA_URL", "http://127.0.0.1:11434"),
		OllamaModel:     flagOrEnv(cmd, "model", "OLLAMA_MODEL", "qwen3.5:0.8b"),
		GeminiModel:     flagOrEnv(cmd, "gemini-model", "GEMINI_MODEL", "gemini-1.5-flash"),
		GeminiBaseURL:   flagOrEnv(cmd, "gemini-base-url", "GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		OpenRouterModel: flagOrEnv(cmd, "openrouter-model", "OPENROUTER_MODEL", "google/gemini-2.0-flash-001"),
	}

	listen := flagOrEnv(cmd, "listen", "LISTEN", ":8080")
	dbPath := flagOrEnv(cmd, "db", "DB_PATH", "scraper-ai.db")

	store, err := cache.New(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open cache: %w", err)
	}
	defer store.Close()

	handler := api.NewHandler(cfg, store)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	fmt.Fprintf(os.Stderr, "Listening on %s\n", listen)
	fmt.Fprintf(os.Stderr, "API endpoints:\n")
	fmt.Fprintf(os.Stderr, "  POST /api/scrape   - Scrape and optionally summarize a URL\n")
	fmt.Fprintf(os.Stderr, "  GET  /api/health    - Health check\n")
	fmt.Fprintf(os.Stderr, "  GET  /              - Web UI\n")

	return http.ListenAndServe(listen, mux)
}
