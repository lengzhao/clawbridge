package bus_test

import (
	"testing"

	"github.com/lengzhao/clawbridge/bus"
)

func TestRecipientKey(t *testing.T) {
	a := bus.Recipient{SessionID: "c1", Kind: "group", UserID: "u1"}
	b := bus.Recipient{SessionID: "c1", Kind: "group", UserID: "u2"}
	if bus.RecipientKey(a) == bus.RecipientKey(b) {
		t.Fatal("expected different keys for different UserID")
	}
}
