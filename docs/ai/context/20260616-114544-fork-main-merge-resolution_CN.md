# fork/main 合并冲突处理记录

## 背景

- 当前分支：`codex/yui-web-billing-handoff`
- 已合入官方上游：`origin/main` / `v7.2.7`
- 本次继续合入用户 fork 主线：`fork/main`，最新提交 `3535e936`
- `fork/main` 额外包含本地定制：
  - inbound request limiting
  - Antigravity image generation routing

## 决策

1. 保留 `fork/main` 的 inbound limit 能力。
   - `internal/api/middleware/inboundlimit/` 和 `internal/config/inbound_limits.go` 保留。
   - `Server.postAuthMiddleware()` 同时执行 yui.web key 状态检查和 inbound limiter。
   - `LoadConfigOptional()` 同时调用 `NormalizePluginsConfig()` 与 `SanitizeInboundLimits()`。

2. 保留 `fork/main` 的 Antigravity image generation routing。
   - `executeClaudeNonStream()` 先将 image generation 请求包装成 chat completions。
   - 同时保留官方最新的 KV 短冷却函数 `antigravityIsInShortCooldownRequired()`。
   - 流式转非流式后保留官方 grounding URL 修复，并在 image generation 场景转回 images 响应。

3. 继续按官方最新删除 Amp。
   - `origin/main` 已删除 `internal/api/modules/amp`。
   - `fork/main` 中残留的 Amp 文件在冲突处理中不恢复。

4. 不修改 `AGENTS.md`。
   - GitHub workflow `agents-md-guard` 会关闭任何修改 `AGENTS.md` 的 PR。
   - 本次合并上下文只写入 `docs/ai/context/`。

## 验证

已通过：

```bash
go test ./internal/api ./internal/api/middleware/inboundlimit ./internal/api/middleware/keyexpiry ./internal/config ./internal/runtime/executor ./internal/usage ./cmd/server
go test ./...
go build -o test-output ./cmd/server && rm test-output
git diff --check
```
