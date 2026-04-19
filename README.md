# clawbridge

多 IM 对接 Claw 系运行时的 Go 库：统一消息总线、媒体定位符与多 client 管理。设计说明见 [`docs/im-bridge-technical-design.md`](docs/im-bridge-technical-design.md)，对外 API 见 [`docs/public-api.md`](docs/public-api.md)。

## 文档与宿主 API 速览

| 能力 | 推荐入口（根包 / `Bridge`） | 说明 |
|------|------------------------------|------|
| 快捷回复 | `Reply(ctx, in, text, mediaPath) (*OutboundMessage, error)` | 若 Driver 实现 **`Replier`**，由 Driver 自定义回复（可写 `Metadata`）；否则默认组装的 `OutboundMessage` 经 **`PublishOutbound`** 发送；详见 public-api |
| 更新消息级状态 | `UpdateStatus(ctx, in, state, metadata)` | `state` 为 `UpdateStatusState`（如 `UpdateStatusProcessing`）；完整自定义用 `UpdateStatusRequest` |
| 编辑已发送内容 | `EditMessage(ctx, out *OutboundMessage)` | Driver 的 **`MessageEditor`** 接收完整 **`OutboundMessage`**（含 `Metadata` 等）；`out.MessageID` 空则见 public-api §2.2.1 |
| 任意出站 | `PublishOutbound` / `Bus().PublishOutbound` | 自行构造 `OutboundMessage` |

`OutboundMessage` 含可选字段 **`message_id`**：发送时各 Driver **忽略**；编辑时用于指定要改的平台消息。详见 public-api §2.1、§2.2。

内置 **noop / webchat / telegram / slack / feishu / weixin** 均已实现 **`client.Replier`**（`Reply` → `DefaultReplyOutbound` + `Send`），与先前经 `PublishOutbound` 的行为一致，但走同步路径并立即触发 **`OutboundSendNotify`**。

## 来源说明

本仓库的**部分思路、接口形态与实现细节**参考或改编自 **[PicoClaw](https://github.com/sipeed/picoclaw)**（`sipeed/picoclaw`）项目中的 channel / 总线 / 消息模型等设计；clawbridge 在出站模型、媒体定位符等方面做了独立裁剪与约定，并非 PicoClaw 的完整拷贝。若你维护或分发衍生作品，请同时遵守 PicoClaw 与本项目各自的许可证要求。

## 快速试用

```bash
go run ./examples/host -config examples/host/config.yaml -duration=3s
```

配置示例见根目录 [`config.example.yaml`](config.example.yaml)。
