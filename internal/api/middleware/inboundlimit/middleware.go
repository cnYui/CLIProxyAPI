package inboundlimit

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	log "github.com/sirupsen/logrus"
)

// Middleware enforces inbound concurrency limits and can be updated on config reload.
type Middleware struct {
	mu      sync.RWMutex
	cfg     runtimeConfig
	limiter *Limiter
}

// NewMiddleware builds an inbound limiting middleware from config.
func NewMiddleware(cfg config.InboundLimitsConfig) *Middleware {
	m := &Middleware{}
	m.SetConfig(cfg)
	return m
}

// SetConfig replaces the runtime limiter config. In-flight requests release against the old limiter.
func (m *Middleware) SetConfig(cfg config.InboundLimitsConfig) {
	if m == nil {
		return
	}
	runtimeCfg := normalizeConfig(cfg)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg = runtimeCfg
	if !runtimeCfg.enabled {
		m.limiter = nil
		return
	}
	m.limiter = newLimiter(runtimeCfg)
}

// Handler returns the Gin middleware function.
func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter, cfg := m.snapshot()
		if limiter == nil || !cfg.enabled {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		principal := principalFromContext(c)
		result, release := limiter.tryAcquire(path, principal)
		if !result.acquired {
			logRejection(c, principal, result)
			c.AbortWithStatusJSON(cfg.rejectStatusCode, rejectResponse(cfg.rejectMessage))
			return
		}
		defer release()

		c.Next()
	}
}

func (m *Middleware) snapshot() (*Limiter, runtimeConfig) {
	if m == nil {
		return nil, runtimeConfig{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.limiter, m.cfg
}

func principalFromContext(c *gin.Context) string {
	if c == nil {
		return "unknown"
	}
	if value, ok := c.Get("apiKey"); ok {
		if principal, ok := value.(string); ok && principal != "" {
			return principal
		}
	}
	if c.ClientIP() != "" {
		return "anonymous:" + c.ClientIP()
	}
	return "unknown"
}

func logRejection(c *gin.Context, principal string, result acquireResult) {
	entry := log.WithFields(log.Fields{
		"path":                 c.Request.URL.Path,
		"method":               c.Request.Method,
		"api_key_hash":         principalHash(principal),
		"active_global":        result.globalActive,
		"active_per_key":       result.keyActive,
		"active_scope":         result.scopeActive,
		"active_scope_per_key": result.scopeKeyActive,
		"global_limit":         result.globalLimit,
		"per_api_key_limit":    result.keyLimit,
		"scope_limit":          result.scopeLimit,
		"scope_per_key_limit":  result.scopeKeyLimit,
		"reason":               result.reason,
		"matched_path_prefix":  result.pathPrefix,
	})
	if requestID := logging.GetGinRequestID(c); requestID != "" {
		entry = entry.WithField("request_id", requestID)
	}
	entry.Warn("inbound request rejected by concurrency limiter")
}

func principalHash(principal string) string {
	sum := sha256.Sum256([]byte(principal))
	return hex.EncodeToString(sum[:])[:12]
}

func rejectResponse(message string) gin.H {
	return gin.H{
		"error": gin.H{
			"message": message,
			"type":    "rate_limit_error",
			"code":    "rate_limit_exceeded",
		},
	}
}
