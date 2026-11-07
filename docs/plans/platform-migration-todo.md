# cc-connect platform → clawbridge 迁移 TODO

目标：在 **不 import cc-connect**、**不走 Cloud Web** 前提下，将 `platform/catalog` 中 `planned` 平台逐个实现为 `agentkit.Platform`。

参考模板：`platform/weixin/`（Receive 循环、ingress、`common.Outbound`、auth、`pluginkit.Register`）。

## 0. 基线（一次性）

- [x] 编写「cc-connect `core` → agentkit」对照表 → [cc-connect-to-agentkit.md](./cc-connect-to-agentkit.md)
- [ ] 抽取可复用脚手架：`allowfrom`、dedup、slash/perm 卡片（对齐 `agentkit/runtime/platform/common`）
- [ ] 约定每个平台目录：`platform.go`、`config`+`deps`、`auth.go`（可选）、`*_test.go`、`doc.go` blank import
- [ ] 提交当前移除 Cloud Web、更新 catalog 的变更

## 1. 平台迁移（按 catalog `planned`）

每个平台统一子步骤：

1. 阅读 cc-connect `platform/<id>/` 与配置项（对齐 `platformMeta` 字段名到 JSON config）
2. 实现 `platform/<id>/platform.go` + `pluginkit.Register("platform/<id>", New)`
3. 实现 `platform/auth-<id>`（如需 token/扫码）
4. 单测（API mock / 录制的 wire JSON）
5. 更新 `platform/catalog`：`StrategyClawbridge` + `PluginKind`
6. 更新 `docs/architecture.md` / README

### 建议优先级

| 顺序 | ID | cc-connect 路径 | 备注 |
|------|-----|-----------------|------|
| P0 | `telegram` | `platform/telegram/` | **进行中**：文本 MVP 已落地，待媒体/callback/流式 |
| P1 | `dingtalk` | `platform/dingtalk/` | 流式 + 卡片，依赖 official stream SDK |
| P1 | `discord` | `platform/discord/` | discordgo，网关 intents |
| P2 | `wecom` | `platform/wecom/` | 回调 + WebSocket，配置项多 |
| P2 | `line` | `platform/line/` | Webhook 伺服 |
| P3 | `qq` | `platform/qq/` | OneBot WS |
| P3 | `qqbot` | `platform/qqbot/` | 官方开放平台 |
| P3 | `yuanbao` | `platform/yuanbao/` | 腾讯元宝 HTTP |
| P4 | `weibo` | `platform/weibo/` | |
| P4 | `matrix` | `platform/matrix/` | E2EE 复杂 |
| P4 | `webex` | `platform/webex/` | |
| P4 | `max` | `platform/max/` | |

### 不迁移到 clawbridge（保持现状）

| ID | 原因 |
|----|------|
| `feishu`, `lark`, `slack` | agentkit 内置，clawbridge 仅 lark setup/auth |
| `weixin` | 已完成 |

## 2. 宿主集成（AgentDock / harness）

- [ ] AgentDock `runtime` 图增加对新 `platform/*` kind 的装配示例
- [ ] setup CLI：按 `catalog.ByID` 选择 auth kind（避免 switch 膨胀）
- [ ] doctor：按平台做连通性探测（token/回调 URL）

## 3. 质量门禁

- [ ] 每平台：`go test ./platform/<id>/...`
- [ ] 全仓：`go test ./...`
- [ ] 真机 E2E 记录在平台包 README（可选）
