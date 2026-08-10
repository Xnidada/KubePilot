package modules

import (
	"github.com/kubepilot/kubepilot/internal/module"
	"github.com/kubepilot/kubepilot/internal/modules/aiops"
	"github.com/kubepilot/kubepilot/internal/modules/appstore"
	"github.com/kubepilot/kubepilot/internal/modules/backup"
	"github.com/kubepilot/kubepilot/internal/modules/eventforward"
	"github.com/kubepilot/kubepilot/internal/modules/inspection"
	"github.com/kubepilot/kubepilot/internal/modules/scheduler"
	"github.com/kubepilot/kubepilot/internal/modules/webhook"
)

// RegisterAll registers the first-batch in-process feature modules.
func RegisterAll(reg *module.Registry) {
	reg.MustRegister(aiops.New())
	reg.MustRegister(inspection.New())
	reg.MustRegister(eventforward.New())
	reg.MustRegister(scheduler.New())
	reg.MustRegister(backup.New())
	reg.MustRegister(webhook.New())
	reg.MustRegister(appstore.New())
}
