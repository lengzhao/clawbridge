# Clawbridge 对外接口定义

本文描述宿主（Agent / Gateway / 业务进程）集成 **clawbridge** 时的 **公开 API 约定**：初始化、消息结构、总线与媒体、典型调用方式。实现时可微调命名，但 **语义应与本文件一致**，并与 [im-bridge-technical-design.md](./im-bridge-technical-design.md) 对齐。

**包布局（实现侧）**：模块根下 `bus`、`client`、`media` 等。

**两种对外用法（二选一或并存）**：

| 方式 | 适合场景 |
|------|----------|
| **显式 `New` + `*Bridge`** | 单进程多 Bridge、单元测试注入、需要把 `Bridge` 当字段传递 |
| **包级函数（§1.3）** | 单进程 **只有一个** Bridge、希望 **少传参、单 import**；底层仍是对 `*Bridge` 的薄封装 |

包级 API **不替代** `New`：实现上推荐 **包级函数委托给内部默认 `*Bridge`**，行为与显式实例一致。

---

## 1. 初始化

### 1.1 配置结构（示意）

```go
// Config 可从 YAML/JSON 解码；字段名实现时可与配置文件对齐。
type Config struct {
    Media   MediaConfig   `json:"media" yaml:"media"`
    Clients []ClientConfig `json:"clients" yaml:"clients"`
}

// MediaConfig 仅描述**内置默认**行为：本地磁盘临时文件。
// S3、对象存储、企业网盘等**不在**配置里内置，请实现 media.Backend 并通过 WithMediaBackend 注入（见 §1.2）。
type MediaConfig struct {
    Root string `json:"root,omitempty" yaml:"root,omitempty"` // 本地根目录；空则使用实现默认（如 os.TempDir 子目录）
}

type ClientConfig struct {
    ID      string         `json:"id" yaml:"id"`
    Driver  string         `json:"driver" yaml:"driver"`
    Enabled bool           `json:"enabled" yaml:"enabled"`
    Options map[string]any `json:"options,omitempty" yaml:"options,omitempty"` // 各 driver 私有：令牌、http 监听、触发规则等
}
```

需要 **HTTP 监听** 的 driver（例如 `webchat`）在 **`options` 内**约定字段（如 `listen`、`path`）。**webchat** 的会话列表与聊天记录由 **浏览器 localStorage** 持久化；服务端仅提供 `POST /api/send` 与按 `chat_id` 订阅的 `GET /api/events`（SSE），不在服务端保存聊天内容。

**配置片段示例（仅本地媒体）**：

```yaml
media:
  root: /var/tmp/clawbridge
clients:
  - id: example
    driver: example
    enabled: true
```

**飞书（`driver: feishu`）配置示例**（长连接；`options` 对应 `drivers/feishu` 解析字段）：

```yaml
media:
  root: /var/tmp/clawbridge
clients:
  - id: feishu-bot-1
    driver: feishu
    enabled: true
    options:
      app_id: cli_xxxxxxxx
      app_secret: "your_app_secret"
      # 事件订阅：验证与加密（与开放平台「事件订阅」配置一致）
      verification_token: "your_verification_token"
      encrypt_key: "" # 未启用加密时可留空字符串
      # 国际版 Lark 填 true，国内飞书默认 false
      is_lark: false
      # 允许发信的用户；空列表表示接受所有人（会打安全告警日志），显式开放可填 ["*"]
      allow_from:
        - "feishu:ou_xxxxxxxx"
        - ou_yyyyyyyy
      # 群聊触发：仅 @ 机器人，或消息前缀（与 PicoClaw group_trigger 语义一致）
      group_trigger:
        mention_only: true
        prefixes:
          - /ask
          - /bot
```

