package catalog

import "testing"

func TestByID(t *testing.T) {
	e := ByID("telegram")
	if e == nil || e.Strategy != StrategyBridge || e.BridgeName != "telegram" {
		t.Fatalf("telegram entry: %#v", e)
	}
	if ByID("nope") != nil {
		t.Fatal("expected nil")
	}
}

func TestCatalogCoversAgentKitNatives(t *testing.T) {
	for _, id := range []string{"feishu", "lark", "slack"} {
		e := ByID(id)
		if e == nil || e.Strategy != StrategyAgentKit {
			t.Fatalf("%s: %#v", id, e)
		}
	}
}
