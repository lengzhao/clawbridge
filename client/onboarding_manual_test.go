package client

import (
	"context"
	"errors"
	"testing"
)

func TestManualInstructionsFlow_WaitReturnsErrManualOnboarding(t *testing.T) {
	desc := OnboardingDescriptor{
		Driver:      "noop",
		Kind:        OnboardingInstructionsOnly,
		DisplayName: "noop",
	}
	payload := ManualInstructionsPayload([]string{"step 1"}, []OnboardingLink{{Title: "Doc", URL: "https://example.test"}})
	f := NewManualInstructionsFlow(desc, payload)
	sess, err := f.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sess.Driver != "noop" {
		t.Fatalf("driver %q", sess.Driver)
	}
	_, err = f.Wait(context.Background(), sess)
	if !errors.Is(err, ErrManualOnboarding) {
		t.Fatalf("Wait err = %v", err)
	}
}
