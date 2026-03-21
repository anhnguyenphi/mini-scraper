package scraper

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// Config holds scraper configuration.
type Config struct {
	CDPURL      string
	Timeout     time.Duration
	WaitForIdle bool
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		CDPURL:      "ws://127.0.0.1:9222",
		Timeout:     30 * time.Second,
		WaitForIdle: true,
	}
}

// Scraper connects to Lightpanda via CDP and fetches page HTML.
type Scraper struct {
	cfg Config
}

// New creates a new Scraper.
func New(cfg Config) *Scraper {
	return &Scraper{cfg: cfg}
}

// Fetch connects to the CDP server, navigates to url, and returns the rendered HTML.
func (s *Scraper) Fetch(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, s.cfg.CDPURL)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	var html string
	tasks := chromedp.Tasks{
		chromedp.Navigate(url),
	}

	if s.cfg.WaitForIdle {
		tasks = append(tasks, chromedp.WaitReady("body"))
	}

	tasks = append(tasks, chromedp.OuterHTML("html", &html))

	if err := chromedp.Run(browserCtx, tasks); err != nil {
		return "", fmt.Errorf("scrape %s: %w", url, err)
	}

	return html, nil
}
