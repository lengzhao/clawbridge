# cc-connect → AgentKit 迁移对照

迁移时 **不 import** `github.com/chenhg5/cc-connect`；仅参考其 `platform/<id>/` 实现，在 clawbridge 中注册 `agentkit.Platform`。

## 生命周期

| cc-connect (`core.Platform`) | AgentKit (`agentkit.Platform`) |
|------------------------------|--------------------------------|
| `Start(MessageHandler)` 注册回调 | `Receive` 阻塞；后台 goroutine 收事件 → `common.Inbox.Push` |
| `Stop()` | `Receive` 的 ctx 取消 + `runCancel` |
| `Reply(replyCtx, text)` | `Send(OutboundEvent)` → `common.Outbound` → 平台 `sendText` |
| `Name()` | `PlatformID()` + `SetPlatformInstance` |

## 消息与路由

| cc-connect | AgentKit |
|------------|----------|
| `core.Message.SessionKey` | `MessageEvent` 的 `Envelope.Route` / `DeliveryID`（不透明 session 字符串） |
| `core.Message.UserID` / `UserName` | `InboundFromContent` + `Metadata`（`sender_id`, `sender_name`） |
| `replyContext`（平台私有） | `sync.Map`：`SessionID` → `delivery{chatID, threadID, …}` |
| `core.AllowList` | `common.AllowList` |
| `core.MessageDedup` | `common.MessageDedup` |
| `core.IsOldMessage` | `message.Date` vs `common.ProcessStartTime` |

## 装配

| cc-connect | AgentKit / clawbridge |
|------------|----------------------|
| `core.RegisterPlatform` + `map[string]any` options | `pluginkit.Register("platform/<id>", New)` + `Config`/`Deps` struct |
| 无统一登录 | `platform.Authenticator` + `platform.SaveLogin`（token/扫码等） |
| `core.MessageHandler` 同步调引擎 | runner 调 `Receive` → loop |

## 能力接口（逐步迁移）

cc-connect 可选接口（`ImageSender`、`MessageUpdater`、…）在 AgentKit 侧通常：

- 文本/媒体入站 → `common.InboundFromContent` + `Workspace`
- 流式/编辑 → 在 `Send` 里处理 `EventMessageUpdate`（参考 agentkit `platform/slack`、`platform/feishu`）
- 权限卡 → `permission.Capable` + `EventPermissionRequest`

Telegram 首版仅：**文本入站 + 文本出站 + 群 @/命令路由**；媒体与 callback 在后续 PR 补齐。
