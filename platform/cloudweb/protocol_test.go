package cloudweb

import "testing"

func TestParseRegisterAck(t *testing.T) {
	raw := []byte(`{"type":"register_ack","ok":true,"capabilities":["text","image"]}`)
	caps, err := parseRegisterAck(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !caps["text"] || !caps["image"] {
		t.Fatalf("caps = %#v", caps)
	}
}

func TestBuildSessionKey(t *testing.T) {
	if got := buildSessionKey("telegram", "c1", "u1", false, ""); got != "telegram:c1:u1" {
		t.Fatalf("got %q", got)
	}
	if got := buildSessionKey("telegram", "c1", "u1", true, ""); got != "telegram:c1" {
		t.Fatalf("got %q", got)
	}
	if got := buildSessionKey("telegram", "c1", "u1", false, "custom:key"); got != "custom:key" {
		t.Fatalf("got %q", got)
	}
}

func TestDeriveBaseURL(t *testing.T) {
	if got := deriveBaseURL("https://gateway.example.com/cloud-web/v1/register"); got != "https://gateway.example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"missing token", Config{Transport: "websocket", BaseURL: "http://x"}, false},
		{"bad transport", Config{Token: "t", Transport: "mqtt"}, false},
		{"ws missing url", Config{Token: "t", Transport: "websocket"}, false},
		{"poll missing base", Config{Token: "t", Transport: "long_poll"}, false},
		{"ws ok", Config{Token: "t", Transport: "websocket", WSURL: "ws://127.0.0.1/ws"}, true},
		{"poll ok", Config{Token: "t", Transport: "long_poll", BaseURL: "http://127.0.0.1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
