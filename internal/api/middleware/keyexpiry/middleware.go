package keyexpiry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	EnvStatusURL          = "SHOP_KEY_STATUS_URL"
	EnvToken              = "SHOP_KEY_STATUS_TOKEN"
	EnvActiveTTLSeconds   = "SHOP_KEY_STATUS_ACTIVE_TTL_SECONDS"
	EnvInactiveTTLSeconds = "SHOP_KEY_STATUS_INACTIVE_TTL_SECONDS"

	defaultActiveTTL   = 60 * time.Second
	defaultInactiveTTL = 10 * time.Second
)

var errStatusUnavailable = errors.New("api key status unavailable")

type Config struct {
	StatusURL   string
	Token       string
	ActiveTTL   time.Duration
	InactiveTTL time.Duration
	HTTPClient  *http.Client
}

type Middleware struct {
	statusURL   string
	token       string
	activeTTL   time.Duration
	inactiveTTL time.Duration
	client      *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	result    statusResult
	expiresAt time.Time
}

type statusResult struct {
	Managed   bool   `json:"managed"`
	Active    bool   `json:"active"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expiresAt"`
}

func NewFromEnv() *Middleware {
	return NewMiddleware(Config{
		StatusURL:   os.Getenv(EnvStatusURL),
		Token:       os.Getenv(EnvToken),
		ActiveTTL:   durationFromEnv(EnvActiveTTLSeconds, defaultActiveTTL),
		InactiveTTL: durationFromEnv(EnvInactiveTTLSeconds, defaultInactiveTTL),
	})
}

func NewMiddleware(cfg Config) *Middleware {
	activeTTL := cfg.ActiveTTL
	if activeTTL <= 0 {
		activeTTL = defaultActiveTTL
	}
	inactiveTTL := cfg.InactiveTTL
	if inactiveTTL <= 0 {
		inactiveTTL = defaultInactiveTTL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	return &Middleware{
		statusURL:   strings.TrimSpace(cfg.StatusURL),
		token:       strings.TrimSpace(cfg.Token),
		activeTTL:   activeTTL,
		inactiveTTL: inactiveTTL,
		client:      client,
		cache:       make(map[string]cacheEntry),
	}
}

func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m == nil || m.statusURL == "" {
			c.Next()
			return
		}

		apiKey := apiKeyFromContext(c)
		if apiKey == "" {
			c.Next()
			return
		}

		result, err := m.check(c.Request.Context(), apiKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, unavailableResponse())
			return
		}
		if result.Managed && !result.Active {
			c.AbortWithStatusJSON(http.StatusUnauthorized, inactiveResponse(result.Status))
			return
		}

		c.Next()
	}
}

func (m *Middleware) check(ctx context.Context, apiKey string) (statusResult, error) {
	if result, ok := m.cached(apiKey); ok {
		return result, nil
	}
	if m.token == "" {
		return statusResult{}, errStatusUnavailable
	}

	endpoint, err := statusEndpoint(m.statusURL, apiKey)
	if err != nil {
		return statusResult{}, errStatusUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return statusResult{}, errStatusUnavailable
	}
	request.Header.Set("x-internal-token", m.token)

	response, err := m.client.Do(request)
	if err != nil {
		return statusResult{}, errStatusUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return statusResult{}, errStatusUnavailable
	}

	var result statusResult
	if errDecode := json.NewDecoder(response.Body).Decode(&result); errDecode != nil {
		return statusResult{}, errStatusUnavailable
	}
	m.store(apiKey, result)
	return result, nil
}

func (m *Middleware) cached(apiKey string) (statusResult, bool) {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.cache[apiKey]
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(m.cache, apiKey)
		}
		return statusResult{}, false
	}
	return entry.result, true
}

func (m *Middleware) store(apiKey string, result statusResult) {
	ttl := m.inactiveTTL
	if !result.Managed || result.Active {
		ttl = m.activeTTL
	}

	m.mu.Lock()
	m.cache[apiKey] = cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(ttl),
	}
	m.mu.Unlock()
}

func apiKeyFromContext(c *gin.Context) string {
	value, ok := c.Get("apiKey")
	if !ok {
		return ""
	}
	apiKey, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(apiKey)
}

func statusEndpoint(rawURL string, apiKey string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("apiKey", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func inactiveResponse(status string) gin.H {
	if status == "" {
		status = "inactive"
	}
	return gin.H{
		"error": gin.H{
			"message": "API key is not active. Redeem a new key before making new requests.",
			"type":    "invalid_request_error",
			"code":    "api_key_inactive",
			"status":  status,
		},
	}
}

func unavailableResponse() gin.H {
	return gin.H{
		"error": gin.H{
			"message": "API key status validation is unavailable.",
			"type":    "service_unavailable",
			"code":    "api_key_status_unavailable",
		},
	}
}
