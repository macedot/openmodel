// Package main provides the command-line interface for openmodel.
//
// Usage:
//
//	openmodel serve     Start the OpenModel server (default)
//	openmodel test      Test configured models
//	openmodel bench     Benchmark models with prompts
//	openmodel -h        Show help
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/macedot/openmodel/internal/api/anthropic"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/endpoints"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server"
	"github.com/macedot/openmodel/internal/state"
)

// Version is set at build time via -ldflags "-X main.Version=1.0.0"
var Version = "dev"

// BuildDate is set at build time via -ldflags "-X main.BuildDate=..."
var BuildDate = "unknown"

// newModelsFlagSet creates a FlagSet for the models command.
func newModelsFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	fs.Bool("json", false, "Output in JSON format")
	return fs
}

// newBenchFlagSet creates a FlagSet for the bench command.
func newBenchFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	fs.String("prompt", "", "Path to file containing the prompt (required)")
	fs.String("scope", "application", "Scope: application, providers, or all")
	fs.Bool("stream", false, "Use streaming mode for requests")
	return fs
}

// newServeFlagSet creates a FlagSet for the serve command.
func newServeFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.String("config", "", "Path to config file (default: ~/.config/openmodel/config.json)")
	fs.Bool("h", false, "Show help")
	return fs
}

func main() {
	command := ""
	args := os.Args[1:]

	// Handle version flag early, before command processing
	for _, arg := range args {
		if arg == "-v" || arg == "--version" {
			printVersion()
			os.Exit(0)
		}
	}

	for i, arg := range args {
		if arg == "-h" || arg == "--help" {
			printUsage()
			os.Exit(0)
		}
		if !strings.HasPrefix(arg, "-") {
			command = arg
			args = args[i+1:]
			break
		}
	}

	// Default command if none provided
	if command == "" {
		printUsage()
		os.Exit(0)
	}

	if command == "models" {
		fs := newModelsFlagSet()
		fs.Usage = func() { printModelsUsage(fs) }

		if err := fs.Parse(args); err != nil {
			os.Exit(1)
		}

		jsonOutput := fs.Lookup("json").Value.(flag.Getter).Get().(bool)
		runModels(jsonOutput)
		return
	}

	if command == "config" {
		fs := flag.NewFlagSet("config", flag.ExitOnError)
		fs.Usage = func() { printConfigUsage() }

		if err := fs.Parse(args); err != nil {
			os.Exit(1)
		}
		runConfig()
		return
	}

	if command == "bench" {
		fs := newBenchFlagSet()
		fs.Usage = func() { printBenchUsage(fs) }

		if err := fs.Parse(args); err != nil {
			os.Exit(1)
		}

		promptFile := fs.Lookup("prompt").Value.String()
		scope := fs.Lookup("scope").Value.String()
		stream := fs.Lookup("stream").Value.(flag.Getter).Get().(bool)

		if promptFile == "" {
			fmt.Fprintf(os.Stderr, "Error: -prompt is required\n\n")
			fs.Usage()
			os.Exit(1)
		}
		runBench(promptFile, scope, stream)
		return
	}

	if command != "serve" {
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}

	// Serve command - the default
	fs := newServeFlagSet()
	fs.Usage = func() { printServerUsage(fs) }

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	showHelp := fs.Lookup("h").Value.(flag.Getter).Get().(bool)
	if showHelp {
		fs.Usage()
		os.Exit(0)
	}

	configPath := fs.Lookup("config").Value.String()
	runServer(configPath)
}

