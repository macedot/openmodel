package server

import "github.com/macedot/openmodel/internal/provider"

type requestProvider interface {
	provider.RawRequester
	provider.APIModeProvider
	provider.URLProvider
	Name() string
	Close() error
}

type providerMap map[string]requestProvider

func cloneProviderMap(providers providerMap) providerMap {
	cloned := make(providerMap, len(providers))
	for key, value := range providers {
		cloned[key] = value
	}
	return cloned
}

// asProviderMap narrows the broad provider.Provider dependency to the server-facing capability set.
func asProviderMap(providers map[string]provider.Provider) providerMap {
	narrowed := make(providerMap, len(providers))
	for key, value := range providers {
		narrowed[key] = value
	}
	return narrowed
}

var _ requestProvider = provider.Provider(nil)
