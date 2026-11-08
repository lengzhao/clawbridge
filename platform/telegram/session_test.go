package telegram

import "testing"

func TestBuildSessionKey(t *testing.T) {
	tests := []struct {
		share                      bool
		chat, thread, user         int64
		want                       string
	}{
		{false, 100, 0, 55, "telegram:100:55"},
		{false, 100, 7, 55, "telegram:100:7:55"},
		{true, 100, 0, 55, "telegram:100"},
		{true, 100, 7, 55, "telegram:100:7"},
	}
	for _, tt := range tests {
		got := string(buildSessionKey(tt.share, tt.chat, int(tt.thread), tt.user))
		if got != tt.want {
			t.Fatalf("share=%v chat=%d thread=%d user=%d → %q want %q",
				tt.share, tt.chat, tt.thread, tt.user, got, tt.want)
		}
	}
}

func TestNewRequiresToken(t *testing.T) {
	_, err := New(Config{}, Deps{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsBadProxy(t *testing.T) {
	_, err := New(Config{Token: "tok", ProxyURL: "://bad"}, Deps{})
	if err == nil {
		t.Fatal("expected proxy parse error")
	}
}
