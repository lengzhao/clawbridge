package client_test

import (
	"context"
	"testing"

	"github.com/lengzhao/clawbridge/client"
	_ "github.com/lengzhao/clawbridge/drivers"
)

func TestRunOnboarding_ManualDriver(t *testing.T) {
	res, err := client.RunOnboarding(context.Background(), client.NewOnboarding("slack", "test-slack-1"))
	if err != nil {
		t.Fatalf("err = %v want nil", err)
	}
	if res.Phase != client.OnboardingPhaseManual {
		t.Fatalf("Phase = %q want manual", res.Phase)
	}
	if res.Ready() {
		t.Fatal("expected not Ready")
	}
	if res.Phase != client.OnboardingPhaseManual {
		t.Fatalf("Phase = %q want manual", res.Phase)
	}
	if len(res.Config.Clients) != 1 || res.Config.Clients[0].Enabled {
		t.Fatalf("skeleton client: %+v", res.Config.Clients)
	}
	if res.Config.Clients[0].Driver != "slack" {
		t.Fatalf("driver %q", res.Config.Clients[0].Driver)
	}
}

func TestListOnboardingDrivers(t *testing.T) {
	names := client.ListOnboardingDrivers()
	if len(names) == 0 {
		t.Fatal("expected registered drivers")
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("duplicate %q", n)
		}
		seen[n] = true
	}
}

func TestDescribeOnboardingDriver(t *testing.T) {
	d, err := client.DescribeOnboardingDriver("telegram")
	if err != nil {
		t.Fatal(err)
	}
	if d.Driver != "telegram" || d.Kind != client.OnboardingInstructionsOnly {
		t.Fatalf("%+v", d)
	}
}
