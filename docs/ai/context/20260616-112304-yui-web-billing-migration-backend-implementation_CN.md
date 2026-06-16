# yui.web 计费迁移后端收尾实施记录

## 目标

把当前运行中依赖的本地自定义能力补回 CLIProxyAPI 源码，使后续重新构建或升级时不会丢失 yui.web 计费链路。

最终边界：

- CLIProxyAPI 只做入口鉴权、yui.web 状态查询和 usage JSONL 生产。
- yui.web 读取 JSONL / 接收实时 usage，完成价格计算、余额扣减、流水和欠费状态。
- 新账户余额为 0 时允许本次调用；扣成负数后，下一次调用由 CLIProxyAPI 查询 yui.web 后拒绝。

## 代码改动

- 新增 `internal/usage/` usage event publisher：
  - 受 `USAGE_EVENTS_ENABLED=true` 控制。
  - 默认写入 `logs/usage/usage-events-YYYY-MM.jsonl`。
  - JSONL 只保存 API key hash 和 preview，不保存完整 API key。
  - 可选实时 POST 到 yui.web `/api/internal/usage-events`。
  - 实时 POST 使用独立 3 秒 context，不继承已取消的请求 context。
- 新增 `internal/api/middleware/keyexpiry/`：
  - 受 `SHOP_KEY_STATUS_URL` 和 `SHOP_KEY_STATUS_TOKEN` 控制。
  - 在认证后查询 yui.web `/api/internal/api-keys/status`。
  - `managed=true, active=false` 返回 `401 api_key_inactive`。
  - 托管 active key 不缓存，保证扣成负数后的下一次调用能立即被拒绝。
  - 未托管 key 和 inactive 结果可短期缓存，减少无意义状态查询。
- 在 `internal/api/server.go` 中把状态检查挂到：
  - `/v1/*`
  - `/backend-api/codex/*`
  - `/v1beta/*`
  - 启用鉴权的 WebSocket 路由。
- 在 `cmd/server/main.go` 启动时注册 usage event publisher。
- 在 `config.example.yaml` 记录 yui.web 计费迁移相关环境变量。

## yui.web 配套

- `yui.web/.env.example` 新增：
  - `SHOP_USAGE_AUTO_IMPORT_ENABLED=true`
  - `SHOP_USAGE_AUTO_IMPORT_INTERVAL_MS=60000`

真实 `.env` 已配置：

- `USAGE_EVENTS_ENABLED=true`
- `USAGE_EVENTS_LOG_DIR=logs/usage`
- `SHOP_KEY_STATUS_URL=http://127.0.0.1:4173/api/internal/api-keys/status`
- `YUI_USAGE_EVENT_URL=http://127.0.0.1:4173/api/internal/usage-events`
- 状态 token 和 HMAC secret 已设置，但不写入文档。

## 运行态验收

已重新构建并重启 CLIProxyAPI，当前监听 `127.0.0.1:8317`。

使用真实兑换 API key 脱敏预览 `sk-yui-F-E...G80atx` 验收：

1. 验收前：
   - yui.web 余额：`0` nanos
   - `usage_count=0`
   - `charge_count=0`
   - CLIProxyAPI JSONL 中该 hash 行数：`0`
2. 第一次真实调用：
   - 请求：`POST /v1/chat/completions`
   - 模型：`gpt-5.4-mini`
   - 返回：HTTP `200`
   - usage：`prompt_tokens=307`、`completion_tokens=5`、`total_tokens=312`
3. 事后入账：
   - CLIProxyAPI JSONL 中该 hash 行数：`1`
   - yui.web `usage_count=1`
   - yui.web `charge_count=1`
   - 扣费：`805000` nanos
   - 余额：`-805000` nanos
4. 下一次请求：
   - 请求：`GET /v1/models`
   - 返回：HTTP `401`
   - 错误码：`api_key_inactive`
   - yui.web 状态：`insufficient_balance`

这证明“本次允许扣成负数，负数后下一次不可调用”已按 yui.web 事后扣费规则生效。

## 验证命令

- `go test ./internal/usage ./internal/api/middleware/keyexpiry ./internal/api`
- `go test ./cmd/server`
- `go build -o test-output ./cmd/server && rm test-output`
- `go test ./...`
- yui.web: `npm test`

## 后续注意

- 不要把价格表、余额计算或扣费流水移回 CLIProxyAPI。
- 后续升级官方 main 时，必须保留 `internal/usage` 和 `keyexpiry` 两块本地能力。
- 如果要追求更低延迟，可继续使用实时 POST；但可靠主链路仍是 JSONL 自动导入。
