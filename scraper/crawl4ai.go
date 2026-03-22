package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Crawl4AIScraper uses crawl4ai via subprocess.
type Crawl4AIScraper struct {
	cfg Crawl4AIConfig
}

// crawl4AIResponse represents the JSON output from crawl4ai_runner.py.
type crawl4AIResponse struct {
	HTML     string `json:"html"`
	Markdown string `json:"markdown,omitempty"`
	Error    string `json:"error,omitempty"`
}

// NewCrawl4AIScraper creates a new Crawl4AIScraper.
func NewCrawl4AIScraper(cfg Crawl4AIConfig) *Crawl4AIScraper {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.PythonPath == "" {
		cfg.PythonPath = "python3"
	}
	if cfg.ScriptPath == "" {
		cfg.ScriptPath = "scripts/crawl4ai_runner.py"
	}
	return &Crawl4AIScraper{cfg: cfg}
}

// Fetch retrieves the rendered HTML from a URL using crawl4ai.
func (s *Crawl4AIScraper) Fetch(ctx context.Context, url string) (string, error) {
	result, err := s.scrape(ctx, url, ModeRaw)
	if err != nil {
		return "", err
	}
	return result.HTML, nil
}

// Scrape fetches the page and optionally converts to markdown.
func (s *Crawl4AIScraper) Scrape(ctx context.Context, url string, mode string) (*ScrapeResult, error) {
	return s.scrape(ctx, url, mode)
}

func (s *Crawl4AIScraper) scrape(ctx context.Context, url string, mode string) (*ScrapeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	args := []string{
		s.cfg.ScriptPath,
		"--url", url,
		"--timeout", fmt.Sprintf("%.0f", s.cfg.Timeout.Seconds()),
	}

	if mode == ModeMarkdown {
		args = append(args, "--markdown")
	}

	cmd := exec.CommandContext(ctx, s.cfg.PythonPath, args...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("crawl4ai failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("crawl4ai execution failed: %w", err)
	}

	jsonBytes := extractLastJSONObject(output)
	var resp crawl4AIResponse
	if err := json.Unmarshal(jsonBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse crawl4ai output: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("crawl4ai error: %s", resp.Error)
	}

	markdown := resp.Markdown
	if markdown != "" {
		if seoHeader := extractSEOHeader(resp.HTML); seoHeader != "" {
			markdown = seoHeader + "\n\n" + markdown
		}
	}

	return &ScrapeResult{
		HTML:     resp.HTML,
		Markdown: markdown,
	}, nil
}

// extractLastJSONObject returns the last line that looks like a JSON object.
// Crawl4AI may print progress lines to stdout before the script's JSON line.
func extractLastJSONObject(output []byte) []byte {
	output = bytes.TrimSpace(output)
	lines := bytes.Split(output, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) > 0 && line[0] == '{' {
			return line
		}
	}
	return output
}

// Close is a no-op for Crawl4AIScraper.
func (s *Crawl4AIScraper) Close() error {
	return nil
}
