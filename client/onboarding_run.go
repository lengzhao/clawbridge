package client

import (
	"context"
	"fmt"

	"github.com/lengzhao/clawbridge/config"
)

// Onboarding is everything [RunOnboarding] needs: identity, optional driver params, client merge fields, and UI hooks.
// Prefer [NewOnboarding] plus With* methods, or use a composite literal.
type Onboarding struct {
	Driver    string
	ClientID  string
	Params    map[string]any // optional driver factory args (e.g. weixin http_listen)
	AllowFrom []string       // merged into client options when non-empty
	StateDir  string
	Proxy     string // used when credential map has no proxy
	Hooks     *OnboardingHooks
}

// NewOnboarding returns a spec with AllowFrom ["*"]. Other fields are zero.
func NewOnboarding(driver, clientID string) Onboarding {
	return Onboarding{
		Driver:    driver,
		ClientID:  clientID,
		AllowFrom: []string{"*"},
	}
}

func (o Onboarding) WithParams(m map[string]any) Onboarding {
	o.Params = m
	return o
}

func (o Onboarding) WithAllowFrom(ids ...string) Onboarding {
	if len(ids) == 0 {
		return o
	}
	o.AllowFrom = append([]string(nil), ids...)
	return o
}

func (o Onboarding) WithStateDir(dir string) Onboarding {
	o.StateDir = dir
	return o
}

func (o Onboarding) WithProxy(proxy string) Onboarding {
	o.Proxy = proxy
	return o
}

func (o Onboarding) WithHooks(h *OnboardingHooks) Onboarding {
	o.Hooks = h
	return o
}

// Silent sets hooks to no terminal QR/output ([SilentOnboardingHooks]).
func (o Onboarding) Silent() Onboarding {
	o.Hooks = SilentOnboardingHooks()
	return o
}

// OnboardingPhase classifies the outcome of [RunOnboarding].
type OnboardingPhase string

const (
	// OnboardingPhaseReady: credentials obtained (or equivalent); [OnboardingResult.Config] has one enabled client suitable for Start.
	OnboardingPhaseReady OnboardingPhase = "ready"
	// OnboardingPhaseManual: instruction-only driver; fill options then set enabled: true. err from [RunOnboarding] is nil.
	OnboardingPhaseManual OnboardingPhase = "manual"
)

// OnboardingResult is returned by [RunOnboarding].
type OnboardingResult struct {
	Phase      OnboardingPhase
	Config     config.Config
	Descriptor OnboardingDescriptor

	// SessionPayload is copied from [OnboardingFlow.Start] (e.g. qr_link, instructions).
	SessionPayload map[string]any

	// CredentialOpts is the raw map from interactive Wait (subset useful for reporting); empty for manual phase.
	CredentialOpts map[string]any
}

// Ready reports whether [OnboardingResult.Config] can be passed to [clawbridge.New] and started as-is.
func (r OnboardingResult) Ready() bool {
	return r.Phase == OnboardingPhaseReady && len(r.Config.Clients) > 0 && r.Config.Clients[0].Enabled
}

// RunOnboarding runs onboarding for one driver and builds [config.Config]. It does not persist config.
// Use [NewOnboarding] (or a literal [Onboarding]) for the common case; [Onboarding.Params] may be nil.
// Terminal output: nil [Onboarding.Hooks] uses [DefaultTerminalOnboardingHooks]; [Onboarding.Silent] for headless.
// Use [ReportOnboarding] (or your UI) for structured summaries.
//
// Manual-only drivers: returns Phase=[OnboardingPhaseManual], err=nil; Config holds one disabled stub client.
//
// Drivers package must be imported for registration (e.g. _ "github.com/lengzhao/clawbridge/drivers").
func RunOnboarding(ctx context.Context, o Onboarding) (OnboardingResult, error) {
	if o.Driver == "" {
		return OnboardingResult{}, fmt.Errorf("client: Onboarding.Driver is required")
	}
	if o.ClientID == "" {
		return OnboardingResult{}, fmt.Errorf("client: Onboarding.ClientID is required")
	}

	hooks := o.Hooks
	if hooks == nil {
		hooks = DefaultTerminalOnboardingHooks()
	}

	flow, err := NewOnboardingFlow(o.Driver, o.Params, hooks)
	if err != nil {
		return OnboardingResult{}, err
	}
	desc := flow.Descriptor()

	if desc.Kind == OnboardingInstructionsOnly {
		sess, err := flow.Start(ctx)
		if err != nil {
			return OnboardingResult{}, err
		}
		_, _ = flow.Wait(ctx, sess)
		skel := manualSkeletonConfig(o)
		return OnboardingResult{
			Phase:          OnboardingPhaseManual,
			Config:         skel,
			Descriptor:     desc,
			SessionPayload: cloneAnyMap(sess.Payload),
			CredentialOpts: nil,
		}, nil
	}

	sess, err := flow.Start(ctx)
	if err != nil {
		return OnboardingResult{}, err
	}
	cred, err := flow.Wait(ctx, sess)
	if err != nil {
		return OnboardingResult{}, err
	}

	cc := mergeOnboardingClientConfig(o.Driver, o.ClientID, cred, o)
	full := config.Config{Clients: []config.ClientConfig{cc}}
	return OnboardingResult{
		Phase:          OnboardingPhaseReady,
		Config:         full,
		Descriptor:     desc,
		SessionPayload: cloneAnyMap(sess.Payload),
		CredentialOpts: cloneAnyMap(cred),
	}, nil
}

func manualSkeletonConfig(o Onboarding) config.Config {
	return config.Config{
		Clients: []config.ClientConfig{
			{
				ID:      o.ClientID,
				Driver:  o.Driver,
				Enabled: false,
				Options: map[string]any{},
			},
		},
	}
}

