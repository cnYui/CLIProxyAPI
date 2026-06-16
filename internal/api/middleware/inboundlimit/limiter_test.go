package inboundlimit

import (
	"sync"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestLimiter_PerPrincipalLimitAndCleanup(t *testing.T) {
	limiter := newLimiter(normalizeConfig(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    10,
		PerAPIKeyConcurrency: 2,
		RejectStatusCode:     429,
		RejectMessage:        "busy",
	}))

	first, releaseFirst := limiter.tryAcquire("/v1/chat/completions", "key-1")
	if !first.acquired {
		t.Fatalf("first acquire rejected: %+v", first)
	}
	second, releaseSecond := limiter.tryAcquire("/v1/chat/completions", "key-1")
	if !second.acquired {
		t.Fatalf("second acquire rejected: %+v", second)
	}
	third, releaseThird := limiter.tryAcquire("/v1/chat/completions", "key-1")
	if third.acquired {
		releaseThird()
		t.Fatal("third acquire should be rejected")
	}
	if third.reason != "per_api_key_concurrency" {
		t.Fatalf("reject reason = %q, want per_api_key_concurrency", third.reason)
	}

	releaseFirst()
	releaseSecond()

	if active := limiter.activeForPrincipal("key-1"); active != 0 {
		t.Fatalf("active for key = %d, want 0", active)
	}
	if principals := limiter.activePrincipals(); principals != 0 {
		t.Fatalf("active principals = %d, want 0", principals)
	}
}

func TestLimiter_GlobalLimit(t *testing.T) {
	limiter := newLimiter(normalizeConfig(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    2,
		PerAPIKeyConcurrency: 5,
		RejectStatusCode:     429,
		RejectMessage:        "busy",
	}))

	first, releaseFirst := limiter.tryAcquire("/v1/chat/completions", "key-1")
	if !first.acquired {
		t.Fatalf("first acquire rejected: %+v", first)
	}
	defer releaseFirst()

	second, releaseSecond := limiter.tryAcquire("/v1/chat/completions", "key-2")
	if !second.acquired {
		t.Fatalf("second acquire rejected: %+v", second)
	}
	defer releaseSecond()

	third, releaseThird := limiter.tryAcquire("/v1/chat/completions", "key-3")
	if third.acquired {
		releaseThird()
		t.Fatal("third acquire should be globally rejected")
	}
	if third.reason != "global_concurrency" {
		t.Fatalf("reject reason = %q, want global_concurrency", third.reason)
	}
}

func TestLimiter_LongestPathPrefixOverride(t *testing.T) {
	limiter := newLimiter(normalizeConfig(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    20,
		PerAPIKeyConcurrency: 5,
		RejectStatusCode:     429,
		RejectMessage:        "busy",
		EndpointOverrides: []config.InboundLimitOverride{
			{PathPrefix: "/v1", GlobalConcurrency: 10, PerAPIKeyConcurrency: 4},
			{PathPrefix: "/v1/images/generations", GlobalConcurrency: 2, PerAPIKeyConcurrency: 1},
		},
	}))

	limit := limiter.limitForPath("/v1/images/generations")
	if limit.pathPrefix != "/v1/images/generations" {
		t.Fatalf("path prefix = %q, want /v1/images/generations", limit.pathPrefix)
	}
	if limit.globalConcurrency != 2 {
		t.Fatalf("global concurrency = %d, want 2", limit.globalConcurrency)
	}
	if limit.perAPIKeyConcurrency != 1 {
		t.Fatalf("per-key concurrency = %d, want 1", limit.perAPIKeyConcurrency)
	}
}

func TestLimiter_EndpointOverrideUsesScopedCounters(t *testing.T) {
	limiter := newLimiter(normalizeConfig(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    100,
		PerAPIKeyConcurrency: 100,
		RejectStatusCode:     429,
		RejectMessage:        "busy",
		EndpointOverrides: []config.InboundLimitOverride{
			{PathPrefix: "/v1/images/generations", GlobalConcurrency: 2, PerAPIKeyConcurrency: 1},
		},
	}))

	releases := make([]func(), 0, 22)
	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	for i := 0; i < 20; i++ {
		result, release := limiter.tryAcquire("/v1/chat/completions", "chat-key")
		if !result.acquired {
			t.Fatalf("chat acquire %d rejected: %+v", i, result)
		}
		releases = append(releases, release)
	}

	firstImage, releaseFirstImage := limiter.tryAcquire("/v1/images/generations", "image-key-1")
	if !firstImage.acquired {
		t.Fatalf("first image acquire rejected: %+v", firstImage)
	}
	releases = append(releases, releaseFirstImage)

	secondImage, releaseSecondImage := limiter.tryAcquire("/v1/images/generations", "image-key-2")
	if !secondImage.acquired {
		t.Fatalf("second image acquire rejected: %+v", secondImage)
	}
	releases = append(releases, releaseSecondImage)

	thirdImage, releaseThirdImage := limiter.tryAcquire("/v1/images/generations", "image-key-3")
	if thirdImage.acquired {
		releaseThirdImage()
		t.Fatal("third image acquire should be rejected by endpoint scope")
	}
	if thirdImage.reason != "endpoint_global_concurrency" {
		t.Fatalf("reject reason = %q, want endpoint_global_concurrency", thirdImage.reason)
	}
	if active := limiter.activeForScope("/v1/images/generations"); active != 2 {
		t.Fatalf("active image scope = %d, want 2", active)
	}
}

func TestLimiter_ConcurrentAcquireRelease(t *testing.T) {
	limiter := newLimiter(normalizeConfig(config.InboundLimitsConfig{
		Enabled:              true,
		GlobalConcurrency:    100,
		PerAPIKeyConcurrency: 100,
		RejectStatusCode:     429,
		RejectMessage:        "busy",
	}))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, release := limiter.tryAcquire("/v1/chat/completions", "key-1")
			if !result.acquired {
				t.Errorf("unexpected rejection: %+v", result)
				return
			}
			release()
		}()
	}
	wg.Wait()

	if active := limiter.activeForPrincipal("key-1"); active != 0 {
		t.Fatalf("active for key = %d, want 0", active)
	}
}
