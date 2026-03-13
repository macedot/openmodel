package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/macedot/openmodel/internal/api/anthropic"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/endpoints"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server/converters"
)

// benchResult holds the result of a single benchmark run
type benchResult struct {
	Type         string       `json:"type"`
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	ProviderID   string       `json:"provider_id,omitempty"`
	Strategy     string       `json:"strategy,omitempty"`
	ApiMode      string       `json:"api_mode"`
	URL          string       `json:"url"`
	Endpoint     string       `json:"endpoint"`
	StatusCode   int          `json:"status_code"`
	Prompt       string       `json:"prompt"`
	Response     string       `json:"response,omitempty"`
	Error        string       `json:"error,omitempty"`
	Duration     string       `json:"duration"`
	Stream       bool         `json:"stream"`
	Tokens       *benchTokens `json:"tokens,omitempty"`
	TokensPerSec float64      `json:"tokens_per_sec,omitempty"`
}

type benchTokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type benchResponse struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// writeBenchResult writes a benchmark result to a JSON file
func writeBenchResult(result benchResult) {
	sanitizedProvider := sanitizeBenchName(result.Provider)
	sanitizedModel := sanitizeBenchName(result.Model)
	sanitizedEndpoint := sanitizeBenchName(result.Endpoint)

	filename := fmt.Sprintf("bench-%d-%s-%s-%s.json", time.Now().UnixNano(), sanitizedProvider, sanitizedModel, sanitizedEndpoint)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling benchmark result: %v\n", err)
		return
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing benchmark file: %v\n", err)
	}
}

func sanitizeBenchName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		return "unknown"
	}
	return result.String()
}

func parseStatusCodeFromError(errStr string) int {
	re := regexp.MustCompile(`status (\d+)`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) > 1 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code
		}
	}
	return 0
}

func runBenchApplication(ctx context.Context, cfg *config.Config, providers benchProviderMap, messages []openai.ChatCompletionMessage, stream bool, modelFilter string) {
	for _, modelName := range cfg.ModelOrder {
		if modelFilter != "" && modelFilter != modelName {
			continue
		}

		modelConfig, exists := cfg.Models[modelName]
		if !exists {
			continue
		}

		prov, providerKey, providerModel, err := findFirstAvailableProvider(providers, modelConfig)
		if err != nil {
			startTime := time.Now()
			writeBenchResult(benchResult{
				Type:       "error",
				Provider:   modelName,
				Model:      modelName,
				Strategy:   modelConfig.Strategy,
				StatusCode: parseStatusCodeFromError(err.Error()),
				Prompt:     strings.TrimSpace(messages[0].Content),
				Error:      err.Error(),
				Duration:   time.Since(startTime).String(),
				Stream:     stream,
			})
			continue
		}

		apiMode := prov.APIMode()
		baseURL := prov.BaseURL()

		for _, endpoint := range getEndpointsForAPIMode(apiMode) {
			startTime := time.Now()

			logger.Info("Benchmarking model",
				"model", modelName,
				"provider", providerKey,
				"url", baseURL,
				"endpoint", endpoint,
				"stream", stream)

			resp, benchErr := runBenchEndpoint(ctx, prov, endpoint, providerModel, messages, stream)
			writeBenchResponse(benchResult{
				Provider:   modelName,
				Model:      modelName,
				ProviderID: providerKey,
				Strategy:   modelConfig.Strategy,
				ApiMode:    apiMode,
				URL:        baseURL + endpoint,
				Endpoint:   endpoint,
				Prompt:     strings.TrimSpace(messages[0].Content),
				Duration:   time.Since(startTime).String(),
				Stream:     stream,
			}, resp, benchErr)
		}
	}
}

func getEndpointsForAPIMode(apiMode string) []string {
	switch apiMode {
	case "openai":
		return []string{endpoints.V1ChatCompletions}
	case "anthropic":
		return []string{endpoints.V1Messages}
	default:
		return []string{endpoints.V1ChatCompletions, endpoints.V1Messages}
	}
}

func runBenchEndpoint(ctx context.Context, prov benchProvider, endpoint, model string, messages []openai.ChatCompletionMessage, stream bool) (*benchResponse, error) {
	if endpoint == endpoints.V1ChatCompletions {
		return benchChat(ctx, prov, model, messages, stream)
	}
	return benchAnthropicEndpoint(ctx, prov, model, messages, stream)
}

func writeBenchResponse(base benchResult, resp *benchResponse, benchErr error) {
	if benchErr != nil {
		base.Type = "error"
		base.StatusCode = parseStatusCodeFromError(benchErr.Error())
		base.Error = benchErr.Error()
		writeBenchResult(base)
		return
	}

	base.Type = "response"
	base.StatusCode = http.StatusOK
	base.Response = resp.Content
	if resp.TotalTokens > 0 {
		base.Tokens = &benchTokens{
			Prompt:     resp.PromptTokens,
			Completion: resp.CompletionTokens,
			Total:      resp.TotalTokens,
		}
		if resp.PromptTokens > 0 {
			duration, err := time.ParseDuration(base.Duration)
			if err == nil && duration > 0 {
				base.TokensPerSec = float64(resp.CompletionTokens) / duration.Seconds()
			}
		}
	}
	writeBenchResult(base)
}

