// Package clawbridge provides AgentKit platform plugins and shared platform auth.
//
// Runtime message transport for Lark/Feishu remains AgentKit's platform/lark and
// platform/feishu kinds. This module adds Weixin iLink (platform/weixin) and
// pluginkit auth slots (platform/auth-lark, platform/auth-feishu,
// platform/auth-weixin) plus credential helpers used during setup.
package clawbridge

import (
	_ "github.com/lengzhao/clawbridge/platform/cloudweb"
	_ "github.com/lengzhao/clawbridge/platform/lark"
	_ "github.com/lengzhao/clawbridge/platform/weixin"
)
