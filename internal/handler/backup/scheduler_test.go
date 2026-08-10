package backup

import (
	"testing"

	"github.com/kubepilot/kubepilot/internal/model"
	"go.uber.org/zap"
)

type captureRunner struct {
	calls int
}

func (c *captureRunner) RunScheduledBackup(schedule *model.BackupSchedule) {
	c.calls++
}

func TestSchedulerAddRemove(t *testing.T) {
	runner := &captureRunner{}
	sched := NewScheduler(nil, zap.NewNop(), runner)

	s := model.BackupSchedule{
		ID:        42,
		Name:      "t",
		ClusterID: 1,
		Schedule:  "@every 1h",
		Status:    "active",
		TTL:       "720h",
	}
	if err := sched.Add(s); err != nil {
		t.Fatal(err)
	}
	if sched.ActiveCount() != 1 {
		t.Fatalf("active=%d", sched.ActiveCount())
	}

	// Re-add replaces entry
	if err := sched.Add(s); err != nil {
		t.Fatal(err)
	}
	if sched.ActiveCount() != 1 {
		t.Fatalf("active after replace=%d", sched.ActiveCount())
	}

	sched.Remove(s.ID)
	if sched.ActiveCount() != 0 {
		t.Fatalf("expected 0 after remove, got %d", sched.ActiveCount())
	}
}

func TestSchedulerSyncPauseAndEmpty(t *testing.T) {
	sched := NewScheduler(nil, zap.NewNop(), &captureRunner{})
	s := model.BackupSchedule{
		ID:       7,
		Schedule: "@every 1h",
		Status:   "active",
	}
	if err := sched.Add(s); err != nil {
		t.Fatal(err)
	}
	s.Status = "paused"
	if err := sched.Sync(s); err != nil {
		t.Fatal(err)
	}
	if sched.ActiveCount() != 0 {
		t.Fatalf("expected 0 after pause sync, got %d", sched.ActiveCount())
	}

	s.Status = "active"
	s.Schedule = ""
	if err := sched.Sync(s); err != nil {
		t.Fatal(err)
	}
	if sched.ActiveCount() != 0 {
		t.Fatalf("expected 0 after empty sync, got %d", sched.ActiveCount())
	}
}

func TestSchedulerRejectsInvalidCron(t *testing.T) {
	sched := NewScheduler(nil, zap.NewNop(), &captureRunner{})
	err := sched.Add(model.BackupSchedule{
		ID:       1,
		Schedule: "not a cron",
		Status:   "active",
	})
	if err == nil {
		t.Fatal("expected invalid cron error")
	}
}
