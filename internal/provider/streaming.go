// Package provider defines the provider interface and implementations
package provider

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"sync"
)

// maxTokenSize defines the maximum token size for streaming buffer
const maxTokenSize = 1024 * 1024 // 1MB

var (
	// streamBufferPool is a pool for reusing streaming buffers (1MB each)
	streamBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, maxTokenSize)
			return &buf
		},
	}
)

// streamResponse is a generic streaming helper that reads from an HTTP response
// and sends parsed responses to a channel.
// The parseFunc receives the data string (already stripped of "data: " prefix) and
// should return the parsed response and any error (errors are skipped).
// The isDoneFunc checks if the data indicates streaming is complete.
func streamResponse[T any](ctx context.Context, resp *http.Response, parseFunc func(data string) (T, error), isDoneFunc func(data string) bool) <-chan T {
	ch := make(chan T, 10)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Get buffer from pool for large streaming responses
		bufPtr := streamBufferPool.Get().(*[]byte)
		defer streamBufferPool.Put(bufPtr)
		scanner.Buffer(*bufPtr, maxTokenSize)

		for scanner.Scan() {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if isDoneFunc(data) {
				break
			}

			parsed, err := parseFunc(data)
			if err != nil {
				continue
			}

			// Non-blocking send with context check
			select {
			case ch <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
