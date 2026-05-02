package client

import (
	"context"
	"errors"
)

// ErrManualOnboarding is returned by [OnboardingFlow.Wait] for instruction-only flows.
// High-level [RunOnboarding] uses Phase=[OnboardingPhaseManual] with err=nil instead.
var ErrManualOnboarding = errors.New(`client: manual onboarding — fill ClientConfig.options using Descriptor.Fields; see SessionPayload ("instructions", "links")`)

// PayloadKeyInstructions is the usual slice-of-steps key in [OnboardingSession.Payload] ([]string).
const PayloadKeyInstructions = "instructions"

// PayloadKeyLinks is the usual key for helpful URLs ([]map[string]string with "title", "url").
const PayloadKeyLinks = "links"

// OnboardingLink is one documentation URL for manual onboarding payloads.
type OnboardingLink struct {
	Title string
	URL   string
}

// ManualInstructionsPayload builds a standard Payload for instruction-only flows.
func ManualInstructionsPayload(instructions []string, links []OnboardingLink) map[string]any {
	m := map[string]any{
		PayloadKeyInstructions: instructions,
	}
	if len(links) > 0 {
		arr := make([]map[string]string, len(links))
		for i, l := range links {
			arr[i] = map[string]string{"title": l.Title, "url": l.URL}
		}
		m[PayloadKeyLinks] = arr
	}
	return m
}

type manualInstructionsFlow struct {
	desc    OnboardingDescriptor
	payload map[string]any
}

// NewManualInstructionsFlow returns an [OnboardingFlow] with Kind [OnboardingInstructionsOnly].
// Start copies payload into the session; Wait always returns [ErrManualOnboarding].
func NewManualInstructionsFlow(desc OnboardingDescriptor, payload map[string]any) OnboardingFlow {
	return &manualInstructionsFlow{desc: desc, payload: cloneAnyMap(payload)}
}

func (f *manualInstructionsFlow) Descriptor() OnboardingDescriptor {
	return f.desc
}

func (f *manualInstructionsFlow) Start(ctx context.Context) (*OnboardingSession, error) {
	_ = ctx
	// Payload is cloned in [RunOnboarding] into SessionPayload; flow owns f.payload.
	return &OnboardingSession{Driver: f.desc.Driver, Payload: f.payload}, nil
}

func (f *manualInstructionsFlow) Wait(ctx context.Context, sess *OnboardingSession) (map[string]any, error) {
	_ = ctx
	_ = sess
	return nil, ErrManualOnboarding
}
