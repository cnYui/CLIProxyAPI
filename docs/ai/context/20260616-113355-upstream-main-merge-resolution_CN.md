# upstream main 合并冲突处理记录

## 背景

- 当前分支：`codex/yui-web-billing-handoff`
- 本地旧主线定制提交：`935cdd8c feat: hand off shop billing to yui web`
- 合并来源：`origin/main`，最新提交 `2406daf3`，对应上游 `v7.2.7`
- 冲突文件：
  - `internal/api/server.go`
  - `internal/api/server_test.go`

## 决策

1. 保留 yui.web 管理 API Key 的 post-auth 状态检查。
   - 这是 shop 兑换后无需写入 CLIProxyAPI 环境变量的关键链路。
   - yui.web 负责判断 key 是否 managed / active，CLIProxyAPI 只在认证后实时查询状态。

2. 删除旧分支中残留的 Amp 路由和测试。
   - 上游最新 `origin/main` 已移除 `internal/api/modules/amp`。
   - 冲突处理时不恢复 Amp import、Amp module 字段或 `/api/provider/*` 测试。

3. 新增的 `/openai/v1` 视频路由也需要接入 `postAuthMiddleware()`。
   - 它和 `/v1`、`/backend-api/codex`、`/v1beta` 一样属于客户端 API 入口。
   - 若不接入，负余额或 inactive 的 yui.web managed key 仍可能访问该入口。

4. WebSocket 动态路由继续保留 key 状态检查。
   - `AttachWebsocketRoute` 在条件认证后追加 `keyExpiry.Handler()`。
   - 这样启用 WebSocket 认证时仍受 yui.web active 状态约束。

## 验证计划

1. `gofmt -w internal/api/server.go internal/api/server_test.go`
2. `git diff --check`
3. `go test ./internal/usage ./internal/api/middleware/keyexpiry ./internal/api ./cmd/server`
4. `go test ./...`
5. `go build -o test-output ./cmd/server && rm test-output`
