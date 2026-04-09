package webchat

import (
	"testing"
	"time"
)

func TestMediaTicketRoundTrip(t *testing.T) {
	d := &driver{id: "c1", listen: "127.0.0.1:9"}
	tok, err := d.signMediaTicket("/tmp/x.bin", "x.bin", "application/octet-stream", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.parseMediaTicket(tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Loc != "/tmp/x.bin" || got.Name != "x.bin" || got.Type != "application/octet-stream" {
		t.Fatalf("payload mismatch: %+v", got)
	}
}

func TestMediaTicketWrongSecret(t *testing.T) {
	d1 := &driver{id: "a", listen: "127.0.0.1:1"}
	d2 := &driver{id: "b", listen: "127.0.0.1:1"}
	tok, err := d1.signMediaTicket("/x", "f", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d2.parseMediaTicket(tok); err == nil {
		t.Fatal("expected verify failure")
	}
}
