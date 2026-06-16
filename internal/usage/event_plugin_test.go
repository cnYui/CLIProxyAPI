package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

const testAPIKey = "sk-yui-test-secret"

func TestUsageEventPluginWritesJSONLWithoutFullAPIKey(t *testing.T) {
	dir := t.TempDir()
	plugin := newUsageEventPlugin(newUsageEventWriter(dir, 0), nil)

	plugin.HandleUsage(context.Background(), coreusage.Record{
		APIKey:      testAPIKey,
		Provider:    "codex",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:     10,
			OutputTokens:    2,
			ReasoningTokens: 1,
			CachedTokens:    4,
		},
	})

	content, err := os.ReadFile(filepath.Join(dir, "usage-events-2026-06.jsonl"))
	if err != nil {
		t.Fatalf("read usage event ledger: %v", err)
	}
	if strings.Contains(string(content), testAPIKey) {
		t.Fatal("usage event ledger contains full API key")
	}

	var event UsageEvent
	if errUnmarshal := json.Unmarshal(bytesTrimNewline(content), &event); errUnmarshal != nil {
		t.Fatalf("unmarshal usage event: %v content=%s", errUnmarshal, string(content))
	}
	if event.APIKeyHash == "" || event.APIKeyPreview == "" {
		t.Fatalf("usage event missing key hash or preview: %#v", event)
	}
	if event.InputTokens != 10 || event.OutputTokens != 2 || event.TotalTokens != 13 {
		t.Fatalf("usage tokens = input %d output %d total %d, want 10/2/13", event.InputTokens, event.OutputTokens, event.TotalTokens)
	}
}

func TestUsageEventSyncClientUsesIndependentContextAfterRequestCancellation(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-internal-token"); got != "internal-token" {
			t.Fatalf("x-internal-token = %q, want internal-token", got)
		}
		if r.Header.Get("x-usage-signature") == "" {
			t.Fatal("missing x-usage-signature")
		}
		requests <- struct{}{}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := newUsageEventSyncClient(server.URL, "internal-token", "hmac-secret")
	if err := client.sync(ctx, UsageEvent{Version: 1, RequestID: "req-canceled-context", RequestedAt: "2026-06-09T12:00:00Z"}); err != nil {
		t.Fatalf("sync usage event after request cancellation: %v", err)
	}

	select {
	case <-requests:
	case <-time.After(time.Second):
		t.Fatal("expected sync request despite canceled caller context")
	}
}

func TestUsageEventPluginFromEnvEnabledWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USAGE_EVENTS_ENABLED", "true")
	t.Setenv("USAGE_EVENTS_LOG_DIR", dir)
	t.Setenv("USAGE_EVENTS_RETENTION_DAYS", "0")
	t.Setenv("YUI_USAGE_EVENT_URL", "")
	t.Setenv("YUI_USAGE_EVENT_TOKEN", "")
	t.Setenv("YUI_USAGE_EVENT_HMAC_SECRET", "")

	plugin, enabled := newUsageEventPluginFromEnv()
	if !enabled || plugin == nil {
		t.Fatal("expected usage event plugin to be enabled")
	}
	plugin.HandleUsage(context.Background(), coreusage.Record{
		APIKey:      testAPIKey,
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	})

	if _, err := os.Stat(filepath.Join(dir, "usage-events-2026-06.jsonl")); err != nil {
		t.Fatalf("expected env-enabled plugin to write JSONL: %v", err)
	}
}

func bytesTrimNewline(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}
