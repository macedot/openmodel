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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
)

// Version is set at build time via -ldflags "-X main.Version=1.0.0"
var Version = "dev"

// BuildDate is set at build time via -ldflags "-X main.BuildDate=..."
var BuildDate = "unknown"

// newModelsFlagSet creates a FlagSet for the models command.
func newModelsFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	return fs
}

// newBenchFlagSet creates a FlagSet for the bench command.
func newBenchFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	fs.String("prompt", "", "Prompt text to send (required)")
	fs.String("scope", "application", "Scope: application, providers, or all")
	fs.String("model", "", "Filter: only benchmark specific model")
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

	switch command {
	case "serve":
		runServeCmd(args)
	case "models":
		runModelsCmd(args)
	case "config":
		runConfigCmd(args)
	case "bench":
		runBenchCmd(args)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
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

// runServeCmd handles the serve command
func runServeCmd(args []string) {
	cfg, exitCode := executeServeCmd(args)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	runServer(cfg)
}

func executeServeCmd(args []string) (*config.Config, int) {
	fs := newServeFlagSet()
	fs.SetOutput(io.Discard)
	fs.Usage = func() { printServerUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return nil, 1
	}

	showHelp := fs.Lookup("h").Value.(flag.Getter).Get().(bool)
	if showHelp {
		fs.Usage()
		return nil, 0
	}

	configPath := fs.Lookup("config").Value.String()
	cfg, err := loadAndValidateConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return nil, 1
	}

	return cfg, 0
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

// runModelsCmd handles the models command
func runModelsCmd(args []string) {
	cfg, exitCode := executeModelsCmd(args)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	printModels(cfg)
}

func executeModelsCmd(args []string) (*config.Config, int) {
	fs := newModelsFlagSet()
	fs.SetOutput(io.Discard)
	fs.Usage = func() { printModelsUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return nil, 1
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n\n", fs.Arg(0))
		fs.Usage()
		return nil, 1
	}

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return nil, 1
	}

	return cfg, 0
}

// runModels is a wrapper for backward compatibility with tests
func runModels(_ bool) {
	cfg, exitCode := executeModelsCmd(nil)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	printModels(cfg)
}

// printModels displays the configured models
func printModels(cfg *config.Config) {
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

// runConfigCmd handles the config command
func runConfigCmd(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() { printConfigUsage() }

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if err := executeConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func executeConfig() error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	configPath := cfg.GetConfigPath()
	if configPath == "" {
		return fmt.Errorf("could not determine config path (home directory not found)")
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", configPath)
	}

	// Only print path on success
	fmt.Println(configPath)
	return nil
}

func runConfig() {
	if err := executeConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runBenchCmd handles the bench command
func runBenchCmd(args []string) {
	promptStr, scope, stream, model, exitCode := parseBenchArgs(args)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	runBench(promptStr, scope, stream, model)
}

func parseBenchArgs(args []string) (string, string, bool, string, int) {
	fs := newBenchFlagSet()
	fs.SetOutput(io.Discard)
	fs.Usage = func() { printBenchUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return "", "", false, "", 1
	}

	promptStr := fs.Lookup("prompt").Value.String()
	scope := fs.Lookup("scope").Value.String()
	model := fs.Lookup("model").Value.String()
	stream := fs.Lookup("stream").Value.(flag.Getter).Get().(bool)

	if promptStr == "" {
		fmt.Fprintf(os.Stderr, "Error: -prompt is required\n\n")
		fs.Usage()
		return "", "", false, "", 1
	}

	return promptStr, scope, stream, model, 0
}

// runBench executes benchmark tests based on scope mode
func runBench(promptStr, scope string, stream bool, model string) {
	cfg := mustLoadAndValidateConfig("")

	// Trim prompt
	promptStr = strings.TrimSpace(promptStr)

	// Initialize providers
	providers := initProviders(cfg)
	benchProviders := asBenchProviderMap(providers)

	// Create benchmark context
	ctx := context.Background()
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: promptStr},
	}

	// Run benchmarks based on scope
	switch scope {
	case "application", "app":
		runBenchApplication(ctx, cfg, benchProviders, messages, stream, model)
	case "providers":
		runBenchProviders(ctx, cfg, providers, messages, stream, model)
	case "all":
		runBenchApplication(ctx, cfg, benchProviders, messages, stream, model)
		runBenchProviders(ctx, cfg, providers, messages, stream, model)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid scope '%s'. Use: application, providers, or all\n", scope)
		os.Exit(1)
	}
}

// formatProviders formats a slice of ModelProvider for display
func formatProviders(providers []config.ModelProvider) string {
	var parts []string
	for _, p := range providers {
		parts = append(parts, fmt.Sprintf("%s/%s", p.Provider, p.Model))
	}
	return strings.Join(parts, ", ")
}