**Slack（`driver: slack`）配置示例**（[Socket Mode](https://api.slack.com/apis/connections/socket)；`options` 对应 `drivers/slack`）：

```yaml
clients:
  - id: slack-bot-1
    driver: slack
    enabled: true
    options:
      bot_token: "xoxb-..."
      app_token: "xapp-..."
      allow_from:
        - "slack:U01234567"
        - U89abcdef
      group_trigger:
        mention_only: true
        prefixes:
          - /ask
```

出站 `To.ChatID` 使用频道 ID，可选在线程中回复：`C1234567890/1234567890.123456`（`频道ID/父消息 thread_ts`），或使用 `thread_id` / `reply_to_id` 字段（见 bus 类型）。

### 1.2 门面：创建与生命周期

```go
package clawbridge

type Option func(*bridgeOptions)

// WithMediaBackend 注入宿主自实现的媒体存储（实现 media.Backend 接口即可；文档中也可称「自定义 MediaStore」）。
// 用于 S3/MinIO、自建对象存储、加密盘等任意后端。设置后 **应忽略** Config.Media 对内置后端的构造（由该 Backend 全权负责 Put/Open/RemoveScope）。
// 未设置时，New **仅**根据 Config.Media 构造内置 **local** 实现，**不包含** S3 等云端配置项。
func WithMediaBackend(b media.Backend) Option

// WithOutboundSendNotify 在每次 Driver.Send 结束后调用（成功或失败）；info 含 Message、Err、成功时的 MessageID（见 §4）。
// 本库不在 Manager 内重试；失败时可自行 [Bus.PublishOutbound] 再投队列（建议 errors.Is 判断临时错误并加退避/限流）。
func WithOutboundSendNotify(n OutboundSendNotify) Option

// OutboundSendNotify / OutboundSendNotifyInfo 为根包类型别名（定义在 client 子包）。

// New 解析配置、构造 Bus、Media、ClientManager；此时尚未监听端口或连接 IM。
func New(cfg Config, opts ...Option) (*Bridge, error)

type Bridge struct { /* 未导出字段 */ }

// Start 启动：按 Clients 拉起各 Driver（长连接 / webhook 等），启动出站派发循环。
// **不**保证由本库把多个 client 的 webhook 注册到同一 HTTP 端口；需单端口多路径时由宿主自建 Server（见 [im-bridge-technical-design.md](./im-bridge-technical-design.md) §3.7）。
func (b *Bridge) Start(ctx context.Context) error

// Stop 优雅停止：停止接收新出站、等待在途 Send 完成（实现可设超时），Stop Driver，关闭 Bus。
func (b *Bridge) Stop(ctx context.Context) error

// Media 返回全局媒体后端；宿主或策略层可对 Inbound 的 Locator 调用 Open（例如非 Driver 路径）。
func (b *Bridge) Media() media.Backend

// Bus 返回消息总线；宿主通过 ConsumeInbound / PublishOutbound 与库交互（见 §3）。
func (b *Bridge) Bus() *bus.MessageBus

// Reply 快捷回复入站会话：语义与 §1.3 包级 Reply 相同；包级 Reply 建议委托给 `Instance()` 得到的 Bridge 的此方法。
func (b *Bridge) Reply(ctx context.Context, in *InboundMessage, text, mediaPath string) error

// UpdateStatus / EditMessage 委托给内部 Manager：仅当对应 Driver 实现可选接口时成功；否则返回 ErrCapabilityUnsupported（见 §3.4）。
func (b *Bridge) UpdateStatus(ctx context.Context, req *UpdateStatusRequest) error
func (b *Bridge) EditMessage(ctx context.Context, req *EditMessageRequest) error
```

**错误语义**：`New` 在配置非法、Driver 未知、内置 local `Media` 构造失败时返回错误；`Start` 在端口占用、鉴权失败等时返回错误。

**媒体小结**：**默认 = 仅本地**（`MediaConfig.Root`）；**扩展 = 自带 Backend**（`WithMediaBackend`），库不维护各云厂商的配置结构。

### 1.3 包级 API（可选，更简单）

在 **根包 `clawbridge`** 内维护 **进程级默认实例**（内部仍为 `*Bridge`），宿主 **只需 `import` 根包** 即可完成初始化与收发（消息类型通过 **类型别名** 暴露，避免再引 `bus`）。

**约束**：

- **同一进程至多初始化一次**；重复 `Init` 返回 `ErrAlreadyInitialized`。
- 需要 **多个独立 Bridge** 时 **不要** 用包级 API，改用 **`New`**。
- `Start` / `PublishOutbound` 等须在 **`Init` 成功之后** 调用；否则返回 `ErrNotInitialized`（实现约定）。

```go
package clawbridge

// 根包类型别名：与 bus 包同名结构体一致，仅少写一层 import
type (
    InboundMessage        = bus.InboundMessage
    OutboundMessage       = bus.OutboundMessage
    Peer                  = bus.Peer
    SenderInfo            = bus.SenderInfo
    Recipient             = bus.Recipient
    MediaPart             = bus.MediaPart
    UpdateStatusRequest   = bus.UpdateStatusRequest
    EditMessageRequest    = bus.EditMessageRequest
)

var (
    ErrAlreadyInitialized = errors.New("clawbridge: already initialized")
    ErrNotInitialized     = errors.New("clawbridge: not initialized")
)

// Init 创建默认 Bridge 并保存；等价于 New + 内部 SetDefault。
func Init(cfg Config, opts ...Option) error

// SetDefault 将已有 *Bridge 设为包级默认（测试替换、或自行 New 后挂上）。
// 若 b == nil，表示清除默认实例（测试 teardown）；生产慎用。
func SetDefault(b *Bridge)

// Instance 返回当前包级 Bridge；未 Init 且未 SetDefault 时返回 ErrNotInitialized。
func Instance() (*Bridge, error)

func Start(ctx context.Context) error
func Stop(ctx context.Context) error

func PublishOutbound(ctx context.Context, msg *OutboundMessage) error
func UpdateStatus(ctx context.Context, req *UpdateStatusRequest) error
func EditMessage(ctx context.Context, req *EditMessageRequest) error
func ConsumeInbound(ctx context.Context) (InboundMessage, error)

// Reply 快捷回复当前入站会话：根据 in 填充 ClientID、To（ChatID / Kind）、ReplyToID（MessageID），再发 text 与可选单附件。
// mediaPath 为空则不带 Parts；非空则 Parts = []MediaPart{{Path: mediaPath}}（Locator 约定同 §2.3）。
// in == nil 或 text 与 mediaPath 均为空时返回 ErrInvalidMessage。
func Reply(ctx context.Context, in *InboundMessage, text, mediaPath string) error

// Media 委托给默认 Bridge.Media()；须在 Init 成功后使用。未初始化时返回 nil 或 panic 由实现 **固定一种** 并在发行说明写明。
func Media() media.Backend
```

**语义**：`PublishOutbound` / `ConsumeInbound` / `Media` / `Reply` 等价于 `Instance()` 取得默认 `*Bridge` 再委托其 `Bus()`（或内部等价路径）；`Reply` 本质是 **组装 `OutboundMessage` 后调用 `PublishOutbound`**。

**`Reply` 字段映射（实现约定）**：

| 出站字段 | 来源 |
|----------|------|
| `ClientID` | `in.Channel` |
| `To.ChatID` | `in.ChatID` |
| `To.Kind` | `in.Peer.Kind` |
| `To.UserID` | 默认 **不填**；若某 Driver 私聊必须填 `UserID`，宿主应使用 `PublishOutbound` 自行构造 `Recipient` |
| `ReplyToID` | `in.MessageID` |
| `Text` | 参数 `text` |
| `Parts` | `mediaPath != ""` 时为单元素 `{Path: mediaPath}`，否则 `nil` |

---

## 2. 消息格式

类型定义在 **`bus` 子包**；根包 **`clawbridge` 提供与 §1.3 相同的类型别名**，使用包级 API 时 **无需** 再 `import "…/bus"`。

### 2.1 公共类型

```go
package bus

// Peer 表示会话侧「端点类型 + ID」，与具体 IM 的 room/chat 概念对应。
type Peer struct {
    Kind string `json:"kind,omitempty"` // direct | group | channel | ""
    ID   string `json:"id,omitempty"`
}

// SenderInfo 发送方展示与鉴权用；CanonicalID 建议 "driver:id" 形式，便于跨系统对账。
type SenderInfo struct {
    Platform    string `json:"platform,omitempty"`
    PlatformID  string `json:"platform_id,omitempty"`
    CanonicalID string `json:"canonical_id,omitempty"`
    Username    string `json:"username,omitempty"`
    DisplayName string `json:"display_name,omitempty"`
}

// InboundMessage 由 Driver 产生，经 Bus 交给宿主。
type InboundMessage struct {
    Channel    string            `json:"channel"`              // = ClientConfig.ID
    ChatID     string            `json:"chat_id"`              // 平台会话 ID
    MessageID  string            `json:"message_id,omitempty"` // 平台消息 ID
    Sender     SenderInfo        `json:"sender"`
    Peer       Peer              `json:"peer"`
    Content    string            `json:"content,omitempty"`
    MediaPaths []string          `json:"media_paths,omitempty"` // Media Locator 列表
    ReceivedAt int64             `json:"received_at,omitempty"` // Unix 毫秒，可选
    Metadata   map[string]string `json:"metadata,omitempty"`    // 平台扩展：reply_to、raw 类型等
}

// Recipient 出站目标；ChatID 通常必填，UserID 按平台选填。
type Recipient struct {
    ChatID string `json:"chat_id"`
    UserID string `json:"user_id,omitempty"`
    Kind   string `json:"kind,omitempty"` // direct | group | channel | ""
}

// MediaPart 单段附件；Path 为 Media Locator（默认本地路径；自定义 Backend 下可为其它字符串）。
type MediaPart struct {
    Path        string `json:"path"`
    Caption     string `json:"caption,omitempty"`
    Filename    string `json:"filename,omitempty"`
    ContentType string `json:"content_type,omitempty"`
}

// OutboundMessage 宿主构造，经 Bus 交给 Manager → Driver Send。
type OutboundMessage struct {
    ClientID  string            `json:"client_id"`            // 对应 ClientConfig.ID
    To        Recipient         `json:"to"`
    Text      string            `json:"text,omitempty"`
    Parts     []MediaPart       `json:"parts,omitempty"`
    ReplyToID string            `json:"reply_to_id,omitempty"`
    ThreadID  string            `json:"thread_id,omitempty"`
    Metadata  map[string]string `json:"metadata,omitempty"`
}

// UpdateStatusRequest：更新**某条已存在消息**在 UI 上的「处理中 / 已完成 / 处理异常」等状态（**消息级**，非会话级打字）。
// 由可选接口 MessageStatusUpdater 使用；与 Send 分离。
type UpdateStatusRequest struct {
    ClientID  string            `json:"client_id"`
    To        Recipient         `json:"to"`                   // 与 OutboundMessage.To 一致，供平台 API 路由
    MessageID string            `json:"message_id"`           // 平台消息 ID；**必填**（消息级必须指向具体消息）
    State     string            `json:"state"`                // 见下「状态取值」
    Metadata  map[string]string `json:"metadata,omitempty"`   // 平台扩展
}

// 状态取值（字符串常量，实现放在 bus 包，例如 bus.StatusProcessing）。
// 未列出的取值若某 Driver 不支持，返回错误或由该 Driver 文档说明。
const (
    StatusProcessing = "processing" // 正在处理
    StatusCompleted  = "completed"  // 处理完成
    StatusFailed     = "failed"     // 处理异常（失败）；详情可放 Metadata，键名由 Driver 文档约定
)

// EditMessageRequest：编辑**已发送**的正文（及可选附件策略由 Driver 文档约定）。
// 与 Send 分离；**不得**用 Metadata 把编辑塞进 Send。
type EditMessageRequest struct {
    ClientID  string            `json:"client_id"`
    To        Recipient         `json:"to"`
    MessageID string            `json:"message_id,omitempty"` // 空则见 §2.2.1
    Text      string            `json:"text,omitempty"`
    Parts     []MediaPart       `json:"parts,omitempty"`
    Metadata  map[string]string `json:"metadata,omitempty"`
}
```

### 2.2 字段约束摘要

| 消息 | 约束 |
|------|------|
| **Inbound** | `Channel`、`ChatID`、`Sender`、`Peer` 应有值；`Content` 与 `MediaPaths` 可同时为空（如纯事件，实现可过滤）；`MediaPaths` 每项为 **Locator**。 |
| **Outbound** | `ClientID`、`To.ChatID`（及平台要求的其它字段）**必填**；`Text` 与 `Parts` 至少其一非空（否则实现可返回 `ErrInvalidMessage`）；`ReplyToID` / `ThreadID` **可选**。 |
| **UpdateStatusRequest** | `ClientID`、`To`、`MessageID`、`State` **必填**；`State` 为 §2.1 所列常量或 Driver 文档扩展值。 |
| **EditMessageRequest** | `ClientID`、`To` **必填**；`MessageID` **可选**（空则 §2.2.1）；`Text` 与 `Parts` 是否允许全空由平台决定，实现可返回 `ErrInvalidMessage`。 |

### 2.2.1 「最后一条发送」的默认 MessageID（仅 Edit）

对 **`EditMessageRequest`**：若 **`MessageID` 为空**，Driver **应**将其解析为：在本实例内，针对同一 **`ClientID` + `To`（按 `ChatID`、`Kind`、`UserID` 全字段参与匹配；空字符串与未设置视为同一键）** 下，**最近一次 `Send` 成功**所对应的那条消息的 **`MessageID`**。

- **仅统计 `Send`**，不包含仅由平台产生、未经过本 Driver `Send` 的消息。
- **并发**：以 **`Send` 成功返回的先后**为准维护「最后一条」；多 goroutine 同时发送时，宿主若需编辑指定条，**应显式填写 `MessageID`**。
- 若无法解析（尚无成功 `Send`、或平台不支持编辑），返回 **`ErrInvalidMessage`** 或与 **`ErrSendFailed`** 区分的明确错误（实现约定并在发行说明中列出）。

### 2.3 Media Locator（与消息字段一致）

- **内置默认 Backend**：`Put` 返回的 Locator 为 **本地路径**；宿主可直接 `os.Open`，或与 `Bridge.Media().Open` 等价（实现可统一走 `Open`）。
- **自定义 Backend**：Locator 形态由 **宿主实现约定**（例如 `s3://bucket/key`、预签名 `https://`）；**必须**在自有 `Open` 中解析；核心库 **不** 内置这些 scheme 的配置或客户端。

---

## 3. 调用方式

### 3.1 消息总线 API

```go
package bus

type MessageBus struct { /* ... */ }

func NewMessageBus() *MessageBus

// PublishInbound 由 Driver 调用；宿主一般不使用。
func (b *MessageBus) PublishInbound(ctx context.Context, msg *InboundMessage) error

// ConsumeInbound 宿主阻塞读取，直到 ctx 取消或 Bus 关闭。常见模式：单独 goroutine 内 for 循环 + 回调业务。
func (b *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, error)

// PublishOutbound 宿主调用：发信到指定 ClientID + To。
func (b *MessageBus) PublishOutbound(ctx context.Context, msg *OutboundMessage) error

// RunOutboundDispatch 在单独 goroutine 中消费出站队列并调用 handler（Bridge 内为 Manager.HandleOutbound）。
// handler 返回的错误 **不会** 停止循环；每条消息 handler 内为单次 Send，再投队列见 §4。
func (b *MessageBus) RunOutboundDispatch(ctx context.Context, h func(context.Context, *OutboundMessage) error) error

// Close 停止总线；与 Bridge.Stop 协调顺序（通常 Stop 内先关出站派发再 Close Bus）。
func (b *MessageBus) Close()
```

**并发**：`PublishOutbound` 可多 goroutine 调用；**当前**实现为 **单条**带缓冲出站 channel，**无**内置 per-client 限流；与平台速率限制相关的排队/限速建议在宿主侧或后续扩展中完成（设计文档 §3.7）。

### 3.2 媒体后端 API（宿主可选使用）

```go
package media

type Backend interface {
    Put(ctx context.Context, scope, name string, r io.Reader, size int64, contentType string) (loc string, err error)
    Open(ctx context.Context, loc string) (io.ReadCloser, error)
    RemoveScope(ctx context.Context, scope string) error
}
```

- **scope**：建议 `clientID:chatID:messageID`，与 [设计文档](./im-bridge-technical-design.md) 一致，用于 `RemoveScope` 批量清理。
- **默认**：Locator 为本地路径时，宿主可 `os.Open`，也可统一 `Media().Open`。
- **自定义 Backend**：凡非普通本地路径的 Locator，由宿主在 `Open` 内处理；Driver 出站读附件时同样调用注入的 `Backend.Open`。

### 3.3 典型集成伪代码

```go
import "github.com/lengzhao/clawbridge/bus"

b, err := clawbridge.New(cfg)
if err != nil { log.Fatal(err) }
if err := b.Start(ctx); err != nil { log.Fatal(err) }
defer b.Stop(shutdownCtx)

msgBus := b.Bus()
go func() {
    for {
        in, err := msgBus.ConsumeInbound(ctx)
        if err != nil { return }
        // 业务处理；需要读附件：
        // for _, loc := range in.MediaPaths { r, err := b.Media().Open(ctx, loc); ... }
        _ = msgBus.PublishOutbound(ctx, &bus.OutboundMessage{
            ClientID:  in.Channel,
            To:        bus.Recipient{ChatID: in.ChatID, Kind: in.Peer.Kind},
            Text:      "pong",
            ReplyToID: in.MessageID,
        })
    }
}()
```

**包级写法（单 import、少传 `Bridge`）**：

```go
import "github.com/lengzhao/clawbridge"

if err := clawbridge.Init(cfg); err != nil { log.Fatal(err) }
if err := clawbridge.Start(ctx); err != nil { log.Fatal(err) }
defer clawbridge.Stop(shutdownCtx)

go func() {
    for {
        in, err := clawbridge.ConsumeInbound(ctx)
        if err != nil { return }
        // 等价于下面手写 Outbound，仅回复文本、无附件时 mediaPath 传 ""
        _ = clawbridge.Reply(ctx, &in, "pong", "")
        // 带一个本地路径（或其它 Locator）的附件：
        // _ = clawbridge.Reply(ctx, &in, "请看附件", "/tmp/out.png")
    }
}()
```

### 3.4 Driver 扩展（供 fork / 内置实现参考）

最小 **`Driver`** 仍只有 `Send`。以下接口 **可选**：Manager 或宿主通过 **`type assert`** 判断后调用；**未实现** 即不具备该能力。

```go
package client

// Factory 在 init 中注册：RegisterDriver("feishu", NewFeishu)
type Factory func(ctx context.Context, cfg ClientConfig, deps Deps) (Driver, error)

type Deps struct {
    Bus    *bus.MessageBus
    Media  media.Backend
}

type Driver interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    // Send 返回平台消息 ID（成功且已知时；否则 ""）与错误。
    Send(ctx context.Context, msg *bus.OutboundMessage) (sentMessageID string, err error)
}

// MessageStatusUpdater：按消息更新「处理中 / 完成 / 异常」等状态（消息级，见 bus.UpdateStatusRequest）。
type MessageStatusUpdater interface {
    UpdateStatus(ctx context.Context, req *bus.UpdateStatusRequest) error
}

// MessageEditor：编辑已发送内容；语义与 Send 分离；`MessageID` 空则见 §2.2.1。
type MessageEditor interface {
    EditMessage(ctx context.Context, req *bus.EditMessageRequest) error
}

func RegisterDriver(name string, f Factory)
```

**与 `Send` 的关系**：**禁止**用 `OutboundMessage.Metadata` 表达「本条其实是编辑」；编辑一律走 **`EditMessage`**。占位符、卡片多步更新等 **IM Client 内部行为** 仍由 Driver 在 `Send` 内自行实现，**不**占用上述接口。

宿主 **通常不调用** `RegisterDriver`。内置 Driver 在 **`drivers/drivers.go`** 中集中空白导入；用户只需：

```go
import _ "github.com/lengzhao/clawbridge/drivers"
```

新增内置实现时，在该文件追加一行 `_ "github.com/lengzhao/clawbridge/drivers/<name>"` 即可。第三方 Driver 仍可单独 `import _ "…/yourmodule"` 自注册。

---

## 4. 发送错误与再投队列（出站）

对外暴露与 PicoClaw 类似的 **哨兵错误**，供宿主或 Driver 用 `errors.Is` 判断：

```go
var (
    ErrNotRunning  = errors.New("clawbridge: client not running")
    ErrRateLimited = errors.New("clawbridge: rate limited")
    ErrTemporary   = errors.New("clawbridge: temporary failure")
    ErrSendFailed  = errors.New("clawbridge: send failed")
    ErrInvalidMessage = errors.New("clawbridge: invalid outbound message")
    ErrCapabilityUnsupported = errors.New("clawbridge: driver does not support this capability")
)
```

**ClientManager 出站行为（已实现）**：

- **`HandleOutbound`** 对每条消息 **只调用一次** **`Driver.Send`**；失败则打 **slog** 错误日志并返回 error；**不在库内退避重试**。
- 可选 **`WithOutboundSendNotify`**：每次 Send 结束后调用，**`OutboundSendNotifyInfo`** 含 **`Message`**、**`Err`**（成功时为 `nil`）、**`MessageID`**（成功且 Driver 返回平台 id 时；失败时为空）。失败时若希望再投队列，可用 **`errors.Is(info.Err, ErrTemporary)`** / **`ErrRateLimited`** 等判断后 **`Bus.PublishOutbound(ctx, info.Message)`**（自行 sleep / 限流 / 次数上限）。
- **`ErrNotRunning`**、**`ErrSendFailed`**、未知 client 等：同样单次 `Send` 失败即返回；`RunOutboundDispatch` **忽略** handler 的 error，派发循环不停止。
- **noop** 等仅实现 `Send` 的 Driver：**不**支持 `UpdateStatus` / `EditMessage`，返回 **`ErrCapabilityUnsupported`**（见 §1.2）。

---

## 5. JSON / HTTP 边界（可选）

若宿主通过 **HTTP 将 OutboundMessage 提交给本进程**，请求体可直接采用 **§2.1 的 JSON 字段名**；鉴权、幂等与队列语义由网关层定义，**不属于** 核心库强制范围。

---

## 6. 版本与稳定性

- 对外保证：`Bridge`、包级 `Init` / `PublishOutbound` / `Reply` 等、`Config`、消息类型（含根包别名）、`media.Backend` 在 **v1** 内尽量 **向后兼容**；破坏性变更递增主版本。
- `Metadata` 键名由各 Driver 文档列出，**不保证** 跨 Driver 统一。
