package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type streamTestTransport func(*http.Request) (*http.Response, error)

func (f streamTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedStreamBody struct {
	io.Reader
	closed chan struct{}
	once   sync.Once
}

func (b *trackedStreamBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestFullStreamBufferCanBeCanceled(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			frame := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"
			if provider == "anthropic" {
				frame = "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n\n"
			}
			body := &trackedStreamBody{Reader: strings.NewReader(strings.Repeat(frame, 150)), closed: make(chan struct{})}
			transport := streamTestTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
			})
			var client Client
			if provider == "openai" {
				c := NewOpenAIClient(&LLMConfig{})
				c.httpClient.Transport = transport
				client = c
			} else {
				c := NewAnthropicClient(&LLMConfig{})
				c.httpClient.Transport = transport
				client = c
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			chunks, err := client.ChatStream(ctx, &ChatRequest{})
			if err != nil {
				t.Fatal(err)
			}
			// Only drain on cleanup: draining before the assertion would hide a
			// producer stuck on an unconditional channel send.
			defer func() {
				cancel()
				for range chunks {
				}
			}()
			deadline := time.Now().Add(2 * time.Second)
			for len(chunks) != cap(chunks) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if len(chunks) != cap(chunks) {
				t.Fatal("stream did not fill buffer")
			}
			cancel()
			select {
			case <-body.closed:
			case <-time.After(2 * time.Second):
				t.Fatal("canceled producer did not close response body with a full buffer")
			}
		})
	}
}
