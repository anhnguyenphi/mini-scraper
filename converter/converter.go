package converter

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os/exec"
	"sort"
	"regexp"
	"strings"
)

// HTMLToMarkdown converts raw HTML to clean markdown.
func HTMLToMarkdown(html string) (string, error) {
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
		"description":       {},
		"keywords":          {},
		"robots":            {},
		"googlebot":         {},
		"author":            {},
		"viewport":          {},
		"twitter:card":      {},
		"twitter:title":     {},
		"twitter:description": {},
		"twitter:image":     {},
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
