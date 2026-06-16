package inboundlimit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
)

func TestMiddleware_DisabledPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mw := NewMiddleware(config.InboundLimitsConfig{})
	engine := gin.New()
	engine.Use(mw.Handler())
	engine.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestMiddleware_RejectsWithOpenAICompatibleBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mw := NewMiddleware(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    10,
		PerAPIKeyConcurrency: 1,
		RejectStatusCode:     http.StatusTooManyRequests,
		RejectMessage:        "busy",
	})

	limiter, _ := mw.snapshot()
	held, release := limiter.tryAcquire("/v1/chat/completions", "secret-key")
	if !held.acquired {
		t.Fatalf("setup acquire rejected: %+v", held)
	}
	defer release()

	var logBuf bytes.Buffer
	originalOutput := log.StandardLogger().Out
	log.SetOutput(&logBuf)
	defer log.SetOutput(originalOutput)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("apiKey", "secret-key")
		c.Next()
	})
	engine.Use(mw.Handler())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}

	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Message != "busy" {
		t.Fatalf("message = %q, want busy", body.Error.Message)
	}
	if body.Error.Type != "rate_limit_error" {
		t.Fatalf("type = %q, want rate_limit_error", body.Error.Type)
	}
	if body.Error.Code != "rate_limit_exceeded" {
		t.Fatalf("code = %q, want rate_limit_exceeded", body.Error.Code)
	}
	if strings.Contains(logBuf.String(), "secret-key") {
		t.Fatalf("log output leaked raw key: %s", logBuf.String())
	}
}

func TestMiddleware_AnonymousPrincipalUsesClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mw := NewMiddleware(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    10,
		PerAPIKeyConcurrency: 1,
		RejectStatusCode:     http.StatusTooManyRequests,
		RejectMessage:        "busy",
	})

	limiter, _ := mw.snapshot()
	held, release := limiter.tryAcquire("/v1/models", "anonymous:192.0.2.10")
	if !held.acquired {
		t.Fatalf("setup acquire rejected: %+v", held)
	}
	defer release()

	engine := gin.New()
	engine.Use(mw.Handler())
	engine.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
}
