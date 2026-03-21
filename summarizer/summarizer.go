package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	ProviderOllama = "ollama"
	ProviderGemini = "gemini"
)

// Config holds summarization provider configuration.
type Config struct {
	Provider      string
	BaseURL       string
	Model         string
	GeminiAPIKey  string
	GeminiBaseURL string
	SystemPrompt  string
	UserPrompt    string
}

// DefaultConfig returns sensible defaults for Ollama.
func DefaultConfig() Config {
	return Config{
		Provider: ProviderOllama,
		BaseURL:  "http://127.0.0.1:11434",
		Model:    "qwen3.5:0.8b",
	}
}

// Summarizer sends content to the configured provider for summarization.
type Summarizer struct {
	cfg    Config
	client *http.Client
}

// New creates a new Summarizer.
func New(cfg Config) *Summarizer {
	return &Summarizer{
		cfg:    cfg,
		client: &http.Client{},
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	System string `json:"system"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type geminiRequest struct {
	SystemInstruction geminiSystemInstruction `json:"system_instruction"`
	Contents          []geminiContent         `json:"contents"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

const defaultSystemPrompt = `You are a helpful assistant that summarizes online shopping product pages.
	Start with shop overview then focus on the brand identity, the price range (budget, mid-range, or luxury),
	the primary product categories, payment options, shipping options under 300 words.
	Do not include any other text than the summary.`

const defaultUserPromptTemplate = `Please summarize the following web page content:

{{content}}`

func (s *Summarizer) buildSystemPrompt() string {
	if strings.TrimSpace(s.cfg.SystemPrompt) != "" {
		return s.cfg.SystemPrompt
	}
	return defaultSystemPrompt
}

func (s *Summarizer) buildUserPrompt(markdown string) string {
	template := s.cfg.UserPrompt
	if strings.TrimSpace(template) == "" {
		template = defaultUserPromptTemplate
	}
	return strings.ReplaceAll(template, "{{content}}", markdown)
}

// Summarize takes markdown content and returns a summary from the configured provider.
func (s *Summarizer) Summarize(ctx context.Context, markdown string) (string, error) {
	provider := strings.TrimSpace(strings.ToLower(s.cfg.Provider))
	if provider == "" {
		provider = ProviderOllama
	}

	switch provider {
	case ProviderOllama:
		return s.summarizeWithOllama(ctx, markdown)
	case ProviderGemini:
		return s.summarizeWithGemini(ctx, markdown)
	default:
		return "", fmt.Errorf("unsupported provider: %q (supported: %s, %s)", s.cfg.Provider, ProviderOllama, ProviderGemini)
	}
}

func (s *Summarizer) summarizeWithOllama(ctx context.Context, markdown string) (string, error) {
	reqBody := ollamaRequest{
		Model:  s.cfg.Model,
		Prompt: s.buildUserPrompt(markdown),
		Stream: false,
		System: s.buildSystemPrompt(),
	}

	baseURL := strings.TrimRight(s.cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultConfig().BaseURL
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return ollamaResp.Response, nil
}

func (s *Summarizer) summarizeWithGemini(ctx context.Context, markdown string) (string, error) {
	apiKey := strings.TrimSpace(s.cfg.GeminiAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("missing Gemini API key")
	}

	baseURL := strings.TrimRight(s.cfg.GeminiBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}

	model := strings.TrimSpace(s.cfg.Model)
	if model == "" {
		model = "gemini-2.5-flash"
	}

	reqBody := geminiRequest{
		SystemInstruction: geminiSystemInstruction{
			Parts: []geminiPart{
				{Text: s.buildSystemPrompt()},
			},
		},
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: s.buildUserPrompt(markdown)},
				},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, model, apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if gemResp.Error != nil && strings.TrimSpace(gemResp.Error.Message) != "" {
			return "", fmt.Errorf("gemini error (status %d): %s", resp.StatusCode, gemResp.Error.Message)
		}
		return "", fmt.Errorf("gemini error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}

	text := strings.TrimSpace(gemResp.Candidates[0].Content.Parts[0].Text)
	if text == "" {
		return "", fmt.Errorf("gemini returned empty summary")
	}

	return text, nil
}
