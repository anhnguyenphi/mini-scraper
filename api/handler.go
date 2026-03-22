package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/scraper-ai/scraper-ai/cache"
	"github.com/scraper-ai/scraper-ai/scraper"
	"github.com/scraper-ai/scraper-ai/summarizer"
)

type Config struct {
	CDPURL          string
	PythonPath      string
	CrawlScriptPath string
	OllamaURL       string
	OllamaModel     string
	GeminiModel     string
	GeminiBaseURL   string
	OpenRouterModel string
}

type Handler struct {
	cfg   Config
	cache *cache.Store
}

func NewHandler(cfg Config, c *cache.Store) *Handler {
	if cfg.PythonPath == "" {
		cfg.PythonPath = "python3"
	}
	if cfg.CrawlScriptPath == "" {
		cfg.CrawlScriptPath = "scripts/crawl4ai_runner.py"
	}
	return &Handler{cfg: cfg, cache: c}
}

type ScrapeRequest struct {
	URL             string `json:"url"`
	Mode            string `json:"mode"`
	Backend         string `json:"backend"`
	Provider        string `json:"provider"`
	OllamaURL       string `json:"ollama_url,omitempty"`
	OllamaModel     string `json:"ollama_model,omitempty"`
	GeminiModel     string `json:"gemini_model,omitempty"`
	GeminiBaseURL   string `json:"gemini_base_url,omitempty"`
	OpenRouterModel string `json:"openrouter_model,omitempty"`
	Summarize       bool   `json:"summarize"`
	SystemPrompt    string `json:"system_prompt,omitempty"`
	UserPrompt      string `json:"user_prompt,omitempty"`
}

type ScrapeResponse struct {
	URL      string `json:"url"`
	Backend  string `json:"backend,omitempty"`
	HTML     string `json:"html,omitempty"`
	Markdown string `json:"markdown,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/scrape", h.handleScrape)
	mux.HandleFunc("GET /api/health", h.handleHealth)
	mux.Handle("/", http.FileServer(http.Dir("static")))
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) handleScrape(w http.ResponseWriter, r *http.Request) {
	var req ScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp := ScrapeResponse{URL: req.URL}

	if entry, ok, err := h.cache.Get(req.URL); err == nil && ok {
		resp.HTML = entry.HTML
		resp.Markdown = entry.Markdown
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cache read failed: %v", err))
		return
	} else {
		s, err := h.createScraper(req.Backend)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid backend: %v", err))
			return
		}
		defer s.Close()

		resp.Backend = req.Backend
		if resp.Backend == "" {
			resp.Backend = "lightpanda"
		}

		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = scraper.ModeMarkdown
		}

		result, err := s.Scrape(ctx, req.URL, mode)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("scrape failed: %v", err))
			return
		}
		resp.HTML = result.HTML
		resp.Markdown = result.Markdown

		if err := h.cache.Set(req.URL, resp.HTML, resp.Markdown); err != nil {
			fmt.Printf("cache write failed: %v\n", err)
		}
	}

	if !req.Summarize {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = summarizer.ProviderOllama
	}

	summCfg := summarizer.Config{
		Provider:      provider,
		BaseURL:       coalesce(req.OllamaURL, h.cfg.OllamaURL),
		GeminiBaseURL: coalesce(req.GeminiBaseURL, h.cfg.GeminiBaseURL),
		SystemPrompt:  req.SystemPrompt,
		UserPrompt:    req.UserPrompt,
	}

	if provider == summarizer.ProviderGemini {
		summCfg.Model = coalesce(req.GeminiModel, h.cfg.GeminiModel)
		summCfg.GeminiAPIKey = r.Header.Get("X-Gemini-API-Key")
		if strings.TrimSpace(summCfg.GeminiAPIKey) == "" {
			writeError(w, http.StatusBadRequest, "X-Gemini-API-Key header required for gemini provider")
			return
		}
	} else if provider == summarizer.ProviderOpenRouter {
		summCfg.OpenRouterModel = coalesce(req.OpenRouterModel, h.cfg.OpenRouterModel)
		summCfg.OpenRouterAPIKey = r.Header.Get("X-OpenRouter-API-Key")
		if strings.TrimSpace(summCfg.OpenRouterAPIKey) == "" {
			writeError(w, http.StatusBadRequest, "X-OpenRouter-API-Key header required for openrouter provider")
			return
		}
	} else {
		summCfg.Model = coalesce(req.OllamaModel, h.cfg.OllamaModel)
	}

	const maxChars = 50000
	markdown := resp.Markdown
	if len(markdown) > maxChars {
		markdown = markdown[:maxChars]
	}

	summ := summarizer.New(summCfg)
	summary, err := summ.Summarize(ctx, markdown)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("summarize failed: %v", err))
		return
	}

	resp.Summary = strings.TrimSpace(summary)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) createScraper(backend string) (scraper.Scraper, error) {
	backendType := scraper.ParseBackend(backend)

	switch backendType {
	case scraper.BackendCrawl4AI:
		cfg := scraper.Crawl4AIConfig{
			PythonPath: h.cfg.PythonPath,
			ScriptPath: h.cfg.CrawlScriptPath,
			Timeout:    300 * time.Second,
		}
		return scraper.NewScraper(scraper.BackendCrawl4AI, cfg)

	case scraper.BackendLightpanda:
		cfg := scraper.LightpandaConfig{
			CDPURL:      h.cfg.CDPURL,
			Timeout:     300 * time.Second,
			WaitForIdle: true,
		}
		return scraper.NewScraper(scraper.BackendLightpanda, cfg)

	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ScrapeResponse{Error: msg})
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
