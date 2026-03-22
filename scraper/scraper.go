package scraper

import (
	"context"
	"fmt"
	"time"
)

const (
	ModeRaw      = "raw"
	ModeMarkdown = "markdown"
)

type BackendType string

const (
	BackendLightpanda BackendType = "lightpanda"
	BackendCrawl4AI   BackendType = "crawl4ai"
)

// Scraper is the interface that all scraper backends must implement.
type Scraper interface {
	// Fetch retrieves the rendered HTML from a URL.
	Fetch(ctx context.Context, url string) (string, error)

	// Scrape fetches the page and optionally converts to markdown.
	Scrape(ctx context.Context, url string, mode string) (*ScrapeResult, error)

	// Close cleans up resources.
	Close() error
}

// ScrapeResult contains the scraped content.
type ScrapeResult struct {
	HTML     string
	Markdown string
}

// LightpandaConfig holds configuration for the Lightpanda/CDP scraper.
type LightpandaConfig struct {
	CDPURL      string
	Timeout     time.Duration
	WaitForIdle bool
}

// Crawl4AIConfig holds configuration for the Crawl4AI scraper.
type Crawl4AIConfig struct {
	PythonPath string
	ScriptPath string
	Timeout    time.Duration
}

// DefaultLightpandaConfig returns sensible defaults for Lightpanda.
func DefaultLightpandaConfig() LightpandaConfig {
	return LightpandaConfig{
		CDPURL:      "ws://127.0.0.1:9222",
		Timeout:     30 * time.Second,
		WaitForIdle: true,
	}
}

// DefaultCrawl4AIConfig returns sensible defaults for Crawl4AI.
func DefaultCrawl4AIConfig() Crawl4AIConfig {
	return Crawl4AIConfig{
		PythonPath: "python3",
		ScriptPath: "scripts/crawl4ai_runner.py",
		Timeout:    60 * time.Second,
	}
}

// NewScraper creates a new Scraper based on the backend type.
func NewScraper(backend BackendType, config interface{}) (Scraper, error) {
	switch backend {
	case BackendLightpanda:
		cfg, ok := config.(LightpandaConfig)
		if !ok {
			cfg = DefaultLightpandaConfig()
		}
		return NewLightpandaScraper(cfg), nil
	case BackendCrawl4AI:
		cfg, ok := config.(Crawl4AIConfig)
		if !ok {
			cfg = DefaultCrawl4AIConfig()
		}
		return NewCrawl4AIScraper(cfg), nil
	default:
		return nil, fmt.Errorf("unknown scraper backend: %s", backend)
	}
}

// ParseBackend parses a backend type string.
func ParseBackend(s string) BackendType {
	switch BackendType(s) {
	case BackendCrawl4AI:
		return BackendCrawl4AI
	default:
		return BackendLightpanda
	}
}
