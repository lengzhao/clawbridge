# clawbridge

多 IM 对接 Claw 系运行时的 Go 库：统一消息总线、媒体定位符与多 client 管理。设计说明见 [`docs/im-bridge-technical-design.md`](docs/im-bridge-technical-design.md)，对外 API 见 [`docs/public-api.md`](docs/public-api.md)。

## 来源说明

本仓库的**部分思路、接口形态与实现细节**参考或改编自 **[PicoClaw](https://github.com/sipeed/picoclaw)**（`sipeed/picoclaw`）项目中的 channel / 总线 / 消息模型等设计；clawbridge 在出站模型、媒体定位符等方面做了独立裁剪与约定，并非 PicoClaw 的完整拷贝。若你维护或分发衍生作品，请同时遵守 PicoClaw 与本项目各自的许可证要求。

## 快速试用

```bash
go run ./examples/host -config examples/host/config.yaml -duration=3s
```

配置示例见根目录 [`config.example.yaml`](config.example.yaml)。
