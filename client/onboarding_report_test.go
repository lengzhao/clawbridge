package client

import (
	"testing"
)

func TestCredentialReportKeys_fieldOrderThenSortedExtras(t *testing.T) {
	cred := map[string]any{
		"extra_z": "z",
		"token":   "secret",
		"base_url": "https://example/",
		"ilink_bot_id": "b",
	}
	desc := OnboardingDescriptor{
		Fields: []CredentialField{
			{Key: "base_url"},
			{Key: "token"},
			{Key: "ilink_bot_id"},
		},
	}
	keys := credentialReportKeys(cred, desc)
	want := []string{"base_url", "token", "ilink_bot_id", "extra_z"}
	if len(keys) != len(want) {
		t.Fatalf("len=%d got %#v want %#v", len(keys), keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("index %d: got %q want %q (full %#v)", i, keys[i], want[i], keys)
		}
	}
}
