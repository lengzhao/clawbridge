// Package catalog lists IM platforms (IDs 与 cc-connect 配置对齐) 及在本生态中的装配方式。
// 仅登记可直接 pluginkit 挂载的实现，不经过中间线协议网关。
package catalog

// Strategy 表示宿主应如何装配该平台。
type Strategy string

const (
	// StrategyAgentKit 使用 agentkit 内置 platform/*（需 import agentkit/plugins）。
	StrategyAgentKit Strategy = "agentkit"
	// StrategyClawbridge 使用本仓库 platform/*。
	StrategyClawbridge Strategy = "clawbridge"
	// StrategyPlanned 尚未在 clawbridge/agentkit 提供运行插件，需在 platform/<id> 原生实现。
	StrategyPlanned Strategy = "planned"
)

// Entry 描述一个逻辑平台。
type Entry struct {
	ID         string
	Label      string
	Strategy   Strategy
	PluginKind string // 非空时表示已可装配的 pluginkit use
}

// All 为当前推荐清单。已实现：agentkit 飞书/Lark/Slack、clawbridge 微信；其余为 planned。
var All = []Entry{
	{ID: "feishu", Label: "飞书", Strategy: StrategyAgentKit, PluginKind: "platform/feishu"},
	{ID: "lark", Label: "Lark", Strategy: StrategyAgentKit, PluginKind: "platform/lark"},
	{ID: "slack", Label: "Slack", Strategy: StrategyAgentKit, PluginKind: "platform/slack"},
	{ID: "weixin", Label: "微信", Strategy: StrategyClawbridge, PluginKind: "platform/weixin"},
	{ID: "telegram", Label: "Telegram", Strategy: StrategyClawbridge, PluginKind: "platform/telegram"},
	{ID: "discord", Label: "Discord", Strategy: StrategyPlanned},
	{ID: "dingtalk", Label: "钉钉", Strategy: StrategyPlanned},
	{ID: "wecom", Label: "企业微信", Strategy: StrategyPlanned},
	{ID: "qq", Label: "QQ (OneBot)", Strategy: StrategyPlanned},
	{ID: "qqbot", Label: "QQ 官方 Bot", Strategy: StrategyPlanned},
	{ID: "yuanbao", Label: "腾讯元宝", Strategy: StrategyPlanned},
	{ID: "line", Label: "LINE", Strategy: StrategyPlanned},
	{ID: "weibo", Label: "微博", Strategy: StrategyPlanned},
	{ID: "matrix", Label: "Matrix", Strategy: StrategyPlanned},
	{ID: "webex", Label: "Webex", Strategy: StrategyPlanned},
	{ID: "max", Label: "MAX", Strategy: StrategyPlanned},
}

// ByID 按平台 id 查询。
func ByID(id string) *Entry {
	for i := range All {
		if All[i].ID == id {
			return &All[i]
		}
	}
	return nil
}

// Implemented 返回当前可装配条目（StrategyAgentKit 或 StrategyClawbridge 且 PluginKind 非空）。
func Implemented() []Entry {
	var out []Entry
	for _, e := range All {
		if e.PluginKind != "" && (e.Strategy == StrategyAgentKit || e.Strategy == StrategyClawbridge) {
			out = append(out, e)
		}
	}
	return out
}
