package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeInboundLimits_DisabledByDefault(t *testing.T) {
	cfg := &Config{}
	cfg.SanitizeInboundLimits()

	if cfg.InboundLimits.Enabled {
		t.Fatal("expected inbound limits to remain disabled")
	}
	if cfg.InboundLimits.GlobalConcurrency != 0 {
		t.Fatalf("global concurrency = %d, want 0", cfg.InboundLimits.GlobalConcurrency)
	}
}

func TestSanitizeInboundLimits_DefaultsWhenEnabled(t *testing.T) {
	cfg := &Config{
		InboundLimits: InboundLimitsConfig{
			Enabled:          true,
			RejectStatusCode: 200,
			EndpointOverrides: []InboundLimitOverride{
				{PathPrefix: "/v1/images/generations"},
				{PathPrefix: "relative"},
				{PathPrefix: ""},
			},
		},
	}

	cfg.SanitizeInboundLimits()

	if cfg.InboundLimits.GlobalConcurrency != DefaultInboundGlobalConcurrency {
		t.Fatalf("global concurrency = %d, want %d", cfg.InboundLimits.GlobalConcurrency, DefaultInboundGlobalConcurrency)
	}
	if cfg.InboundLimits.PerAPIKeyConcurrency != DefaultInboundPerAPIKeyConcurrency {
		t.Fatalf("per-key concurrency = %d, want %d", cfg.InboundLimits.PerAPIKeyConcurrency, DefaultInboundPerAPIKeyConcurrency)
	}
	if cfg.InboundLimits.RejectStatusCode != DefaultInboundRejectStatusCode {
		t.Fatalf("reject status = %d, want %d", cfg.InboundLimits.RejectStatusCode, DefaultInboundRejectStatusCode)
	}
	if cfg.InboundLimits.RejectMessage != DefaultInboundRejectMessage {
		t.Fatalf("reject message = %q, want %q", cfg.InboundLimits.RejectMessage, DefaultInboundRejectMessage)
	}
	if len(cfg.InboundLimits.EndpointOverrides) != 1 {
		t.Fatalf("override count = %d, want 1", len(cfg.InboundLimits.EndpointOverrides))
	}
	override := cfg.InboundLimits.EndpointOverrides[0]
	if override.GlobalConcurrency != DefaultInboundGlobalConcurrency {
		t.Fatalf("override global concurrency = %d, want %d", override.GlobalConcurrency, DefaultInboundGlobalConcurrency)
	}
	if override.PerAPIKeyConcurrency != DefaultInboundPerAPIKeyConcurrency {
		t.Fatalf("override per-key concurrency = %d, want %d", override.PerAPIKeyConcurrency, DefaultInboundPerAPIKeyConcurrency)
	}
}

func TestLoadConfigOptional_InboundLimits(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	data := []byte(`
inbound-limits:
  enabled: true
  global-concurrency: 10
  per-api-key-concurrency: 5
  reject-status-code: 429
  reject-message: "busy"
  endpoint-overrides:
    - path-prefix: "/v1/images/generations"
      global-concurrency: 2
      per-api-key-concurrency: 1
`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}

	if !cfg.InboundLimits.Enabled {
		t.Fatal("expected inbound limits to be enabled")
	}
	if cfg.InboundLimits.GlobalConcurrency != 10 {
		t.Fatalf("global concurrency = %d, want 10", cfg.InboundLimits.GlobalConcurrency)
	}
	if cfg.InboundLimits.PerAPIKeyConcurrency != 5 {
		t.Fatalf("per-key concurrency = %d, want 5", cfg.InboundLimits.PerAPIKeyConcurrency)
	}
	if cfg.InboundLimits.RejectMessage != "busy" {
		t.Fatalf("reject message = %q, want busy", cfg.InboundLimits.RejectMessage)
	}
	if len(cfg.InboundLimits.EndpointOverrides) != 1 {
		t.Fatalf("override count = %d, want 1", len(cfg.InboundLimits.EndpointOverrides))
	}
}
