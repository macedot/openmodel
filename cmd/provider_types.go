package main

import (
	"context"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/provider"
)

type benchProvider interface {
	provider.URLProvider
	provider.APIModeProvider
	provider.RawRequester

	Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error)
	StreamChat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (<-chan openai.ChatCompletionResponse, error)
}

type benchProviderMap map[string]benchProvider

func asBenchProviderMap(providers map[string]provider.Provider) benchProviderMap {
	benchProviders := make(benchProviderMap, len(providers))
	for name, prov := range providers {
		benchProviders[name] = prov
	}
	return benchProviders
}