// initProviders creates and initializes all configured providers
func initProviders(cfg *config.Config) map[string]provider.Provider {
	providers := make(map[string]provider.Provider)

	// Convert config.HTTP to provider.HTTPConfig
	httpConfig := provider.HTTPConfig{
		TimeoutSeconds:               cfg.HTTP.TimeoutSeconds,
		MaxIdleConns:                 cfg.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost:          cfg.HTTP.MaxIdleConnsPerHost,
		IdleConnTimeoutSeconds:       cfg.HTTP.IdleConnTimeoutSeconds,
		DialTimeoutSeconds:           cfg.HTTP.DialTimeoutSeconds,
		TLSHandshakeTimeoutSeconds:   cfg.HTTP.TLSHandshakeTimeoutSeconds,
		ResponseHeaderTimeoutSeconds: cfg.HTTP.ResponseHeaderTimeoutSeconds,
	}

	for name, pc := range cfg.Providers {
		providers[name] = provider.NewOpenAIProviderWithConfig(name, pc.URL, pc.APIKey, httpConfig)
		logger.Info("Provider initialized", "name", name, "url", pc.URL)
	}
	return providers
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [command]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  serve    Start the OpenModel server\n")
	fmt.Fprintf(os.Stderr, "  models   List available models\n")
	fmt.Fprintf(os.Stderr, "  config   Find and validate config file\n")
	fmt.Fprintf(os.Stderr, "  bench    Benchmark models with prompts\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  -h, --help    Show help\n")
	fmt.Fprintf(os.Stderr, "  -v, --version Show version\n")
	fmt.Fprintf(os.Stderr, "\nServe options:\n")
	fmt.Fprintf(os.Stderr, "  --config <path>   Path to config file (default: ~/.config/openmodel/config.json)\n")
	fmt.Fprintf(os.Stderr, "\nRun '%s <command> -h' for more information on a command.\n", os.Args[0])
}

func printVersion() {
	fmt.Printf("openmodel version %s\n", Version)
	if BuildDate != "unknown" {
		fmt.Printf("build date: %s\n", BuildDate)
	}
}

func printBenchUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, "Usage: %s bench [options]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Benchmark models by submitting prompts.\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	fs.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nScope modes:\n")
	fmt.Fprintf(os.Stderr, "  application  Test each model alias (uses configured failover chains)\n")
	fmt.Fprintf(os.Stderr, "  providers    Test every model on every provider individually\n")
	fmt.Fprintf(os.Stderr, "  all          Run both application and providers modes\n")
}

func runServer(configPath string) {
	// Load config from specified path or default location
	var cfg *config.Config
	var err error

	if configPath != "" {
		// Validate custom config path exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			log.Fatalf("Config file not found: %s", configPath)
		}
		cfg, err = config.LoadFromPath(configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		logger.Info("Config loaded from custom path", "config_path", configPath)
	} else {
		cfg, err = config.Load()
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		logger.Info("Config loaded", "config_path", config.GetConfigPath())
	}

	if err := logger.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Validate all provider references exist
	if err := cfg.ValidateProviderReferences(); err != nil {
		log.Fatalf("Configuration error:\n%v", err)
	}

	// Validate only one model is marked as default
	if err := cfg.ValidateDefaultModels(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Validate api_mode values
	if err := cfg.ValidateApiModes(); err != nil {
		log.Fatalf("Configuration error:\n%v", err)
	}

	providers := initProviders(cfg)

	stateMgr := state.New(10000) // 10 second initial timeout
	srv := server.New(cfg, providers, stateMgr, Version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Shutting down...")
		srv.Stop(ctx)
		cancel()
	}()

	logger.Info("Starting openmodel", "host", cfg.Server.Host, "port", cfg.Server.Port)
	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server error", "error", err)
	}
}

func printServerUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, "Usage: %s serve [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fs.PrintDefaults()
}

func printModelsUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, "Usage: %s models [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fs.PrintDefaults()
}

func printConfigUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s config\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nFind and validate config file.\n")
	fmt.Fprintf(os.Stderr, "\nOutputs the config file path if valid.\n")
	fmt.Fprintf(os.Stderr, "Only prints errors if validation fails.\n")
}

