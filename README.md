# clawbridge

AgentKit **platform** 扩展库：微信 iLink 运行插件、Lark/飞书 setup 登录插件，以及统一的 `platform.Authenticator` / `credentials` 边界。

设计说明见 [`docs/architecture.md`](docs/architecture.md)。参考实现与契约来自同级仓库 [AgentDock](https://github.com/lengzhao/AgentDock) 的 `platform/` 包。

## 提供的 pluginkit kind

| Kind | 类型 |
|------|------|
| `platform/weixin` | `agentkit.Platform` |
| `platform/cloud-web` | cc-connect Cloud Web 协议 → `agentkit.Platform` |
| `platform/auth-lark` | `platform.Authenticator` |
| `platform/auth-cloud-web` | Cloud Web 网关 token |
| `platform/auth-feishu` | `platform.Authenticator` |
| `platform/auth-weixin` | `platform.Authenticator` |
| `clawbridge/credentials-keyring` | `credentials.Store` |
| `clawbridge/credentials-memory` | `credentials.Store` |

Lark/飞书/Slack **消息收发**请使用 agentkit 内置插件。Telegram、Discord、钉钉等见 `platform/catalog`：**cc-connect 跑 IM，本库 `platform/cloud-web` 做协议转换**。

```yaml
platform.telegram:
  use: platform/cloud-web
  config:
    bridgeName: telegram
    token: "..."
    transport: websocket
    baseUrl: "https://your-cloud-web-gateway"
```

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
