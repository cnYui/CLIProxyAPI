# AGENTS.md

Go 1.26+ proxy server providing OpenAI/Gemini/Claude/Codex compatible APIs with OAuth and round-robin load balancing.

## Repository
- GitHub: https://github.com/router-for-me/CLIProxyAPI

## Commands
```bash
gofmt -w . # Format (required after Go changes)
go build -o cli-proxy-api ./cmd/server # Build
go run ./cmd/server # Run dev server
go test ./... # Run all tests
go test -v -run TestName ./path/to/pkg # Run single test
go build -o test-output ./cmd/server && rm test-output # Verify compile (REQUIRED after changes)
```
- Common flags: `--config <path>`, `--tui`, `--standalone`, `--local-model`, `--no-browser`, `--oauth-callback-port <port>`

## Config
- Default config: `config.yaml` (template: `config.example.yaml`)
- `.env` is auto-loaded from the working directory
- Auth material defaults under `auths/`
- Storage backends: file-based default; optional Postgres/git/object store (`PGSTORE_*`, `GITSTORE_*`, `OBJECTSTORE_*`)
- 2026-06-11: 当前代码不支持 `inbound-limits` / `per-api-key-concurrency` 运行时配置；遇到本地残留块时应清理，不要当作有效入口限流。
- 2026-06-12: Added a local client API key with prefix `sk-yui-73iCV...` to `config.yaml` for testing against `127.0.0.1:8317`; context: `docs/ai/context/20260612-114015-local-client-api-key-config.md`.
- 2026-06-12: Verified the `sk-yui-73iCV...` key locally: `/v1/models` and `/v1/chat/completions` succeeded; result: `docs/ai/context/20260612-114211-local-client-api-key-test-result.md`.
- 2026-06-12: Added Shop client API key preview `sk-yui-i7XU8...956mss` for phone `138****6694` to `config.yaml`; backup: `backups/config-before-add-13813756694-key-20260612-173803.yaml`; `/v1/models` returned HTTP 200 with 5 models.
- 2026-06-13: Added Shop client API key preview `sk-yui-3l5x_...fWnc6g` for phone `193****7925` to `config.yaml`; backup: `backups/config-before-add-19301367925-key-20260613-165117.yaml`; `/v1/models` returned HTTP 200 with 5 models.
- 2026-06-13: Verified Shop client API key preview `sk-yui-OKeCq...hc9zuG` for phone `152****8391` already existed in `config.yaml`; backup: `backups/config-before-add-15279148391-key-20260613-203919.yaml`; `/v1/models` returned HTTP 200 with 5 models.
- 2026-06-13: Added 20 new Shop pool client keys to `config.yaml`; backup: `backups/config-before-add-20-shop-api-keys-20260613-174633.yaml`. Sample preview `sk-yui-lxLd7...OQjSJH` returned `401 api_key_inactive`, which is expected before Shop redemption; yui.web context: `docs/ai/context/20260613-174633-add-20-shop-api-keys-implementation_CN.md`.
- 2026-06-16: yui.web 计费迁移后端边界已落到源码：CLIProxyAPI 只生成 usage JSONL、可选实时 POST usage、并在认证后查询 yui.web key 状态；价格、扣费、余额和欠费状态都由 yui.web 负责。环境变量见 `config.example.yaml` 注释块；实施记录：`docs/ai/context/20260616-112304-yui-web-billing-migration-backend-implementation_CN.md`。

## Architecture
- `cmd/server/` — Server entrypoint
- `internal/api/` — Gin HTTP API (routes, middleware, modules)
- `internal/api/middleware/keyexpiry/` — yui.web 托管 API key 状态检查；`managed=true, active=false` 返回 `401 api_key_inactive`。托管 active key 不缓存，确保事后扣成负数后下一次调用立即被拒绝。
- `internal/api/modules/amp/` — Amp integration (Amp-style routes + reverse proxy)
- `internal/thinking/` — Main thinking/reasoning pipeline. `ApplyThinking()` (apply.go) parses suffixes (`suffix.go`, suffix overrides body), normalizes config to canonical `ThinkingConfig` (`types.go`), normalizes and validates centrally (`validate.go`/`convert.go`), then applies provider-specific output via `ProviderApplier`. Do not break this "canonical representation → per-provider translation" architecture.
- `internal/runtime/executor/` — Per-provider runtime executors (incl. Codex WebSocket)
- `internal/translator/` — Provider protocol translators (and shared `common`)
- `internal/registry/` — Model registry + remote updater (`StartModelsUpdater`); `--local-model` disables remote updates
- `internal/store/` — Storage implementations and secret resolution
- `internal/managementasset/` — Config snapshots and management assets
- `internal/cache/` — Request signature caching
- `internal/watcher/` — Config hot-reload and watchers
- `internal/wsrelay/` — WebSocket relay sessions
- `internal/usage/` — Usage and token accounting; 本地 Shop 计费迁移依赖这里的 usage event publisher 写入 `logs/usage/usage-events-YYYY-MM.jsonl`，不要在升级官方 main 时丢失。
- `internal/tui/` — Bubbletea terminal UI (`--tui`, `--standalone`)
- `sdk/cliproxy/` — Embeddable SDK entry (service/builder/watchers/pipeline)
- `test/` — Cross-module integration tests

## Code Conventions
- Keep changes small and simple (KISS)
- Comments in English only
- If editing code that already contains non-English comments, translate them to English (don’t add new non-English comments)
- For user-visible strings, keep the existing language used in that file/area
- New Markdown docs should be in English unless the file is explicitly language-specific (e.g. `README_CN.md`)
- As a rule, do not make standalone changes to `internal/translator/`. You may modify it only as part of broader changes elsewhere.
- If a task requires changing only `internal/translator/`, run `gh repo view --json viewerPermission -q .viewerPermission` to confirm you have `WRITE`, `MAINTAIN`, or `ADMIN`. If you do, you may proceed; otherwise, file a GitHub issue including the goal, rationale, and the intended implementation code, then stop further work.
- `internal/runtime/executor/` should contain executors and their unit tests only. Place any helper/supporting files under `internal/runtime/executor/helps/`.
- Follow `gofmt`; keep imports goimports-style; wrap errors with context where helpful
- Do not use `log.Fatal`/`log.Fatalf` (terminates the process); prefer returning errors and logging via logrus
- Shadowed variables: use method suffix (`errStart := server.Start()`)
- Wrap defer errors: `defer func() { if err := f.Close(); err != nil { log.Errorf(...) } }()`
- Use logrus structured logging; avoid leaking secrets/tokens in logs
- Avoid panics in HTTP handlers; prefer logged errors and meaningful HTTP status codes
- Timeouts are allowed only during credential acquisition; after an upstream connection is established, do not set timeouts for any subsequent network behavior. Intentional exceptions that must remain allowed are the Codex websocket liveness deadlines in `internal/runtime/executor/codex_websockets_executor.go`, the wsrelay session deadlines in `internal/wsrelay/session.go`, the management APICall timeout in `internal/api/handlers/management/api_tools.go`, and the `cmd/fetch_antigravity_models` utility timeouts