func runModels(jsonOutput bool) {
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n\n", flag.Arg(0))
		printModelsUsage(flag.NewFlagSet("models", flag.ExitOnError))
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Build grouped models from config
	type providerInfo struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	type modelInfo struct {
		Name      string         `json:"name"`
		Providers []providerInfo `json:"providers"`
		Default   bool           `json:"default,omitempty"`
	}

	// Group providers by model name, using preserved order from config
	modelOrder := cfg.ModelOrder
	if len(modelOrder) == 0 {
		// Fallback: extract model names from map (order will be non-deterministic)
		modelOrder = make([]string, 0, len(cfg.Models))
		for name := range cfg.Models {
			modelOrder = append(modelOrder, name)
		}
	}

	modelMap := make(map[string][]providerInfo)
	for name, modelConfig := range cfg.Models {
		for _, p := range modelConfig.Providers {
			modelMap[name] = append(modelMap[name], providerInfo{
				Provider: p.Provider,
				Model:    p.Model,
			})
		}
	}

	// Build ordered list with default marker
	models := make([]modelInfo, 0, len(modelOrder))
	hasExplicitDefault := false
	for _, name := range modelOrder {
		providers := modelMap[name]
		modelConfig := cfg.Models[name]
		info := modelInfo{
			Name:      name,
			Providers: providers,
			Default:   modelConfig.Default, // Use configured default
		}
		if modelConfig.Default {
			hasExplicitDefault = true
		}
		models = append(models, info)
	}

	// If no explicit default is set, use the first model (in config order)
	if !hasExplicitDefault && len(models) > 0 {
		models[0].Default = true
	}

	if jsonOutput {
		data, err := json.MarshalIndent(models, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		if len(models) == 0 {
			fmt.Println("No models configured")
			return
		}
		fmt.Println("Available models:")
		fmt.Println()
		for _, m := range models {
			defaultMarker := ""
			if m.Default {
				defaultMarker = " (default)"
			}
			fmt.Printf("  %s%s\n", m.Name, defaultMarker)
			for _, p := range m.Providers {
				fmt.Printf("    provider: %s, model: %s\n", p.Provider, p.Model)
			}
		}
	}
}

func runConfig() {
	configPath := config.GetConfigPath()
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not determine config path (home directory not found)")
		os.Exit(1)
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: config file not found: %s\n", configPath)
		os.Exit(1)
	}

	// Try to load and validate config
	_, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Only print path on success
	fmt.Println(configPath)
}

// runBench executes benchmark tests based on scope mode
func runBench(promptFile, scope string, stream bool) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Read prompt file
	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading prompt file: %v\n", err)
		os.Exit(1)
	}
	promptStr := strings.TrimSpace(string(prompt))

	// Initialize providers
	providers := initProviders(cfg)

	// Create benchmark context
	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: promptStr},
	}

	// Run benchmarks based on scope
	switch scope {
	case "application", "app":
		runBenchApplication(ctx, cfg, providers, messages, stream)
	case "providers":
		runBenchProviders(ctx, cfg, providers, messages, stream)
	case "all":
		runBenchApplication(ctx, cfg, providers, messages, stream)
		runBenchProviders(ctx, cfg, providers, messages, stream)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid scope '%s'. Use: application, providers, or all\n", scope)
		os.Exit(1)
	}
}

