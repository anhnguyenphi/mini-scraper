package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// LightpandaScraper connects to Lightpanda via CDP and fetches page HTML.
type LightpandaScraper struct {
	cfg LightpandaConfig
}

// NewLightpandaScraper creates a new LightpandaScraper.
func NewLightpandaScraper(cfg LightpandaConfig) *LightpandaScraper {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.CDPURL == "" {
		cfg.CDPURL = "ws://127.0.0.1:9222"
	}
	return &LightpandaScraper{cfg: cfg}
}

// Fetch connects to the CDP server, navigates to url, and returns the rendered HTML.
func (s *LightpandaScraper) Fetch(ctx context.Context, url string) (string, error) {
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

// Scrape fetches the page and optionally converts to markdown.
func (s *LightpandaScraper) Scrape(ctx context.Context, url string, mode string) (*ScrapeResult, error) {
	html, err := s.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}

	result := &ScrapeResult{HTML: html}

	if mode == ModeMarkdown {
		markdown, err := htmlToMarkdown(html)
		if err != nil {
			return nil, fmt.Errorf("convert to markdown: %w", err)
		}
		result.Markdown = markdown
	}

	return result, nil
}

// Close is a no-op for LightpandaScraper.
func (s *LightpandaScraper) Close() error {
	return nil
}

// htmlToMarkdown converts raw HTML to clean markdown.
func htmlToMarkdown(html string) (string, error) {
	cmd := exec.Command("markitdown")
	cmd.Stdin = strings.NewReader(html)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return "", fmt.Errorf("markitdown CLI not found; install with `pip install 'markitdown[all]'` and ensure it is on PATH")
		}
		if stderr.Len() > 0 {
			return "", fmt.Errorf("markitdown failed: %s", strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("markitdown failed: %w", err)
	}

	markdown := stdout.String()

	lines := strings.Split(markdown, "\n")
	var cleaned []string
	prevBlank := false
	for _, line := range lines {
		isBlank := strings.TrimSpace(line) == ""
		if isBlank && prevBlank {
			continue
		}
		cleaned = append(cleaned, line)
		prevBlank = isBlank
	}

	result := strings.TrimSpace(strings.Join(cleaned, "\n"))
	if result == "" {
		return "", fmt.Errorf("markitdown returned empty output")
	}

	seoHeader := extractSEOHeader(html)
	if seoHeader != "" {
		result = seoHeader + "\n\n" + result
	}

	return result, nil
}

var headRegex = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`)
var titleRegex = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
var linkTagRegex = regexp.MustCompile(`(?is)<link\b[^>]*>`)
var metaTagRegex = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
var attrRegex = regexp.MustCompile(`(?is)([a-zA-Z_:][a-zA-Z0-9:_.-]*)\s*=\s*("[^"]*"|'[^']*')`)

func extractSEOHeader(pageHTML string) string {
	head := headRegex.FindString(pageHTML)
	if strings.TrimSpace(head) == "" {
		return ""
	}

	var lines []string

	if titleMatch := titleRegex.FindStringSubmatch(head); len(titleMatch) > 1 {
		title := strings.TrimSpace(html.UnescapeString(titleMatch[1]))
		if title != "" {
			lines = append(lines, "- title: "+title)
		}
	}

	for _, linkTag := range linkTagRegex.FindAllString(head, -1) {
		rel := strings.ToLower(getAttr(linkTag, "rel"))
		if rel == "" {
			continue
		}

		relParts := strings.Fields(rel)
		for _, part := range relParts {
			if part == "canonical" {
				if canonicalURL := getAttr(linkTag, "href"); canonicalURL != "" {
					lines = append(lines, "- canonical: "+canonicalURL)
				}
				break
			}
		}
	}

	allowedMetaNames := map[string]struct{}{
		"description":         {},
		"keywords":            {},
		"robots":              {},
		"googlebot":           {},
		"author":              {},
		"viewport":            {},
		"twitter:card":        {},
		"twitter:title":       {},
		"twitter:description": {},
		"twitter:image":       {},
	}

	var metaLines []string
	for _, tag := range metaTagRegex.FindAllString(head, -1) {
		name := strings.ToLower(getAttr(tag, "name"))
		property := strings.ToLower(getAttr(tag, "property"))
		content := getAttr(tag, "content")
		if content == "" {
			continue
		}

		switch {
		case strings.HasPrefix(property, "og:"):
			metaLines = append(metaLines, "- "+property+": "+content)
		case name != "":
			if _, ok := allowedMetaNames[name]; ok {
				metaLines = append(metaLines, "- "+name+": "+content)
			}
		}
	}

	sort.Strings(metaLines)
	lines = append(lines, metaLines...)

	if len(lines) == 0 {
		return ""
	}

	return "## SEO Header\n\n" + strings.Join(lines, "\n")
}

func getAttr(tag string, attrName string) string {
	for _, match := range attrRegex.FindAllStringSubmatch(tag, -1) {
		if len(match) < 3 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(match[1]), attrName) {
			value := strings.TrimSpace(match[2])
			value = strings.Trim(value, `"'`)
			return strings.TrimSpace(html.UnescapeString(value))
		}
	}
	return ""
}
