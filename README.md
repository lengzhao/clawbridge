# clawbridge

AgentKit **platform** 扩展库：微信 iLink 运行插件、Lark/飞书 setup 登录插件，以及统一的 `platform.Authenticator` / `credentials` 边界。

设计说明见 [`docs/architecture.md`](docs/architecture.md)。参考实现与契约来自同级仓库 [AgentDock](https://github.com/lengzhao/AgentDock) 的 `platform/` 包。

## 提供的 pluginkit kind

| Kind | 类型 |
|------|------|
| `platform/weixin` | `agentkit.Platform` |
| `platform/telegram` | `agentkit.Platform`（文本 + 图片/文件/语音入站，图片/文件出站） |
| `platform/discord` | `agentkit.Platform`（文本 MVP：网关、群 @、分片出站） |
| `platform/auth-discord` | `platform.Authenticator`（bot token） |
| `platform/auth-telegram` | `platform.Authenticator`（bot token） |
| `platform/auth-lark` | `platform.Authenticator` |
| `platform/auth-feishu` | `platform.Authenticator` |
| `platform/auth-weixin` | `platform.Authenticator` |
| `clawbridge/credentials-keyring` | `credentials.Store` |
| `clawbridge/credentials-memory` | `credentials.Store` |

Lark/飞书/Slack **消息收发**使用 agentkit 内置 `platform/*`（`_ "github.com/lengzhao/agentkit/plugins"`）。  
更多 IM 见 `platform/catalog`：已实现条目走 agentkit 或 clawbridge `platform/*`；**planned** 平台需在 `platform/<name>/` 按 `agentkit.Platform` 原生实现（不走中间网关协议）。

## 使用

```go
import (
    _ "github.com/lengzhao/agentkit/plugins"
    _ "github.com/lengzhao/clawbridge"
    _ "github.com/lengzhao/clawbridge/plugin"
)
```

## 测试

```bash
go test ./...
```
