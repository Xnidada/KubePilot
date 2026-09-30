package aiops

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/testutil"
	"gorm.io/gorm"
)

func TestPostgresRuntimeConfigRefreshAndBillingSnapshot(t *testing.T) {
	db := testutil.Postgres(t, &model.LLMConfig{}, &model.TokenUsageLog{})
	config := model.LLMConfig{Provider: "openai", APIKey: "test-only", Model: "old-model", IsActive: true, InputPricePerM: 1, OutputPricePerM: 2}
	if err := db.Create(&config).Error; err != nil {
		t.Fatal(err)
	}
	a, b := &Service{db: db}, &Service{db: db}
	ctx, client, err := a.requestLLM(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&config).Updates(map[string]any{"model": "new-model", "input_price_per_m": 9}).Error; err != nil {
		t.Fatal(err)
	}
	for _, service := range []*Service{a, b} {
		fresh, _, err := service.requestLLM(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got := fresh.Value(runtimeConfigKey{}).(runtimeConfig).config; got.Model != "new-model" || got.InputPricePerM != 9 {
			t.Fatalf("replica used stale config: %+v", got)
		}
	}
	_, pinned, err := b.requestLLM(ctx)
	if err != nil || pinned != client {
		t.Fatalf("in-flight client changed: %v", err)
	}
	a.persistTokenUsage(ctx, 1, 0, llm.Usage{PromptTokens: 1000000, CompletionTokens: 1000000, TotalTokens: 2000000}, "agent")
	var usage model.TokenUsageLog
	if err := db.First(&usage).Error; err != nil {
		t.Fatal(err)
	}
	if usage.Model != "old-model" || usage.CostEstimate == nil || *usage.CostEstimate != 3 {
		t.Fatalf("usage was billed against later configuration: %+v", usage)
	}
}

func TestPostgresDefaultConfigSerializesAndRollsBack(t *testing.T) {
	db := testutil.Postgres(t, &model.LLMConfig{})
	configs := []model.LLMConfig{{Provider: "openai", Model: "a", IsActive: true}, {Provider: "openai", Model: "b", IsActive: true}}
	if err := db.Create(&configs).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.EnsureSingleActiveLLMConfig(db); err != nil {
		t.Fatal(err)
	}
	var active model.LLMConfig
	if err := db.Where("is_active = true").First(&active).Error; err != nil || active.ID != configs[1].ID {
		t.Fatalf("legacy repair: %v %+v", err, active)
	}
	failure := errors.New("simulated failed insert")
	if err := model.WithLLMConfigWrite(db, func(tx *gorm.DB) error {
		if err := tx.Model(&model.LLMConfig{}).Where("is_active = true").Update("is_active", false).Error; err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("missing transaction error: %v", err)
	}
	var n int64
	if err := db.Model(&model.LLMConfig{}).Where("is_active = true").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("rollback lost default: %d %v", n, err)
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			errorsCh <- model.WithLLMConfigWrite(db, func(tx *gorm.DB) error {
				if err := tx.Model(&model.LLMConfig{}).Where("is_active = true").Update("is_active", false).Error; err != nil {
					return err
				}
				return tx.Model(&model.LLMConfig{}).Where("id = ?", id).Update("is_active", true).Error
			})
		}(configs[i%2].ID)
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.LLMConfig{}).Where("is_active = true").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("concurrent switches: %d %v", n, err)
	}
	if err := db.Model(&model.LLMConfig{}).Where("is_active = false").Update("is_active", true).Error; err == nil {
		t.Fatal("database allowed two defaults")
	}
}

func TestAgentStreamEmitsConfigurationFailure(t *testing.T) {
	var events []AgentStreamEvent
	err := (&Service{}).AgentChatStream(context.Background(), 1, 1, 0, "hello", false, func(event AgentStreamEvent) { events = append(events, event) })
	if err == nil || len(events) != 1 || events[0].Type != "error" {
		t.Fatalf("failure lost by SSE: %v %+v", err, events)
	}
}
