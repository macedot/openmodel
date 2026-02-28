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

type TestResult struct {
	Model    string        `json:"model"`
	Backend  string        `json:"backend"`
	Chat     *MethodResult `json:"chat,omitempty"`
	Complete *MethodResult `json:"complete,omitempty"`
	Embed    *MethodResult `json:"embed,omitempty"`
}

type MethodResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Latency string `json:"latency"`
}

type TestSummary struct {
	TotalTests int          `json:"total_tests"`
	Passed     int          `json:"passed"`
	Failed     int          `json:"failed"`
	Results    []TestResult `json:"results"`
}

func main() {
	command := "serve"
	args := os.Args[1:]

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

	if command == "test" {
		modelName := flag.String("model", "", "Model name to test (tests all if omitted)")
		jsonOutput := flag.Bool("check", false, "Output results in JSON format")

		err := flag.CommandLine.Parse(args)
		if err != nil {
			if err == flag.ErrHelp {
				printTestUsage()
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			printTestUsage()
			os.Exit(1)
		}
		runTest(modelName, jsonOutput)
		return
	}

	if command != "serve" {
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}

	flag.CommandLine.Init("serve", flag.ContinueOnError)
	err := flag.CommandLine.Parse(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printServerUsage()
		os.Exit(1)
	}
	runServer()
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [command]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  serve    Start the OpenModel server (default)\n")
	fmt.Fprintf(os.Stderr, "  test     Test configured models\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  -h, --help    Show help\n")
	fmt.Fprintf(os.Stderr, "\nRun '%s <command> -h' for more information on a command.\n", os.Args[0])
}

func runServer() {
	serveFlag := flag.Bool("h", false, "Show help")
	if *serveFlag {
		printServerUsage()
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := logger.Init(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	backends := make(map[string]provider.Provider)
	for name, bc := range cfg.Backends {
		backends[name] = provider.NewOpenAIProvider(name, bc.URL, bc.APIKey)
		logger.Info("Backend initialized", "name", name, "url", bc.URL)
	}

	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := server.New(cfg, backends, stateMgr)

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
	modelName := flag.String("model", "", "Model name to test (tests all if omitted)")
	_ = modelName
	jsonOutput := flag.Bool("check", false, "Output results in JSON format")
	_ = jsonOutput
	fmt.Fprintf(os.Stderr, "Usage: %s test [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func printServerUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s serve [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func runTest(modelName *string, jsonOutput *bool) {
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

	logger.Info("Initializing backends")

	backends := make(map[string]provider.Provider)
	for name, bc := range cfg.Backends {
		backends[name] = provider.NewOpenAIProvider(name, bc.URL, bc.APIKey)
		logger.Info("Backend initialized", "name", name, "url", bc.URL)
	}

	modelsToTest := getModelsToTest(cfg.Models, *modelName)

	if *modelName != "" {
		logger.Info("Testing model", "model", *modelName)
	} else {
		logger.Info("Testing all configured models")
	}

	summary := runTests(backends, modelsToTest)

	if *jsonOutput {
		printJSON(summary)
	} else {
		printText(summary)
	}

	logger.Info("Test completed", "total", summary.TotalTests, "passed", summary.Passed, "failed", summary.Failed)

	if summary.Failed > 0 {
		os.Exit(1)
	}
}

func getModelsToTest(models map[string][]config.ModelBackend, targetModel string) map[string][]config.ModelBackend {
	if targetModel == "" {
		return models
	}

	result := make(map[string][]config.ModelBackend)
	if backends, exists := models[targetModel]; exists {
		result[targetModel] = backends
	}
	return result
}

func runTests(backends map[string]provider.Provider, models map[string][]config.ModelBackend) TestSummary {
	summary := TestSummary{
		Results: make([]TestResult, 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for modelName, modelBackends := range models {
		logger.Info("Testing model", "model", modelName)

		for _, backend := range modelBackends {
			backendKey := fmt.Sprintf("%s/%s", backend.Backend, backend.Model)
			summary.TotalTests++

			logger.Info("Testing backend", "backend", backendKey)

			result := TestResult{
				Model:   modelName,
				Backend: backendKey,
			}

			prov, exists := backends[backend.Backend]
			if !exists {
				logger.Error("Backend not found", "backend", backend.Backend)
				result.Chat = &MethodResult{Success: false, Error: "backend not found"}
				result.Complete = &MethodResult{Success: false, Error: "backend not found"}
				result.Embed = &MethodResult{Success: false, Error: "backend not found"}
				summary.Failed += 3
				summary.Results = append(summary.Results, result)
				continue
			}

			result.Chat = testChat(ctx, prov, backend.Model)
			if result.Chat.Success {
				summary.Passed++
				logger.Info("Chat test passed", "backend", backendKey, "latency", result.Chat.Latency)
			} else {
				summary.Failed++
				logger.Error("Chat test failed", "backend", backendKey, "error", result.Chat.Error)
			}

			result.Complete = testComplete(ctx, prov, backend.Model)
			if result.Complete.Success {
				summary.Passed++
				logger.Info("Complete test passed", "backend", backendKey, "latency", result.Complete.Latency)
			} else {
				summary.Failed++
				logger.Error("Complete test failed", "backend", backendKey, "error", result.Complete.Error)
			}

			result.Embed = testEmbed(ctx, prov, backend.Model)
			if result.Embed.Success {
				summary.Passed++
				logger.Info("Embed test passed", "backend", backendKey, "latency", result.Embed.Latency)
			} else {
				summary.Failed++
				logger.Error("Embed test failed", "backend", backendKey, "error", result.Embed.Error)
			}

			summary.Results = append(summary.Results, result)
		}
	}

	return summary
}

func testChat(ctx context.Context, prov provider.Provider, model string) *MethodResult {
	start := time.Now()
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "Hello, respond with 'OK' if you can read this."},
	}

	_, err := prov.Chat(ctx, model, messages, nil)
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

func testComplete(ctx context.Context, prov provider.Provider, model string) *MethodResult {
	start := time.Now()

	req := &openai.CompletionRequest{
		Prompt: "Say 'OK' if you can read this.",
	}

	_, err := prov.Complete(ctx, model, req)
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

func testEmbed(ctx context.Context, prov provider.Provider, model string) *MethodResult {
	start := time.Now()

	_, err := prov.Embed(ctx, model, []string{"test", "hello world"})
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

func printText(summary TestSummary) {
	fmt.Println()
	fmt.Println("==============================================")
	fmt.Println("           Model Test Results                ")
	fmt.Println("==============================================")
	fmt.Println()

	for _, result := range summary.Results {
		fmt.Printf("Model: %s | Backend: %s\n", result.Model, result.Backend)
		fmt.Println(strings.Repeat("-", 50))

		if result.Chat != nil {
			status := "PASS"
			if !result.Chat.Success {
				status = "FAIL"
			}
			fmt.Printf("  Chat:      [%s] %s\n", status, result.Chat.Latency)
			if !result.Chat.Success {
				fmt.Printf("             Error: %s\n", result.Chat.Error)
			}
		}

		if result.Complete != nil {
			status := "PASS"
			if !result.Complete.Success {
				status = "FAIL"
			}
			fmt.Printf("  Complete:  [%s] %s\n", status, result.Complete.Latency)
			if !result.Complete.Success {
				fmt.Printf("             Error: %s\n", result.Complete.Error)
			}
		}

		if result.Embed != nil {
			status := "PASS"
			if !result.Embed.Success {
				status = "FAIL"
			}
			fmt.Printf("  Embed:     [%s] %s\n", status, result.Embed.Latency)
			if !result.Embed.Success {
				fmt.Printf("             Error: %s\n", result.Embed.Error)
			}
		}

		fmt.Println()
	}

	fmt.Println("==============================================")
	fmt.Printf("Total: %d | Passed: %d | Failed: %d\n", summary.TotalTests, summary.Passed, summary.Failed)
	fmt.Println("==============================================")
}

func printJSON(summary TestSummary) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
