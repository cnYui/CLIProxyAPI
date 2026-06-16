package inboundlimit

import (
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

type endpointLimit struct {
	pathPrefix           string
	globalConcurrency    int
	perAPIKeyConcurrency int
}

type runtimeConfig struct {
	enabled              bool
	globalConcurrency    int
	perAPIKeyConcurrency int
	rejectStatusCode     int
	rejectMessage        string
	endpointOverrides    []endpointLimit
}

type limitSnapshot struct {
	pathPrefix           string
	globalConcurrency    int
	perAPIKeyConcurrency int
	rejectStatusCode     int
	rejectMessage        string
}

type principalCounter struct {
	active int
}

type scopeCounter struct {
	active      int
	byPrincipal map[string]*principalCounter
}

type acquireResult struct {
	acquired       bool
	reason         string
	pathPrefix     string
	globalActive   int
	keyActive      int
	scopeActive    int
	scopeKeyActive int
	globalLimit    int
	keyLimit       int
	scopeLimit     int
	scopeKeyLimit  int
}

// Limiter tracks active inbound requests for a single runtime config snapshot.
type Limiter struct {
	cfg runtimeConfig

	mu                 sync.Mutex
	activeGlobal       int
	byPrincipal        map[string]*principalCounter
	scopesByPathPrefix map[string]*scopeCounter
}

func newLimiter(cfg runtimeConfig) *Limiter {
	return &Limiter{
		cfg:                cfg,
		byPrincipal:        make(map[string]*principalCounter),
		scopesByPathPrefix: make(map[string]*scopeCounter),
	}
}

func normalizeConfig(cfg config.InboundLimitsConfig) runtimeConfig {
	cfg = config.SanitizeInboundLimitsConfig(cfg)
	if !cfg.Enabled {
		return runtimeConfig{}
	}

	overrides := make([]endpointLimit, 0, len(cfg.EndpointOverrides))
	for _, override := range cfg.EndpointOverrides {
		overrides = append(overrides, endpointLimit{
			pathPrefix:           override.PathPrefix,
			globalConcurrency:    override.GlobalConcurrency,
			perAPIKeyConcurrency: override.PerAPIKeyConcurrency,
		})
	}

	return runtimeConfig{
		enabled:              true,
		globalConcurrency:    cfg.GlobalConcurrency,
		perAPIKeyConcurrency: cfg.PerAPIKeyConcurrency,
		rejectStatusCode:     cfg.RejectStatusCode,
		rejectMessage:        cfg.RejectMessage,
		endpointOverrides:    overrides,
	}
}

func (l *Limiter) limitForPath(path string) limitSnapshot {
	snapshot := limitSnapshot{
		globalConcurrency:    l.cfg.globalConcurrency,
		perAPIKeyConcurrency: l.cfg.perAPIKeyConcurrency,
		rejectStatusCode:     l.cfg.rejectStatusCode,
		rejectMessage:        l.cfg.rejectMessage,
	}

	longest := 0
	for _, override := range l.cfg.endpointOverrides {
		if !strings.HasPrefix(path, override.pathPrefix) {
			continue
		}
		if len(override.pathPrefix) <= longest {
			continue
		}
		longest = len(override.pathPrefix)
		snapshot.pathPrefix = override.pathPrefix
		snapshot.globalConcurrency = override.globalConcurrency
		snapshot.perAPIKeyConcurrency = override.perAPIKeyConcurrency
	}

	return snapshot
}

func (l *Limiter) tryAcquire(path, principal string) (acquireResult, func()) {
	limit := l.limitForPath(path)

	l.mu.Lock()
	defer l.mu.Unlock()

	totalCounter := l.byPrincipal[principal]
	keyActive := 0
	if totalCounter != nil {
		keyActive = totalCounter.active
	}

	var scope *scopeCounter
	var scopeKeyCounter *principalCounter
	scopeActive := 0
	scopeKeyActive := 0
	if limit.pathPrefix != "" {
		scope = l.scopesByPathPrefix[limit.pathPrefix]
		if scope != nil {
			scopeActive = scope.active
			scopeKeyCounter = scope.byPrincipal[principal]
			if scopeKeyCounter != nil {
				scopeKeyActive = scopeKeyCounter.active
			}
		}
	}

	result := acquireResult{
		pathPrefix:     limit.pathPrefix,
		globalActive:   l.activeGlobal,
		keyActive:      keyActive,
		scopeActive:    scopeActive,
		scopeKeyActive: scopeKeyActive,
		globalLimit:    l.cfg.globalConcurrency,
		keyLimit:       l.cfg.perAPIKeyConcurrency,
		scopeLimit:     limit.globalConcurrency,
		scopeKeyLimit:  limit.perAPIKeyConcurrency,
	}

	if l.activeGlobal >= l.cfg.globalConcurrency {
		result.reason = "global_concurrency"
		return result, nil
	}
	if keyActive >= l.cfg.perAPIKeyConcurrency {
		result.reason = "per_api_key_concurrency"
		return result, nil
	}
	if limit.pathPrefix != "" {
		if scopeActive >= limit.globalConcurrency {
			result.reason = "endpoint_global_concurrency"
			return result, nil
		}
		if scopeKeyActive >= limit.perAPIKeyConcurrency {
			result.reason = "endpoint_per_api_key_concurrency"
			return result, nil
		}
	}

	if totalCounter == nil {
		totalCounter = &principalCounter{}
		l.byPrincipal[principal] = totalCounter
	}
	l.activeGlobal++
	totalCounter.active++

	if limit.pathPrefix != "" {
		if scope == nil {
			scope = &scopeCounter{byPrincipal: make(map[string]*principalCounter)}
			l.scopesByPathPrefix[limit.pathPrefix] = scope
		}
		if scopeKeyCounter == nil {
			scopeKeyCounter = &principalCounter{}
			scope.byPrincipal[principal] = scopeKeyCounter
		}
		scope.active++
		scopeKeyCounter.active++
		result.scopeActive = scope.active
		result.scopeKeyActive = scopeKeyCounter.active
	}

	result.acquired = true
	result.globalActive = l.activeGlobal
	result.keyActive = totalCounter.active

	return result, func() {
		l.release(limit.pathPrefix, principal)
	}
}

func (l *Limiter) release(pathPrefix, principal string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.activeGlobal > 0 {
		l.activeGlobal--
	}

	totalCounter := l.byPrincipal[principal]
	if totalCounter != nil {
		if totalCounter.active > 0 {
			totalCounter.active--
		}
		if totalCounter.active == 0 {
			delete(l.byPrincipal, principal)
		}
	}

	if pathPrefix == "" {
		return
	}
	scope := l.scopesByPathPrefix[pathPrefix]
	if scope == nil {
		return
	}
	if scope.active > 0 {
		scope.active--
	}
	scopeKeyCounter := scope.byPrincipal[principal]
	if scopeKeyCounter != nil {
		if scopeKeyCounter.active > 0 {
			scopeKeyCounter.active--
		}
		if scopeKeyCounter.active == 0 {
			delete(scope.byPrincipal, principal)
		}
	}
	if scope.active == 0 {
		delete(l.scopesByPathPrefix, pathPrefix)
	}
}

func (l *Limiter) activeForPrincipal(principal string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	counter := l.byPrincipal[principal]
	if counter == nil {
		return 0
	}
	return counter.active
}

func (l *Limiter) activePrincipals() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.byPrincipal)
}

func (l *Limiter) activeForScope(pathPrefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	scope := l.scopesByPathPrefix[pathPrefix]
	if scope == nil {
		return 0
	}
	return scope.active
}
