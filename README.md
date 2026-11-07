# clawbridge

AgentKit **platform** 扩展库：微信 iLink 运行插件、Lark/飞书 setup 登录插件，以及统一的 `platform.Authenticator` / `credentials` 边界。

设计说明见 [`docs/architecture.md`](docs/architecture.md)。参考实现与契约来自同级仓库 [AgentDock](https://github.com/lengzhao/AgentDock) 的 `platform/` 包。

## 提供的 pluginkit kind

| Kind | 类型 |
|------|------|
| `platform/weixin` | `agentkit.Platform` |
| `platform/telegram` | `agentkit.Platform`（文本 MVP） |
| `platform/auth-telegram` | `platform.Authenticator`（bot token） |
| `platform/auth-lark` | `platform.Authenticator` |
| `platform/auth-feishu` | `platform.Authenticator` |
| `platform/auth-weixin` | `platform.Authenticator` |
| `clawbridge/credentials-keyring` | `credentials.Store` |
| `clawbridge/credentials-memory` | `credentials.Store` |

Lark/飞书/Slack **消息收发**使用 agentkit 内置 `platform/*`（`_ "github.com/lengzhao/agentkit/plugins"`）。  
更多 IM（Telegram、Discord 等）见 `platform/catalog`：当前为 **planned**，需在 `platform/<name>/` 按 `agentkit.Platform` **原生实现**，本仓库不提供 Cloud Web / 中间网关协议。

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
