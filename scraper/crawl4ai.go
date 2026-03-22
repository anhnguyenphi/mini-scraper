package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

// Crawl4AIScraper uses crawl4ai via subprocess or persistent HTTP service.
type Crawl4AIScraper struct {
	cfg Crawl4AIConfig
}

// crawl4AIResponse represents the JSON output from crawl4ai_runner.py.
type crawl4AIResponse struct {
	HTML     string `json:"html"`
	Markdown string `json:"markdown,omitempty"`
	Error    string `json:"error,omitempty"`
}

// crawl4AIHTTPRequest is the JSON body for POST /scrape on the crawl4ai serve endpoint.
type crawl4AIHTTPRequest struct {
	URL                   string   `json:"url"`
	Markdown              bool     `json:"markdown"`
	Timeout               int      `json:"timeout"`
	TextMode              bool     `json:"text_mode"`
	LightMode             bool     `json:"light_mode"`
	CacheMode             string   `json:"cache_mode"`
	WaitUntil             string   `json:"wait_until"`
	DelayBeforeReturnHTML float64  `json:"delay_before_return_html"`
	ExtraArgs             []string `json:"extra_args,omitempty"`
	CDPURL                string   `json:"cdp_url,omitempty"`
	BrowserMode           string   `json:"browser_mode,omitempty"`
}

func resolveBoolPtr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func mergeCrawl4AI(cfg Crawl4AIConfig) Crawl4AIConfig {
	d := DefaultCrawl4AIConfig()
	if cfg.Timeout == 0 {
		cfg.Timeout = d.Timeout
	}
	if cfg.PythonPath == "" {
		cfg.PythonPath = d.PythonPath
	}
	if cfg.ScriptPath == "" {
		cfg.ScriptPath = d.ScriptPath
	}
	if cfg.CacheMode == "" {
		cfg.CacheMode = d.CacheMode
	}
	if cfg.WaitUntil == "" {
		cfg.WaitUntil = d.WaitUntil
	}
	if cfg.BrowserMode == "" {
		cfg.BrowserMode = d.BrowserMode
	}
	return cfg
}

// NewCrawl4AIScraper creates a new Crawl4AIScraper.
func NewCrawl4AIScraper(cfg Crawl4AIConfig) *Crawl4AIScraper {
	cfg = mergeCrawl4AI(cfg)
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

	if strings.TrimSpace(s.cfg.ServiceURL) != "" {
		return s.scrapeHTTP(ctx, url, mode)
	}
	return s.scrapeSubprocess(ctx, url, mode)
}

func (s *Crawl4AIScraper) scrapeHTTP(ctx context.Context, url string, mode string) (*ScrapeResult, error) {
	tm := resolveBoolPtr(s.cfg.TextMode, true)
	lm := resolveBoolPtr(s.cfg.LightMode, true)

	body := crawl4AIHTTPRequest{
		URL:                   url,
		Markdown:              mode == ModeMarkdown,
		Timeout:               int(s.cfg.Timeout.Seconds()),
		TextMode:              tm,
		LightMode:             lm,
		CacheMode:             s.cfg.CacheMode,
		WaitUntil:             s.cfg.WaitUntil,
		DelayBeforeReturnHTML: s.cfg.DelayBeforeHTML,
		ExtraArgs:             s.cfg.ExtraArgs,
		CDPURL:                s.cfg.CDPURL,
		BrowserMode:           s.cfg.BrowserMode,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal crawl4ai request: %w", err)
	}

	base := strings.TrimRight(strings.TrimSpace(s.cfg.ServiceURL), "/")
	reqURL := base + "/scrape"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := strings.TrimSpace(s.cfg.APIKey); k != "" {
		req.Header.Set("X-API-Key", k)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crawl4ai HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read crawl4ai response: %w", err)
	}

	jsonBytes := extractLastJSONObject(respBody)
	var crawlResp crawl4AIResponse
	if err := json.Unmarshal(jsonBytes, &crawlResp); err != nil {
		return nil, fmt.Errorf("failed to parse crawl4ai HTTP response: %w", err)
	}

	if resp.StatusCode >= 400 && crawlResp.Error == "" {
		return nil, fmt.Errorf("crawl4ai HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if crawlResp.Error != "" {
		return nil, fmt.Errorf("crawl4ai error: %s", crawlResp.Error)
	}

	return s.buildScrapeResult(&crawlResp)
}

func (s *Crawl4AIScraper) scrapeSubprocess(ctx context.Context, url string, mode string) (*ScrapeResult, error) {
	args := []string{
		s.cfg.ScriptPath,
		"--url", url,
		"--timeout", fmt.Sprintf("%.0f", s.cfg.Timeout.Seconds()),
	}

	tm := resolveBoolPtr(s.cfg.TextMode, true)
	lm := resolveBoolPtr(s.cfg.LightMode, true)
	if tm {
		args = append(args, "--text-mode")
	} else {
		args = append(args, "--no-text-mode")
	}
	if lm {
		args = append(args, "--light-mode")
	} else {
		args = append(args, "--no-light-mode")
	}

	args = append(args, "--cache-mode", s.cfg.CacheMode)
	args = append(args, "--wait-until", s.cfg.WaitUntil)
	args = append(args, "--delay-before-return-html", fmt.Sprintf("%g", s.cfg.DelayBeforeHTML))

	for _, ex := range s.cfg.ExtraArgs {
		args = append(args, "--extra-arg", ex)
	}
	if cu := strings.TrimSpace(s.cfg.CDPURL); cu != "" {
		args = append(args, "--cdp-url", cu)
		args = append(args, "--browser-mode", s.cfg.BrowserMode)
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

	return s.buildScrapeResult(&resp)
}

func (s *Crawl4AIScraper) buildScrapeResult(resp *crawl4AIResponse) (*ScrapeResult, error) {
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
