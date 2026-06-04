package keyexpiry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMiddlewareAllowsRequestsWhenStatusURLIsEmpty(t *testing.T) {
	middleware := NewMiddleware(Config{
		StatusURL: "",
		Token:     "token",
	})

	statusCode, body := runRequest(t, middleware.Handler(), "sk-local")

	if statusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", statusCode, http.StatusOK, body)
	}
}

func TestMiddlewareAllowsUnmanagedAndActiveShopKeys(t *testing.T) {
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-internal-token"); got != "shared-token" {
			t.Fatalf("x-internal-token = %q, want %q", got, "shared-token")
		}

		response := map[string]any{
			"managed":   false,
			"active":    false,
			"status":    "not_found",
			"expiresAt": "",
		}
		if r.URL.Query().Get("apiKey") == "sk-active" {
			response = map[string]any{
				"managed":   true,
				"active":    true,
				"status":    "active",
				"expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
			}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer statusServer.Close()

	middleware := NewMiddleware(Config{
		StatusURL: statusServer.URL,
		Token:     "shared-token",
	})

	for _, apiKey := range []string{"sk-local", "sk-active"} {
		statusCode, body := runRequest(t, middleware.Handler(), apiKey)
		if statusCode != http.StatusOK {
			t.Fatalf("api key %s: status code = %d, want %d; body=%s", apiKey, statusCode, http.StatusOK, body)
		}
	}
}

func TestMiddlewareRejectsManagedInactiveShopKeys(t *testing.T) {
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"managed":   true,
			"active":    false,
			"status":    "expired",
			"expiresAt": "2000-01-01T00:00:00.000Z",
		})
	}))
	defer statusServer.Close()

	middleware := NewMiddleware(Config{
		StatusURL: statusServer.URL,
		Token:     "shared-token",
	})

	statusCode, body := runRequest(t, middleware.Handler(), "sk-expired")

	if statusCode != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d; body=%s", statusCode, http.StatusUnauthorized, body)
	}
	if !strings.Contains(body, "api_key_inactive") {
		t.Fatalf("body missing inactive code: %s", body)
	}
}

func TestMiddlewareFailsClosedWhenStatusServiceIsUnavailable(t *testing.T) {
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer statusServer.Close()

	middleware := NewMiddleware(Config{
		StatusURL: statusServer.URL,
		Token:     "shared-token",
	})

	statusCode, body := runRequest(t, middleware.Handler(), "sk-active")

	if statusCode != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d; body=%s", statusCode, http.StatusServiceUnavailable, body)
	}
	if !strings.Contains(body, "api_key_status_unavailable") {
		t.Fatalf("body missing unavailable code: %s", body)
	}
}

func runRequest(t *testing.T, middleware gin.HandlerFunc, apiKey string) (int, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if apiKey != "" {
			c.Set("apiKey", apiKey)
		}
		c.Next()
	})
	router.Use(middleware)
	router.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder.Code, recorder.Body.String()
}