func benchChat(ctx context.Context, prov benchProvider, model string, messages []openai.ChatCompletionMessage, stream bool) (*benchResponse, error) {
	if stream {
		return benchChatStream(ctx, prov, model, messages)
	}
	return benchChatNonStream(ctx, prov, model, messages)
}

func benchChatNonStream(ctx context.Context, prov benchProvider, model string, messages []openai.ChatCompletionMessage) (*benchResponse, error) {
	resp, err := prov.Chat(ctx, model, messages, nil)
	if err != nil {
		return nil, err
	}

	return &benchResponse{
		Content:          resp.Choices[0].Message.Content,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}, nil
}

func benchChatStream(ctx context.Context, prov benchProvider, model string, messages []openai.ChatCompletionMessage) (*benchResponse, error) {
	ch, err := prov.StreamChat(ctx, model, messages, nil)
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	var promptTokens, completionTokens, totalTokens int

	for chunk := range ch {
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			content.WriteString(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage.TotalTokens > 0 {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
			totalTokens = chunk.Usage.TotalTokens
		}
	}

	return &benchResponse{
		Content:          content.String(),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}, nil
}

func benchAnthropicEndpoint(ctx context.Context, prov benchProvider, model string, messages []openai.ChatCompletionMessage, stream bool) (*benchResponse, error) {
	req := &openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	}
	anthropicReq := anthropic.OpenAIToAnthropicRequest(req)

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}

	headers := map[string]string{
		converters.HeaderAnthropicVersion: converters.AnthropicAPIVersion,
	}

	if stream {
		return benchAnthropicStream(ctx, prov, body, headers)
	}
	return benchAnthropicNonStream(ctx, prov, body, headers)
}

func benchAnthropicNonStream(ctx context.Context, prov benchProvider, body []byte, headers map[string]string) (*benchResponse, error) {
	respBody, err := prov.DoRequest(ctx, endpoints.V1Messages, body, headers)
	if err != nil {
		return nil, err
	}

	var anthropicResp anthropic.MessagesResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	openaiResp := anthropic.AnthropicToOpenAIResponse(&anthropicResp)

	return &benchResponse{
		Content:          openaiResp.Choices[0].Message.Content,
		PromptTokens:     openaiResp.Usage.PromptTokens,
		CompletionTokens: openaiResp.Usage.CompletionTokens,
		TotalTokens:      openaiResp.Usage.TotalTokens,
	}, nil
}

func benchAnthropicStream(ctx context.Context, prov benchProvider, body []byte, headers map[string]string) (*benchResponse, error) {
	ch, err := prov.DoStreamRequest(ctx, endpoints.V1Messages, body, headers)
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	var promptTokens, completionTokens, totalTokens int

	for line := range ch {
		lineStr := string(line)
		if !strings.HasPrefix(lineStr, "data: ") {
			continue
		}
		data := strings.TrimPrefix(lineStr, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Message *anthropic.MessagesResponse `json:"message"`
			Usage   struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				content.WriteString(event.Delta.Text)
			}
		case "message_start":
			if event.Message != nil {
				promptTokens = event.Message.Usage.InputTokens
			}
		case "message_delta":
			completionTokens = event.Usage.OutputTokens
		case "message_stop":
			totalTokens = promptTokens + completionTokens
		}
	}

	return &benchResponse{
		Content:          content.String(),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}, nil
}

func findFirstAvailableProvider(providers benchProviderMap, modelConfig config.ModelConfig) (benchProvider, string, string, error) {
	for _, mp := range modelConfig.Providers {
		prov, exists := providers[mp.Provider]
		if !exists {
			continue
		}
		providerKey := fmt.Sprintf("%s/%s", mp.Provider, mp.Model)
		return prov, providerKey, mp.Model, nil
	}
	return nil, "", "", fmt.Errorf("no available providers")
}

func runBenchProviders(ctx context.Context, cfg *config.Config, providers map[string]provider.Provider, messages []openai.ChatCompletionMessage, stream bool, modelFilter string) {
	benchProviders := asBenchProviderMap(providers)
	var providerNames []string
	for name := range cfg.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, providerName := range providerNames {
		provConfig := cfg.Providers[providerName]
		prov, exists := benchProviders[providerName]
		if !exists {
			continue
		}

		baseURL := prov.BaseURL()
		apiMode := prov.APIMode()

		models := make([]string, len(provConfig.Models))
		copy(models, provConfig.Models)
		sort.Strings(models)

		for _, modelName := range models {
			if modelFilter != "" && modelFilter != providerName+"/"+modelName && modelFilter != modelName {
				continue
			}
			for _, endpoint := range getEndpointsForAPIMode(apiMode) {
				startTime := time.Now()

				logger.Info("Benchmarking provider model",
					"provider", providerName,
					"model", modelName,
					"url", baseURL,
					"endpoint", endpoint,
					"api_mode", apiMode,
					"stream", stream)

				resp, benchErr := runBenchEndpoint(ctx, prov, endpoint, modelName, messages, stream)
				writeBenchResponse(benchResult{
					Provider: providerName,
					Model:    modelName,
					ApiMode:  apiMode,
					URL:      baseURL + endpoint,
					Endpoint: endpoint,
					Prompt:   strings.TrimSpace(messages[0].Content),
					Duration: time.Since(startTime).String(),
					Stream:   stream,
				}, resp, benchErr)
			}
		}
	}
}
