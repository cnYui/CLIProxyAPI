package config

import "strings"

const (
	DefaultInboundGlobalConcurrency    = 75
	DefaultInboundPerAPIKeyConcurrency = 5
	DefaultInboundRejectStatusCode     = 429
	DefaultInboundRejectMessage        = "too many concurrent requests"
)

// InboundLimitsConfig controls inbound application-layer concurrency limits.
type InboundLimitsConfig struct {
	Enabled              bool                   `yaml:"enabled" json:"enabled"`
	GlobalConcurrency    int                    `yaml:"global-concurrency" json:"global-concurrency"`
	PerAPIKeyConcurrency int                    `yaml:"per-api-key-concurrency" json:"per-api-key-concurrency"`
	RejectStatusCode     int                    `yaml:"reject-status-code" json:"reject-status-code"`
	RejectMessage        string                 `yaml:"reject-message" json:"reject-message"`
	EndpointOverrides    []InboundLimitOverride `yaml:"endpoint-overrides" json:"endpoint-overrides"`
}

// InboundLimitOverride overrides inbound concurrency limits for matching paths.
type InboundLimitOverride struct {
	PathPrefix           string `yaml:"path-prefix" json:"path-prefix"`
	GlobalConcurrency    int    `yaml:"global-concurrency" json:"global-concurrency"`
	PerAPIKeyConcurrency int    `yaml:"per-api-key-concurrency" json:"per-api-key-concurrency"`
}

// SanitizeInboundLimits normalizes inbound limit settings without enabling them implicitly.
func (cfg *Config) SanitizeInboundLimits() {
	if cfg == nil {
		return
	}
	cfg.InboundLimits = SanitizeInboundLimitsConfig(cfg.InboundLimits)
}

// SanitizeInboundLimitsConfig normalizes inbound limit settings.
func SanitizeInboundLimitsConfig(limits InboundLimitsConfig) InboundLimitsConfig {
	if !limits.Enabled {
		return InboundLimitsConfig{}
	}

	if limits.GlobalConcurrency <= 0 {
		limits.GlobalConcurrency = DefaultInboundGlobalConcurrency
	}
	if limits.PerAPIKeyConcurrency <= 0 {
		limits.PerAPIKeyConcurrency = DefaultInboundPerAPIKeyConcurrency
	}
	if limits.RejectStatusCode < 400 || limits.RejectStatusCode > 599 {
		limits.RejectStatusCode = DefaultInboundRejectStatusCode
	}
	limits.RejectMessage = strings.TrimSpace(limits.RejectMessage)
	if limits.RejectMessage == "" {
		limits.RejectMessage = DefaultInboundRejectMessage
	}

	overrides := make([]InboundLimitOverride, 0, len(limits.EndpointOverrides))
	for _, override := range limits.EndpointOverrides {
		prefix := strings.TrimSpace(override.PathPrefix)
		if prefix == "" || !strings.HasPrefix(prefix, "/") {
			continue
		}
		override.PathPrefix = prefix
		if override.GlobalConcurrency <= 0 {
			override.GlobalConcurrency = limits.GlobalConcurrency
		}
		if override.PerAPIKeyConcurrency <= 0 {
			override.PerAPIKeyConcurrency = limits.PerAPIKeyConcurrency
		}
		overrides = append(overrides, override)
	}
	limits.EndpointOverrides = overrides

	return limits
}
