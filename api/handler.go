package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/scraper-ai/scraper-ai/converter"
	"github.com/scraper-ai/scraper-ai/scraper"
	"github.com/scraper-ai/scraper-ai/summarizer"
)

type Config struct {
	CDPURL          string
	OllamaURL       string
	OllamaModel     string
	GeminiModel     string
	GeminiBaseURL   string
	OpenRouterModel string
}

type Handler struct {
	cfg Config
}

func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

type ScrapeRequest struct {
	URL             string `json:"url"`
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

	scraperCfg := scraper.Config{
		CDPURL:  h.cfg.CDPURL,
		Timeout: 30 * time.Second,
	}
	s := scraper.New(scraperCfg)

	html, err := s.Fetch(ctx, req.URL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("fetch failed: %v", err))
		return
	}
	resp.HTML = html

	markdown, err := converter.HTMLToMarkdown(html)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("convert failed: %v", err))
		return
	}
	resp.Markdown = markdown

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
