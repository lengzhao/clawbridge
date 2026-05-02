package client

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/lengzhao/clawbridge/config"
	"gopkg.in/yaml.v3"
)

// OnboardingPrintMode selects how [ReportOnboarding] formats output.
type OnboardingPrintMode int

// ParseOnboardingPrintMode parses CLI-style values: none | yaml | json | human (empty defaults to human).
func ParseOnboardingPrintMode(s string) (OnboardingPrintMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "human":
		return OnboardingPrintHuman, nil
	case "none":
		return OnboardingPrintNone, nil
	case "yaml":
		return OnboardingPrintYAML, nil
	case "json":
		return OnboardingPrintJSON, nil
	default:
		return 0, fmt.Errorf("unknown onboarding print mode %q (none|yaml|json|human)", s)
	}
}

const (
	OnboardingPrintNone OnboardingPrintMode = iota
	OnboardingPrintYAML
	OnboardingPrintJSON
	OnboardingPrintHuman
)

// ReportOptions configures [ReportOnboarding].
type ReportOptions struct {
	// MaskSecrets hides secret option values in YAML/JSON/human when true (recommended default).
	MaskSecrets bool
	// ErrWriter receives marshal errors; nil uses io.Discard.
	ErrWriter io.Writer
}

// ReportOnboarding writes a summary of res to w. QR rendering is controlled by [OnboardingHooks], not this reporter.
func ReportOnboarding(w io.Writer, mode OnboardingPrintMode, res OnboardingResult, opts ReportOptions) {
	if mode == OnboardingPrintNone || w == nil {
		return
	}
	errOut := opts.ErrWriter
	if errOut == nil {
		errOut = io.Discard
	}
	mask := opts.MaskSecrets
	switch mode {
	case OnboardingPrintYAML:
		cfg := res.Config
		if mask {
			cfg = maskConfigClients(cfg, res.Descriptor)
		}
		raw, err := yaml.Marshal(&cfg)
		if err != nil {
			fmt.Fprintf(errOut, "client: onboarding yaml: %v\n", err)
			return
		}
		fmt.Fprintf(w, "\n%s\n", string(raw))
	case OnboardingPrintJSON:
		cfg := res.Config
		if mask {
			cfg = maskConfigClients(cfg, res.Descriptor)
		}
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintf(errOut, "client: onboarding json: %v\n", err)
			return
		}
		fmt.Fprintf(w, "\n%s\n", string(raw))
	case OnboardingPrintHuman:
		reportHumanOnboarding(w, res, mask)
	}
}

func secretKeysForMask(desc OnboardingDescriptor) map[string]bool {
	m := map[string]bool{
		"token": true, "bot_token": true, "app_token": true, "app_secret": true,
		"encrypt_key": true, "verification_token": true, "authorization": true,
	}
	for _, f := range desc.Fields {
		if f.Secret {
			m[f.Key] = true
		}
	}
	return m
}

func maskConfigClients(cfg config.Config, desc OnboardingDescriptor) config.Config {
	keys := secretKeysForMask(desc)
	out := cfg
	out.Clients = make([]config.ClientConfig, len(cfg.Clients))
	for i, c := range cfg.Clients {
		out.Clients[i] = c
		if c.Options == nil {
			continue
		}
		opts := cloneAnyMap(c.Options)
		for k := range keys {
			if _, ok := opts[k]; !ok {
				continue
			}
			opts[k] = maskSecretString(credString(opts, k))
		}
		out.Clients[i].Options = opts
	}
	return out
}

func maskSecretString(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-4:]
}

func reportHumanOnboarding(w io.Writer, res OnboardingResult, maskSecrets bool) {
	d := res.Descriptor
	fmt.Fprintf(w, "\n=== %s (%s) ===\n", d.DisplayName, d.Driver)
	if d.HelpURL != "" {
		fmt.Fprintf(w, "Help: %s\n", d.HelpURL)
	}
	if len(d.ParamsHelp) > 0 {
		fmt.Fprintln(w, "\nDriver params (Onboarding.Params):")
		for _, o := range d.ParamsHelp {
			line := fmt.Sprintf("  - %s (%s)", o.Key, o.Type)
			if o.DefaultValue != "" {
				line += fmt.Sprintf(" default=%s", o.DefaultValue)
			}
			fmt.Fprintln(w, line)
			if o.Description != "" {
				fmt.Fprintf(w, "      %s\n", o.Description)
			}
		}
	}
	if len(d.Fields) > 0 {
		fmt.Fprintln(w, "Options keys:")
		for _, f := range d.Fields {
			tag := ""
			if f.Secret {
				tag = " (secret)"
			}
			fmt.Fprintf(w, "  - %s%s\n", f.Key, tag)
		}
	}
	if res.Phase == OnboardingPhaseManual && res.SessionPayload != nil {
		if lines, ok := res.SessionPayload[PayloadKeyInstructions].([]string); ok {
			fmt.Fprintln(w, "\nSteps:")
			for _, line := range lines {
				fmt.Fprintf(w, "  - %s\n", line)
			}
		}
		if links := payloadLinks(res.SessionPayload[PayloadKeyLinks]); len(links) > 0 {
			fmt.Fprintln(w, "\nLinks:")
			for _, L := range links {
				fmt.Fprintf(w, "  - %s\n    %s\n", L["title"], L["url"])
			}
		}
		fmt.Fprintln(w, "\nFill options in config, set enabled: true, then New + Start.")
		return
	}
	if len(res.CredentialOpts) > 0 {
		fmt.Fprintln(w, "\nCredentials acquired (for reference; full secrets only when mask off):")
		maskKeys := secretKeysForMask(d)
		for _, key := range credentialReportKeys(res.CredentialOpts, d) {
			v := credString(res.CredentialOpts, key)
			if v == "" {
				continue
			}
			show := v
			if maskSecrets && maskKeys[key] {
				show = maskSecretString(v)
			}
			fmt.Fprintf(w, "  %s: %q\n", key, show)
		}
	}
}

func credentialReportKeys(cred map[string]any, desc OnboardingDescriptor) []string {
	if len(cred) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(cred))
	var ordered []string
	for _, f := range desc.Fields {
		if credString(cred, f.Key) == "" {
			continue
		}
		ordered = append(ordered, f.Key)
		seen[f.Key] = true
	}
	var extras []string
	for k := range cred {
		if seen[k] || k == "" {
			continue
		}
		if credString(cred, k) == "" {
			continue
		}
		extras = append(extras, k)
	}
	sort.Strings(extras)
	return append(ordered, extras...)
}

func payloadLinks(raw any) []map[string]string {
	switch x := raw.(type) {
	case []map[string]string:
		return x
	case []any:
		var out []map[string]string
		for _, e := range x {
			switch m := e.(type) {
			case map[string]string:
				out = append(out, m)
			case map[string]any:
				out = append(out, map[string]string{
					"title": fmt.Sprint(m["title"]),
					"url":   fmt.Sprint(m["url"]),
				})
			}
		}
		return out
	default:
		return nil
	}
}
