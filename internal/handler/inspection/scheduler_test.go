package inspection

import (
	"testing"

	"github.com/kubepilot/kubepilot/internal/model"
)

func TestValidateCron(t *testing.T) {
	if err := ValidateCron(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCron("@every 1m"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCron("0 * * * *"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCron("not-a-cron"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSchedulerAddRemove(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	rule := model.InspectionRule{ID: 7, Schedule: "@every 1h", Enabled: true}
	if err := s.Add(rule); err != nil {
		t.Fatal(err)
	}
	if s.ActiveCount() != 1 {
		t.Fatalf("active=%d", s.ActiveCount())
	}
	if err := s.Sync(model.InspectionRule{ID: 7, Schedule: "", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if s.ActiveCount() != 0 {
		t.Fatalf("expected removed, got %d", s.ActiveCount())
	}
}
