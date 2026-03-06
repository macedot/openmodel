// Package main provides the command-line interface for openmodel.
//
// Usage:
//
//	openmodel serve     Start the OpenModel server (default)
//	openmodel test      Test configured models
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
		modelName := flag.String("model", "", "Model name to test (tests all if omitted)")

		if err := parseCommandFlags(args, printTestUsage); err != nil {
			return
		}
		runTest(modelName)
		return
	}

	if command == "models" {
		jsonOutput := flag.Bool("json", false, "Output in JSON format")

		if err := parseCommandFlags(args, printModelsUsage); err != nil {
			return
		}
		runModels(jsonOutput)
		return
	}

	if command == "config" {
		if err := parseCommandFlags(args, printConfigUsage); err != nil {
			return
		}
		runConfig()
		return
	}

	if command != "serve" {
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}

	// Serve command - declare flags before parsing
	configPath := flag.String("config", "", "Path to config file (default: ~/.config/openmodel/config.json)")
	showHelp := flag.Bool("h", false, "Show help")

	if err := parseCommandFlags(args, printServerUsage); err != nil {
		return
	}

	if *showHelp {
		printServerUsage()
		os.Exit(0)
	}

	runServer(configPath)
}

// parseCommandFlags parses command-line flags with error handling
func parseCommandFlags(args []string, usage func()) error {
	err := flag.CommandLine.Parse(args)
	if err != nil {
		if err == flag.ErrHelp {
			usage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		usage()
		os.Exit(1)
	}
	return nil
}

// initProviders creates and initializes all configured providers
func initProviders(cfg *config.Config) map[string]provider.Provider {
	providers := make(map[string]provider.Provider)
	for name, pc := range cfg.Providers {
		providers[name] = provider.NewOpenAIProvider(name, pc.URL, pc.APIKey)
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

func runServer(configPath *string) {
	// Load config from specified path or default location
	var cfg *config.Config
	var err error

	if *configPath != "" {
		// Validate custom config path exists
		if _, err := os.Stat(*configPath); os.IsNotExist(err) {
			log.Fatalf("Config file not found: %s", *configPath)
		}
		cfg, err = config.LoadFromPath(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		logger.Info("Config loaded from custom path", "config_path", *configPath)
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

	providers := initProviders(cfg)

	stateMgr := state.New(10000) // 10 second initial timeout
	srv := server.New(cfg, providers, stateMgr)

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

func printTestUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s test [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func printServerUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s serve [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  --config string   Path to config file (default: ~/.config/openmodel/config.json)\n")
	fmt.Fprintf(os.Stderr, "  -h               Show help\n")
}

func printModelsUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s models [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func printConfigUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s config\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nFind and validate config file.\n")
	fmt.Fprintf(os.Stderr, "\nOutputs the config file path if valid.\n")
	fmt.Fprintf(os.Stderr, "Only prints errors if validation fails.\n")
}

func runModels(jsonOutput *bool) {
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n\n", flag.Arg(0))
		printModelsUsage()
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

	// Group providers by model name, preserving order
	modelMap := make(map[string][]providerInfo)
	modelOrder := make([]string, 0)
	for name, modelConfig := range cfg.Models {
		if _, exists := modelMap[name]; !exists {
			modelOrder = append(modelOrder, name)
		}
		for _, p := range modelConfig.Providers {
			modelMap[name] = append(modelMap[name], providerInfo{
				Provider: p.Provider,
				Model:    p.Model,
			})
		}
	}

	// Build ordered list with default marker
	models := make([]modelInfo, 0, len(modelOrder))
	for i, name := range modelOrder {
		providers := modelMap[name]
		info := modelInfo{
			Name:      name,
			Providers: providers,
		}
		// First defined model is the default, or if only one model exists
		if i == 0 || len(modelOrder) == 1 {
			info.Default = true
		}
		models = append(models, info)
	}

	if *jsonOutput {
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

func runTest(modelName *string) {
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n\n", flag.Arg(0))
		printTestUsage()
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

	if *modelName != "" {
		logger.Info("Testing specific model", "model", *modelName)
	} else {
		logger.Info("Testing all configured models")
	}

	failed := runTests(providers, cfg, *modelName)

	if failed > 0 {
		os.Exit(1)
	}
}

func runTests(providers map[string]provider.Provider, cfg *config.Config, modelName string) int {
	failed := 0

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Determine which models to test
	modelsToTest := cfg.Models
	if modelName != "" {
		if modelConfig, exists := cfg.Models[modelName]; exists {
			modelsToTest = map[string]config.ModelConfig{
				modelName: modelConfig,
			}
		} else {
			logger.Error("Model not found", "model", modelName)
			return 1
		}
	}

	// Test each model's providers (submodels)
	for modelName, modelConfig := range modelsToTest {
		logger.Info("Testing model", "model", modelName)

		for _, mp := range modelConfig.Providers {
			prov, exists := providers[mp.Provider]
			if !exists {
				logger.Error("Provider not found", "provider", mp.Provider, "model", modelName)
				failed++
				continue
			}

			logger.Info("Testing submodel", "model", modelName, "provider", mp.Provider, "submodel", mp.Model)

			// Test Chat with "hi" message
			chatResult := testChatModel(ctx, prov, mp.Model)

			if chatResult.Success {
				logger.Info("Test passed", "model", modelName, "provider", mp.Provider, "submodel", mp.Model, "latency", chatResult.Latency)
			} else {
				failed++
				logger.Error("Test failed", "model", modelName, "provider", mp.Provider, "submodel", mp.Model, "error", chatResult.Error)
			}
		}
	}

	total := 0
	for _, modelConfig := range modelsToTest {
		total += len(modelConfig.Providers)
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
