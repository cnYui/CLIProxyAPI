# yui.web 计费迁移后端收尾计划

## 背景

用户已确认新的计费边界：

1. CLIProxyAPI 只产生日志。
2. yui.web 负责解析 CLIProxyAPI JSONL。
3. 计费为事后扣费：本次调用允许把余额扣成负数；余额为负后，下一次 API 调用不可用。

当前 yui.web 已具备 JSONL 导入、幂等写入、价格计算、余额扣减、负余额状态判断和 0 余额放行。真实库也已验证最新 CLIProxyAPI JSONL 能进入 yui.web 并生成 `api_charge_records`。

## 当前缺口

CLIProxyAPI 当前运行的本地二进制包含自定义能力：

- 生成 `logs/usage/usage-events-YYYY-MM.jsonl`。
- 可选 POST usage event 到 yui.web。
- 调用 yui.web `/api/internal/api-keys/status` 判断 Shop 托管 key 是否 active。

但当前 `main` 源码没有这些能力。若后续基于当前源码或官方最新版重新构建，yui.web 后端计费链路会失去 usage JSONL 输入，负余额后的下一次调用拦截也会失效。

## 本次收尾范围

本次只做后端必要闭环，不改 Shop 前端：

- 把 `internal/usage` usage event publisher 补回当前 CLIProxyAPI 源码。
- 把 `keyexpiry` 中间件补回当前 CLIProxyAPI 源码，使 CLIProxyAPI 只做 yui.web 状态查询，不做价格或余额计算。
- 修复 usage event POST 不能继承已取消请求 context 的问题；JSONL 写入仍是可靠主链路，POST 只是实时加速。
- 补 `.env.example` / `config.example.yaml` 运行配置说明。
- 保持 yui.web 为唯一计费事实来源：价格、扣费、余额、欠费状态都不进入 CLIProxyAPI。

## 验收标准

- CLIProxyAPI 单测覆盖：
  - usage JSONL 写入。
  - 已取消原请求 context 时，实时 POST 仍能送达。
  - yui.web 状态返回 `managed=true, active=false` 时，API 路由返回 `401 api_key_inactive`。
- `go test` 覆盖新增包和相关 API server 测试。
- `go build -o test-output ./cmd/server && rm test-output` 成功。
- yui.web 继续通过 `npm test`。
- 真实服务端到端验收：
  - 使用 0 余额 active key 可请求 `/v1/models`。
  - 写入一条 JSONL 后 yui.web 导入并扣费。
  - 扣成负数后下一次状态检查返回 inactive，CLIProxyAPI 拒绝。

## 非目标

- 不把 yui.web 的价格表、扣费规则或余额计算移回 CLIProxyAPI。
- 不重算历史账单。
- 不暴露完整 API key、内部 token 或 HMAC secret。
