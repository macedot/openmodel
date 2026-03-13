// Package server implements the HTTP server and handlers
package server

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/server/converters"
)

// streamWithFailover handles streaming requests with failover and format conversion
func (s *Server) streamWithFailover(c *fiber.Ctx, model string, body []byte, headers map[string]string, ctx context.Context, sourceFormat, targetFormat converters.APIFormat) error {
	var triedProviders []string
	requestID, _ := c.Locals("request_id").(string)

	// Get converter if needed
	var converter converters.StreamConverter
	var hasConverter bool
	if sourceFormat != targetFormat {
		converter, hasConverter = converters.GetConverter(sourceFormat, targetFormat)
		if !hasConverter {
			handleError(c, fmt.Sprintf("no converter available for %s to %s", sourceFormat, targetFormat), fiber.StatusInternalServerError)
			return nil
		}
	}

	for {
		prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
		if err != nil {
			applogger.Error("all_providers_failed",
				"request_id", requestID,
				"model", model,
				"providers_tried", triedProviders,
				"error", err.Error())
			s.handleAllProvidersFailedFiber(c, fmt.Errorf("model %q temporarily unavailable: all providers failed", model))
			return nil
		}

		triedProviders = append(triedProviders, providerKey)
		threshold := s.GetConfig().GetThresholds(providerKey).FailuresBeforeSwitch

		// Log request processing
		applogger.Debug("PROCESSING", "request_id", requestID, "provider", providerKey, "model", model)

		// Store provider in context for logging
		c.Locals("provider", providerKey)
		c.Locals("model", model)

		// Replace model name in body
		provBody := replaceModelInBody(body, providerModel)

		// Merge headers from converter if present
		streamHeaders := headers
		if hasConverter {
			converterHeaders := converter.GetHeaders()
			if len(converterHeaders) > 0 {
				streamHeaders = make(map[string]string)
				for k, v := range headers {
					streamHeaders[k] = v
				}
				for k, v := range converterHeaders {
					streamHeaders[k] = v
				}
			}
		}

		// Determine endpoint
		endpoint := EndpointV1ChatCompletions
		if sourceFormat == converters.APIFormatAnthropic {
			endpoint = EndpointV1Messages
		}
		if hasConverter {
			endpoint = converter.GetEndpoint(endpoint)
		}

		// Set streaming headers
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer w.Flush()

			stream, err := prov.DoStreamRequest(ctx, endpoint, provBody, streamHeaders)
			if err != nil {
				applogger.Warn("provider_stream_failed",
					"request_id", requestID,
					"provider", providerKey,
					"error", err.Error())
				s.state.RecordFailure(providerKey, threshold)
				return
			}

			// Track state for stream conversion
			streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
			isFirst := true
			blockIdx := 0

			for line := range stream {
				lineStr := string(line)

				// Convert stream format if converter is present
				if hasConverter {
					state := &converters.StreamState{
						IsFirst:  &isFirst,
						BlockIdx: &blockIdx,
					}
					converted := converter.ConvertStreamLine(lineStr, model, streamID, state)
					if converted == "" {
						continue // Skip events that have no equivalent
					}
					if _, err := fmt.Fprintf(w, "%s\n", converted); err != nil {
						applogger.Info("client_disconnected", "request_id", requestID, "provider", providerKey)
						return
					}
				} else {
					// Passthrough - write line as-is
					if _, err := fmt.Fprintf(w, "%s\n", lineStr); err != nil {
						applogger.Info("client_disconnected", "request_id", requestID, "provider", providerKey)
						return
					}
				}
				w.Flush()
			}

			// Write [DONE] marker for OpenAI format streams
			if sourceFormat == converters.APIFormatOpenAI && targetFormat == converters.APIFormatOpenAI {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				w.Flush()
			}

			s.state.ResetModel(providerKey)
		})
		return nil
	}
}
