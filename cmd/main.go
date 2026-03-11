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

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/server"
	"github.com/macedot/openmodel/internal/state"
)

// Version is set at build time via -ldflags "-X main.Version=1.0.0"
var Version = "dev"

// BuildDate is set at build time via -ldflags "-X main.BuildDate=..."
var BuildDate = "unknown"

// newTestFlagSet creates a FlagSet for the test command.
func newTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	fs.String("model", "", "Model name to test (tests all if omitted)")
	return fs
}

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
	return fs
}

// newServeFlagSet creates a FlagSet for the serve command.
func newServeFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.String("config", "", "Path to config file (default: ~/.config/openmodel/config.json)")
	fs.Bool("h", false, "Show help")
	return fs
}

// MethodResult represents the result of testing a single API method.
type MethodResult struct {
	Success bool
	Error   string
	Latency string
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

	if command == "test" {
		fs := newTestFlagSet()
		fs.Usage = func() { printTestUsage(fs) }

		if err := fs.Parse(args); err != nil {
			os.Exit(1)
		}

		modelName := fs.Lookup("model").Value.String()
		runTest(modelName)
		return
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

		if promptFile == "" {
			fmt.Fprintf(os.Stderr, "Error: -prompt is required\n\n")
			fs.Usage()
			os.Exit(1)
		}
		runBench(promptFile, scope)
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
	fmt.Fprintf(os.Stderr, "  test     Test configured providers\n")
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

func printTestUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, "Usage: %s test [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fs.PrintDefaults()
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

func runTest(modelName string) {
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n\n", flag.Arg(0))
		printTestUsage(flag.NewFlagSet("test", flag.ExitOnError))
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Initializing providers")

	providers := initProviders(cfg)

	if modelName != "" {
		logger.Info("Testing specific model", "model", modelName)
	} else {
		logger.Info("Testing all configured models")
	}

	failed := runTests(providers, cfg, modelName)

	if failed > 0 {
		os.Exit(1)
	}
}

func runTests(providers map[string]provider.Provider, cfg *config.Config, modelName string) int {
	failed := 0

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Determine which providers to test
	providersToTest := cfg.Providers
	if modelName != "" {
		// If a specific model is given, find which provider has it
		found := false
		for name, prov := range cfg.Providers {
			for _, m := range prov.Models {
				if m == modelName {
					providersToTest = map[string]config.ProviderConfig{
						name: prov,
					}
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			logger.Error("Model not found in any provider", "model", modelName)
			return 1
		}
	}

	// Test each provider's models
	for provName, provConfig := range providersToTest {
		prov, exists := providers[provName]
		if !exists {
			logger.Error("Provider not initialized", "provider", provName)
			failed++
			continue
		}

		for _, model := range provConfig.Models {
			logger.Info("Testing provider model", "provider", provName, "model", model)

			// Test Chat with "hi" message
			chatResult := testChatModel(ctx, prov, model)

			if chatResult.Success {
				logger.Info("Test passed", "provider", provName, "model", model, "latency", chatResult.Latency)
			} else {
				failed++
				logger.Error("Test failed", "provider", provName, "model", model, "error", chatResult.Error)
			}
		}
	}

	total := 0
	for _, provConfig := range providersToTest {
		total += len(provConfig.Models)
	}
	passed := total - failed
	logger.Info("Test completed", "total", total, "passed", passed, "failed", failed)

	return failed
}

func testChatModel(ctx context.Context, prov provider.Provider, model string) *MethodResult {
	start := time.Now()

	messages := []openai.ChatCompletionMessage{
		{
			Role:    "user",
			Content: "hi",
		},
	}

	_, err := prov.Chat(ctx, model, messages, &openai.ChatCompletionRequest{
		MaxTokens: intPtr(10),
	})
	latency := time.Since(start)

	if err != nil {
		return &MethodResult{
			Success: false,
			Error:   err.Error(),
			Latency: latency.String(),
		}
	}

	return &MethodResult{
		Success: true,
		Latency: latency.String(),
	}
}

func intPtr(i int) *int {
	return &i
}

// runBench executes benchmark tests based on scope mode
func runBench(promptFile, scope string) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
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
		runBenchApplication(ctx, cfg, providers, messages)
	case "providers":
		runBenchProviders(ctx, cfg, providers, messages)
	case "all":
		runBenchApplication(ctx, cfg, providers, messages)
		runBenchProviders(ctx, cfg, providers, messages)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid scope '%s'. Use: application, providers, or all\n", scope)
		os.Exit(1)
	}
}

// benchResult holds the result of a single benchmark run
type benchResult struct {
	Type       string `json:"type"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	ProviderID string `json:"provider_id,omitempty"`
	Strategy   string `json:"strategy,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Error      string `json:"error,omitempty"`
	Response   string `json:"response,omitempty"`
	Duration   string `json:"duration"`
	Tokens     *benchTokens `json:"tokens,omitempty"`
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

	filename := fmt.Sprintf("%d-bench-%s-%s.json", time.Now().UnixNano(), sanitizedProvider, sanitizedModel)
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
func runBenchApplication(ctx context.Context, cfg *config.Config, providers map[string]provider.Provider, messages []openai.ChatCompletionMessage) {
	for _, modelName := range cfg.ModelOrder {
		modelConfig, exists := cfg.Models[modelName]
		if !exists {
			continue
		}

		startTime := time.Now()

		// Get first available provider from the chain
		prov, providerKey, providerModel, err := findFirstAvailableProvider(cfg, providers, modelConfig)

		if err != nil {
			result := benchResult{
				Type:     "error",
				Provider: modelName,
				Model:    modelName,
				Strategy: modelConfig.Strategy,
				Prompt:   truncate(strings.TrimSpace(messages[0].Content), 100),
				Error:    err.Error(),
				Duration: time.Since(startTime).String(),
			}
			writeBenchResult(result)
			continue
		}

		// Make the request
		resp, err := prov.Chat(ctx, providerModel, messages, nil)
		duration := time.Since(startTime)

		if err != nil {
			result := benchResult{
				Type:       "error",
				Provider:   modelName,
				Model:      modelName,
				ProviderID: providerKey,
				Strategy:   modelConfig.Strategy,
				Prompt:     truncate(strings.TrimSpace(messages[0].Content), 100),
				Error:      err.Error(),
				Duration:   duration.String(),
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
			Prompt:     truncate(strings.TrimSpace(messages[0].Content), 100),
			Response:   resp.Choices[0].Message.Content,
			Duration:   duration.String(),
		}

		if resp.Usage.TotalTokens > 0 {
			result.Tokens = &benchTokens{
				Prompt:     resp.Usage.PromptTokens,
				Completion: resp.Usage.CompletionTokens,
				Total:      resp.Usage.TotalTokens,
			}
			if resp.Usage.PromptTokens > 0 {
				result.TokensPerSec = float64(resp.Usage.CompletionTokens) / duration.Seconds()
			}
		}

		writeBenchResult(result)
	}
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
func runBenchProviders(ctx context.Context, cfg *config.Config, providers map[string]provider.Provider, messages []openai.ChatCompletionMessage) {
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

		// Sort model names for consistent output
		models := make([]string, len(provConfig.Models))
		copy(models, provConfig.Models)
		sort.Strings(models)

		for _, modelName := range models {
			startTime := time.Now()

			resp, err := prov.Chat(ctx, modelName, messages, nil)
			duration := time.Since(startTime)

			if err != nil {
				result := benchResult{
					Type:     "error",
					Provider: providerName,
					Model:    modelName,
					Prompt:   truncate(strings.TrimSpace(messages[0].Content), 100),
					Error:    err.Error(),
					Duration: duration.String(),
				}
				writeBenchResult(result)
				continue
			}

			result := benchResult{
				Type:     "response",
				Provider: providerName,
				Model:    modelName,
				Prompt:   truncate(strings.TrimSpace(messages[0].Content), 100),
				Response: resp.Choices[0].Message.Content,
				Duration: duration.String(),
			}

			if resp.Usage.TotalTokens > 0 {
				result.Tokens = &benchTokens{
					Prompt:     resp.Usage.PromptTokens,
					Completion: resp.Usage.CompletionTokens,
					Total:      resp.Usage.TotalTokens,
				}
				if resp.Usage.PromptTokens > 0 {
					result.TokensPerSec = float64(resp.Usage.CompletionTokens) / duration.Seconds()
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