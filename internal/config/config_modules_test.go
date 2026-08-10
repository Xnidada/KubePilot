package config

import "testing"

func TestModuleEnabledDefaultsTrue(t *testing.T) {
	cfg := &Config{}
	if !cfg.ModuleEnabled("aiops") {
		t.Fatal("expected default enabled")
	}
}

func TestModuleSettings(t *testing.T) {
	cfg := &Config{
		Modules: map[string]ModuleConfig{
			"eventforward": {
				FailRateThreshold: 0.8,
				MinMatched:        10,
				HealthSustain:     "1m",
			},
		},
	}
	mc := cfg.ModuleSettings("eventforward")
	if mc.FailRateThreshold != 0.8 || mc.MinMatched != 10 || mc.HealthSustain != "1m" {
		t.Fatalf("unexpected settings: %+v", mc)
	}
	empty := cfg.ModuleSettings("missing")
	if empty.FailRateThreshold != 0 {
		t.Fatal("missing module should return zero settings")
	}
}
