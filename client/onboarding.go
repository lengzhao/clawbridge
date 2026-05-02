package client

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/lengzhao/clawbridge/config"
)

// OnboardingKind describes how the user completes credential setup (for UI hints).
type OnboardingKind string

const (
	OnboardingManualPaste      OnboardingKind = "manual_paste"
	OnboardingOAuthBrowser     OnboardingKind = "oauth_browser"
	OnboardingDeviceCode       OnboardingKind = "device_code"
	OnboardingQRPoll           OnboardingKind = "qr_poll"
	OnboardingInstructionsOnly OnboardingKind = "instructions_only"
)

// CredentialField names one key written into ClientConfig.Options after a successful flow.
type CredentialField struct {
	Key    string
	Secret bool
}

// DriverOptField documents one key accepted in [Onboarding.Params] for interactive onboarding (discovery / UI).
type DriverOptField struct {
	Key          string
	Type         string // "string", "duration", etc.
	Description  string
	DefaultValue string // display-only hint; empty if none
}

// OnboardingDescriptor is static metadata for a driver onboarding implementation.
type OnboardingDescriptor struct {
	Driver      string
	Kind        OnboardingKind
	DisplayName string
	Fields     []CredentialField
	ParamsHelp []DriverOptField // documented keys for [Onboarding.Params]
	HelpURL    string
}

// OnboardingSession is returned from [OnboardingFlow.Start]. Payload is driver-defined
// (e.g. weixin: page_url, qr_link).
type OnboardingSession struct {
	Driver  string
	Payload map[string]any
}

// OnboardingFlow acquires credentials for one client driver instance.
// Implementations are not required to be safe for concurrent use or reusable after Wait returns.
type OnboardingFlow interface {
	Descriptor() OnboardingDescriptor
	Start(ctx context.Context) (*OnboardingSession, error)
	Wait(ctx context.Context, sess *OnboardingSession) (options map[string]any, err error)
}

// OnboardingFactory builds an [OnboardingFlow] from driver-specific options (often YAML-decoded map) and UI hooks.
// opts may be nil when the driver needs no configuration; hooks may be nil (drivers should treat hooks as optional).
type OnboardingFactory func(opts map[string]any, hooks *OnboardingHooks) (OnboardingFlow, error)

// OnboardingClientBuilder merges credential map (from interactive Wait) into one [config.ClientConfig].
// Nil means [DefaultOnboardingClientConfig] is used.
type OnboardingClientBuilder func(clientID string, credentialOpts map[string]any, spec Onboarding) config.ClientConfig

type onboardingDriverReg struct {
	factory OnboardingFactory
	builder OnboardingClientBuilder
}

var (
	onboardingMu      sync.RWMutex
	onboardingDrivers = make(map[string]onboardingDriverReg)
)

// RegisterOnboarding registers onboarding for driver: flow factory and optional merge into [config.ClientConfig].
// Pass builder nil to use [DefaultOnboardingClientConfig] after interactive Wait.
func RegisterOnboarding(driver string, f OnboardingFactory, builder OnboardingClientBuilder) {
	onboardingMu.Lock()
	defer onboardingMu.Unlock()
	onboardingDrivers[driver] = onboardingDriverReg{factory: f, builder: builder}
}

// ListOnboardingDrivers returns sorted registered driver names (requires importing drivers that call [RegisterOnboarding]).
func ListOnboardingDrivers() []string {
	onboardingMu.RLock()
	defer onboardingMu.RUnlock()
	out := make([]string, 0, len(onboardingDrivers))
	for d := range onboardingDrivers {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// DescribeOnboardingDriver returns static descriptor by constructing a flow with empty opts and no hooks.
func DescribeOnboardingDriver(driver string) (OnboardingDescriptor, error) {
	flow, err := NewOnboardingFlow(driver, nil, nil)
	if err != nil {
		return OnboardingDescriptor{}, err
	}
	return flow.Descriptor(), nil
}

// NewOnboardingFlow constructs a registered onboarding flow. opts may be nil if the driver needs no [Onboarding.Params].
// hooks may be nil; use [SilentOnboardingHooks] or [DefaultTerminalOnboardingHooks] when you need explicit behavior.
func NewOnboardingFlow(driver string, opts map[string]any, hooks *OnboardingHooks) (OnboardingFlow, error) {
	onboardingMu.RLock()
	e, ok := onboardingDrivers[driver]
	onboardingMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("client: no onboarding registered for driver %q", driver)
	}
	return e.factory(opts, hooks)
}

func mergeOnboardingClientConfig(driver, clientID string, credentialOpts map[string]any, spec Onboarding) config.ClientConfig {
	onboardingMu.RLock()
	e, ok := onboardingDrivers[driver]
	onboardingMu.RUnlock()
	if ok && e.builder != nil {
		return e.builder(clientID, credentialOpts, spec)
	}
	return DefaultOnboardingClientConfig(driver, clientID, credentialOpts, spec)
}
