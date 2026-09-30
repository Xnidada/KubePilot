package aiops

import (
	"context"
	"sync"
	"testing"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
)

type fixedChatClient struct{ chunks []llm.StreamChunk }

func (*fixedChatClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func (c *fixedChatClient) ChatStream(context.Context, *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	out := make(chan llm.StreamChunk, len(c.chunks))
	for _, chunk := range c.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

func TestChatHistoryIsScopedByUserAndCluster(t *testing.T) {
	ctx := context.Background()
	store := cache.NewMemoryCache()
	defer store.Close()
	s := &Service{cache: store}
	s.saveChatHistoryToCache(ctx, 1, 10, []llm.Message{{Role: "user", Content: "cluster 10"}})
	s.saveChatHistoryToCache(ctx, 1, 20, []llm.Message{{Role: "user", Content: "cluster 20"}})
	for _, tc := range []struct {
		user, cluster uint
		want          string
	}{
		{1, 10, "cluster 10"}, {1, 20, "cluster 20"}, {1, 0, ""}, {2, 10, ""},
	} {
		history := s.getChatHistory(ctx, tc.user, tc.cluster)
		got := ""
		if len(history) > 0 {
			got = history[0].Content
		}
		if got != tc.want {
			t.Fatalf("user=%d cluster=%d: got %q, want %q", tc.user, tc.cluster, got, tc.want)
		}
	}
}

func TestChatStreamForwardsEveryChunkAndSavesFullReply(t *testing.T) {
	ctx := context.Background()
	s := &Service{llmClient: &fixedChatClient{chunks: []llm.StreamChunk{
		{Content: "one"}, {Content: "two"}, {Content: "three"}, {Done: true},
	}}}
	stream, err := s.ChatStream(ctx, 7, &ChatRequest{Message: "question"})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []llm.StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 4 || !chunks[3].Done || chunks[0].Content+chunks[1].Content+chunks[2].Content != "onetwothree" {
		t.Fatalf("incomplete stream: %+v", chunks)
	}
	history := s.getChatHistory(ctx, 7, 0)
	if len(history) != 2 || history[1].Content != "onetwothree" {
		t.Fatalf("incomplete saved history: %+v", history)
	}
}

func TestChatHistoryConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := &Service{}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				s.saveChatHistoryToCache(ctx, uint(worker%2+1), uint(i%3), []llm.Message{{Content: "reply"}})
				_ = s.getChatHistory(ctx, uint(worker%2+1), uint(i%3))
			}
		}(worker)
	}
	wg.Wait()
}
