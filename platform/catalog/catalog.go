// Package catalog lists IM platforms aligned with cc-connect and how AgentKit
// should attach them: native agentkit plugins, clawbridge natives, or Cloud Web
// bridge adapters speaking cc-connect's wire protocol.
package catalog

// Strategy tells the host how to wire a logical platform ID.
type Strategy string

const (
	// StrategyAgentKit uses a built-in agentkit platform/* plugin.
	StrategyAgentKit Strategy = "agentkit"
	// StrategyClawbridge uses a clawbridge platform/* plugin.
	StrategyClawbridge Strategy = "clawbridge"
	// StrategyBridge uses platform/cloud-web with bridgeName set to the cc-connect
	// platform id (telegram, discord, …) while cc-connect or another gateway
	// owns the real IM protocol.
	StrategyBridge Strategy = "bridge"
)

// Entry is one supported logical platform.
type Entry struct {
	// ID matches cc-connect platform id (web UI / config).
	ID string
	// Label is a short human name.
	Label string
	Strategy Strategy
	// PluginKind is the pluginkit use value when Strategy is agentkit or clawbridge.
	PluginKind string
	// BridgeName is the Cloud Web register "platform" field; required for StrategyBridge.
	BridgeName string
}

// All lists cc-connect platforms and the recommended AgentKit attachment.
// Feishu/Lark/Slack stay on agentkit; Weixin on clawbridge; the rest bridge via Cloud Web.
var All = []Entry{
	{ID: "feishu", Label: "飞书", Strategy: StrategyAgentKit, PluginKind: "platform/feishu"},
	{ID: "lark", Label: "Lark", Strategy: StrategyAgentKit, PluginKind: "platform/lark"},
	{ID: "slack", Label: "Slack", Strategy: StrategyAgentKit, PluginKind: "platform/slack"},
	{ID: "weixin", Label: "微信", Strategy: StrategyClawbridge, PluginKind: "platform/weixin"},
	{ID: "telegram", Label: "Telegram", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "telegram"},
	{ID: "discord", Label: "Discord", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "discord"},
	{ID: "dingtalk", Label: "钉钉", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "dingtalk"},
	{ID: "wecom", Label: "企业微信", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "wecom"},
	{ID: "qq", Label: "QQ (OneBot)", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "qq"},
	{ID: "qqbot", Label: "QQ 官方 Bot", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "qqbot"},
	{ID: "yuanbao", Label: "腾讯元宝", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "yuanbao"},
	{ID: "line", Label: "LINE", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "line"},
	{ID: "weibo", Label: "微博", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "weibo"},
	{ID: "matrix", Label: "Matrix", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "matrix"},
	{ID: "webex", Label: "Webex", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "webex"},
	{ID: "max", Label: "MAX", Strategy: StrategyBridge, PluginKind: "platform/cloud-web", BridgeName: "max"},
	{ID: "cloud_web", Label: "Cloud Web", Strategy: StrategyClawbridge, PluginKind: "platform/cloud-web", BridgeName: "cloud_web"},
}

// ByID returns the catalog entry or nil.
func ByID(id string) *Entry {
	for i := range All {
		if All[i].ID == id {
			return &All[i]
		}
	}
	return nil
}
