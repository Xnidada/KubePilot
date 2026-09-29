package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStreamUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"include_usage":true`) {
			t.Error("stream usage was not requested")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAIClient(&LLMConfig{BaseURL: server.URL, APIKey: "test", Model: "test"})
	chunks, err := client.ChatStream(context.Background(), &ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	var usage Usage
	var done bool
	for chunk := range chunks {
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		done = done || chunk.Done
	}
	if !done || usage.PromptTokens != 12 || usage.CompletionTokens != 3 || usage.TotalTokens != 15 {
		t.Fatalf("done=%v usage=%+v", done, usage)
	}
}

func TestAnthropicStreamUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":8}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":4}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()
	client := NewAnthropicClient(&LLMConfig{BaseURL: server.URL, APIKey: "test", Model: "test"})
	chunks, err := client.ChatStream(context.Background(), &ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	var usage Usage
	var done bool
	for chunk := range chunks {
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		done = done || chunk.Done
	}
	if !done || usage.PromptTokens != 8 || usage.CompletionTokens != 4 || usage.TotalTokens != 12 {
		t.Fatalf("done=%v usage=%+v", done, usage)
	}
}
