package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type usageEventWriter struct {
	dir           string
	retentionDays int
	mu            sync.Mutex
}

func newUsageEventWriter(dir string, retentionDays int) *usageEventWriter {
	return &usageEventWriter{
		dir:           strings.TrimSpace(dir),
		retentionDays: retentionDays,
	}
}

func (w *usageEventWriter) write(event UsageEvent) error {
	if w == nil || w.dir == "" {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return fmt.Errorf("create usage event log dir: %w", err)
	}
	if w.retentionDays > 0 {
		w.cleanupLocked(time.Now())
	}

	path := filepath.Join(w.dir, "usage-events-"+resolveUsageEventMonth(event)+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open usage event ledger: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal usage event: %w", err)
	}
	if _, errWrite := file.Write(append(payload, '\n')); errWrite != nil {
		return fmt.Errorf("write usage event ledger: %w", errWrite)
	}
	return nil
}

func resolveUsageEventMonth(event UsageEvent) string {
	requestedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(event.RequestedAt))
	if err != nil {
		requestedAt = time.Now()
	}
	return requestedAt.Format("2006-01")
}

func (w *usageEventWriter) cleanupLocked(now time.Time) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -w.retentionDays)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "usage-events-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(w.dir, name))
	}
}