// benchResult holds the result of a single benchmark run
type benchResult struct {
	Type        string `json:"type"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	ProviderID  string `json:"provider_id,omitempty"`
	Strategy    string `json:"strategy,omitempty"`
	ApiMode     string `json:"api_mode,omitempty"`
	URL         string `json:"url"`
	Endpoint    string `json:"endpoint"`
	Prompt      string `json:"prompt,omitempty"`
	Error       string `json:"error,omitempty"`
	Response    string `json:"response,omitempty"`
	Duration    string `json:"duration"`
	Stream      bool   `json:"stream"`
	Tokens      *benchTokens `json:"tokens,omitempty"`
	TokensPerSec float64 `json:"tokens_per_sec,omitempty"`
}

type benchTokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// writeBenchResult writes a benchmark result to a JSON file
func writeBenchResult(result benchResult) {
	// Sanitize provider and model names for filename
	sanitizedProvider := sanitizeBenchName(result.Provider)
	sanitizedModel := sanitizeBenchName(result.Model)
	sanitizedEndpoint := sanitizeBenchName(result.Endpoint)

	filename := fmt.Sprintf("%d-bench-%s-%s-%s.json", time.Now().UnixNano(), sanitizedProvider, sanitizedModel, sanitizedEndpoint)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling benchmark result: %v\n", err)
		return
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing benchmark file: %v\n", err)
	}
}

// sanitizeBenchName sanitizes names for use in filenames
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

// runBenchApplication tests each configured model alias using its failover chain
func runBenchApplication(ctx context.Context, cfg *config.Config, providers map[string]provider.Provider, messages []openai.ChatCompletionMessage, stream bool) {
	for _, modelName := range cfg.ModelOrder {
		modelConfig, exists := cfg.Models[modelName]
		if !exists {
			continue
		}

		// Get first available provider from the chain
		prov, providerKey, providerModel, err := findFirstAvailableProvider(cfg, providers, modelConfig)
		if err != nil {
			startTime := time.Now()
			result := benchResult{
				Type:     "error",
				Provider: modelName,
				Model:    modelName,
				Strategy: modelConfig.Strategy,
				ApiMode:  modelConfig.ApiMode,
				Prompt:   truncate(strings.TrimSpace(messages[0].Content), 100),
				Error:    err.Error(),
				Duration: time.Since(startTime).String(),
				Stream:   stream,
			}
			writeBenchResult(result)
			continue
		}

		// Determine which endpoints to test based on api_mode
		testEndpoints := getEndpointsForApiMode(modelConfig.ApiMode)
		baseURL := prov.BaseURL()

		for _, endpoint := range testEndpoints {
			startTime := time.Now()

			logger.Info("Benchmarking model",
				"model", modelName,
				"provider", providerKey,
				"url", baseURL,
				"endpoint", endpoint,
				"stream", stream)

			var resp *benchResponse
			var benchErr error

			if endpoint == endpoints.ChatCompletions {
				// OpenAI endpoint
				resp, benchErr = benchChat(ctx, prov, providerModel, messages, stream)
			} else {
				// Anthropic endpoint - convert and use raw request
				resp, benchErr = benchAnthropicEndpoint(ctx, prov, providerModel, messages, stream)
			}

			duration := time.Since(startTime)

			if benchErr != nil {
				result := benchResult{
					Type:       "error",
					Provider:   modelName,
					Model:      modelName,
					ProviderID: providerKey,
					Strategy:   modelConfig.Strategy,
					ApiMode:    modelConfig.ApiMode,
					URL:        baseURL,
					Endpoint:   endpoint,
					Prompt:     truncate(strings.TrimSpace(messages[0].Content), 100),
					Error:      benchErr.Error(),
					Duration:   duration.String(),
					Stream:     stream,
				}
				writeBenchResult(result)
				continue
			}

			result := benchResult{
				Type:       "response",
				Provider:   modelName,
				Model:      modelName,
				ProviderID: providerKey,
				Strategy:   modelConfig.Strategy,
				ApiMode:    modelConfig.ApiMode,
				URL:        baseURL,
				Endpoint:   endpoint,
				Prompt:     truncate(strings.TrimSpace(messages[0].Content), 100),
				Response:   resp.Content,
				Duration:   duration.String(),
				Stream:     stream,
			}

			if resp.TotalTokens > 0 {
				result.Tokens = &benchTokens{
					Prompt:     resp.PromptTokens,
					Completion: resp.CompletionTokens,
					Total:      resp.TotalTokens,
				}
				if resp.PromptTokens > 0 {
					result.TokensPerSec = float64(resp.CompletionTokens) / duration.Seconds()
				}
			}

			writeBenchResult(result)
		}
	}
}

// getEndpointsForApiMode returns the endpoints to test based on api_mode
func getEndpointsForApiMode(apiMode string) []string {
	switch apiMode {
	case "openai":
		return []string{endpoints.ChatCompletions}
	case "anthropic":
		return []string{endpoints.V1Messages}
	default:
		// Empty or unknown: test both endpoints
		return []string{endpoints.ChatCompletions, endpoints.V1Messages}
	}
}

// benchResponse holds the response from a benchmark chat call
type benchResponse struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// benchChat performs a chat request, using streaming or non-streaming based on the stream flag
func benchChat(ctx context.Context, prov provider.Provider, model string, messages []openai.ChatCompletionMessage, stream bool) (*benchResponse, error) {
	if stream {
		return benchChatStream(ctx, prov, model, messages)
	}
	return benchChatNonStream(ctx, prov, model, messages)
}

// benchChatNonStream performs a non-streaming chat request
func benchChatNonStream(ctx context.Context, prov provider.Provider, model string, messages []openai.ChatCompletionMessage) (*benchResponse, error) {
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

// benchChatStream performs a streaming chat request
func benchChatStream(ctx context.Context, prov provider.Provider, model string, messages []openai.ChatCompletionMessage) (*benchResponse, error) {
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
		// Accumulate usage from streaming response
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

// benchAnthropicEndpoint performs a request using Anthropic format to /v1/messages endpoint
func benchAnthropicEndpoint(ctx context.Context, prov provider.Provider, model string, messages []openai.ChatCompletionMessage, stream bool) (*benchResponse, error) {
	// Convert OpenAI messages to Anthropic format
	req := &openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	}
	anthropicReq := anthropic.OpenAIToAnthropicRequest(req)

	// Marshal the Anthropic request
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}

	// Set headers for Anthropic API
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}

	if stream {
		return benchAnthropicStream(ctx, prov, body, headers, model)
	}
	return benchAnthropicNonStream(ctx, prov, body, headers, model)
}

// benchAnthropicNonStream performs a non-streaming Anthropic request
func benchAnthropicNonStream(ctx context.Context, prov provider.Provider, body []byte, headers map[string]string, model string) (*benchResponse, error) {
	respBody, err := prov.DoRequest(ctx, endpoints.V1Messages, body, headers)
	if err != nil {
		return nil, err
	}

	// Parse Anthropic response
	var anthropicResp anthropic.MessagesResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	// Convert to OpenAI format for consistent handling
	openaiResp := anthropic.AnthropicToOpenAIResponse(&anthropicResp)

	return &benchResponse{
		Content:          openaiResp.Choices[0].Message.Content,
		PromptTokens:     openaiResp.Usage.PromptTokens,
		CompletionTokens: openaiResp.Usage.CompletionTokens,
		TotalTokens:      openaiResp.Usage.TotalTokens,
	}, nil
}

// benchAnthropicStream performs a streaming Anthropic request
func benchAnthropicStream(ctx context.Context, prov provider.Provider, body []byte, headers map[string]string, model string) (*benchResponse, error) {
	ch, err := prov.DoStreamRequest(ctx, endpoints.V1Messages, body, headers)
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	var promptTokens, completionTokens, totalTokens int

	for line := range ch {
		// Parse SSE line
		lineStr := string(line)
		if !strings.HasPrefix(lineStr, "data: ") {
			continue
		}
		data := strings.TrimPrefix(lineStr, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		// Parse the Anthropic event
		var event struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Delta   struct {
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

		// Handle different event types
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
			// Message complete
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

// findFirstAvailableProvider returns the first provider in the chain that is available
func findFirstAvailableProvider(cfg *config.Config, providers map[string]provider.Provider, modelConfig config.ModelConfig) (provider.Provider, string, string, error) {
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

// runBenchProviders tests every model on every provider individually
func runBenchProviders(ctx context.Context, cfg *config.Config, providers map[string]provider.Provider, messages []openai.ChatCompletionMessage, stream bool) {
	// Sort provider names for consistent output
	var providerNames []string
	for name := range cfg.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, providerName := range providerNames {
		provConfig := cfg.Providers[providerName]
		prov, exists := providers[providerName]
		if !exists {
			continue
		}

		baseURL := prov.BaseURL()

		// Sort model names for consistent output
		models := make([]string, len(provConfig.Models))
		copy(models, provConfig.Models)
		sort.Strings(models)

		for _, modelName := range models {
			startTime := time.Now()

			logger.Info("Benchmarking provider model",
				"provider", providerName,
				"model", modelName,
				"url", baseURL,
				"endpoint", endpoints.ChatCompletions,
				"stream", stream)

			resp, err := benchChat(ctx, prov, modelName, messages, stream)
			duration := time.Since(startTime)

			if err != nil {
				result := benchResult{
					Type:     "error",
					Provider: providerName,
					Model:    modelName,
					URL:      baseURL,
					Endpoint: endpoints.ChatCompletions,
					Prompt:   truncate(strings.TrimSpace(messages[0].Content), 100),
					Error:    err.Error(),
					Duration: duration.String(),
					Stream:   stream,
				}
				writeBenchResult(result)
				continue
			}

			result := benchResult{
				Type:     "response",
				Provider: providerName,
				Model:    modelName,
				URL:      baseURL,
				Endpoint: endpoints.ChatCompletions,
				Prompt:   truncate(strings.TrimSpace(messages[0].Content), 100),
				Response: resp.Content,
				Duration: duration.String(),
				Stream:   stream,
			}

			if resp.TotalTokens > 0 {
				result.Tokens = &benchTokens{
					Prompt:     resp.PromptTokens,
					Completion: resp.CompletionTokens,
					Total:      resp.TotalTokens,
				}
				if resp.PromptTokens > 0 {
					result.TokensPerSec = float64(resp.CompletionTokens) / duration.Seconds()
				}
			}

			writeBenchResult(result)
		}
	}
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatProviders formats a slice of ModelProvider for display
func formatProviders(providers []config.ModelProvider) string {
	var parts []string
	for _, p := range providers {
		parts = append(parts, fmt.Sprintf("%s/%s", p.Provider, p.Model))
	}
	return strings.Join(parts, ", ")
}