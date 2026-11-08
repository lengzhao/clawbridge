package catalog

import "testing"

func TestByID(t *testing.T) {
	e := ByID("weixin")
	if e == nil || e.Strategy != StrategyClawbridge || e.PluginKind != "platform/weixin" {
		t.Fatalf("weixin: %#v", e)
	}
	e = ByID("telegram")
	if e == nil || e.Strategy != StrategyClawbridge || e.PluginKind != "platform/telegram" {
		t.Fatalf("telegram: %#v", e)
	}
	e = ByID("discord")
	if e == nil || e.Strategy != StrategyClawbridge || e.PluginKind != "platform/discord" {
		t.Fatalf("discord: %#v", e)
	}
}

func TestImplemented(t *testing.T) {
	n := len(Implemented())
	if n < 4 {
		t.Fatalf("expected at least feishu/lark/slack/weixin, got %d", n)
	}
}
