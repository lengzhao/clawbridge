package client

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lengzhao/clawbridge/bus"
	"github.com/lengzhao/clawbridge/config"
)

func init() {
	RegisterDriver("retrytest_flaky", newRetryTestFlaky)
	RegisterDriver("retrytest_twice_first_fail", newRetryTestTwiceFirstFail)
	RegisterDriver("retrytest_with_mid", newRetryTestWithMID)
}

type retryTestFlakyDriver struct {
	failLeft int
}

func newRetryTestFlaky(ctx context.Context, cfg config.ClientConfig, deps Deps) (Driver, error) {
	_ = ctx
	_ = deps
	n := 0
	if cfg.Credentials != nil {
		switch v := cfg.Credentials["_test_failures"].(type) {
		case int:
			n = v
		case float64:
			n = int(v)
		case int64:
			n = int(v)
		}
	}
	return &retryTestFlakyDriver{failLeft: n}, nil
}

func (d *retryTestFlakyDriver) Name() string { return "retrytest_flaky" }

func (d *retryTestFlakyDriver) Start(ctx context.Context) error { return nil }
func (d *retryTestFlakyDriver) Stop(ctx context.Context) error  { return nil }

func (d *retryTestFlakyDriver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	_ = ctx
	_ = msg
	if d.failLeft > 0 {
		d.failLeft--
		return "", fmt.Errorf("flaky: %w", ErrTemporary)
	}
	return "flaky-ok", nil
}

func TestHandleOutboundSendSucceeds(t *testing.T) {
	mb := bus.NewMessageBus()
	m := NewManager(mb, nil)
	ctx := context.Background()
	err := m.StartClients(ctx, []config.ClientConfig{{
		ID:      "x",
		Driver:  "retrytest_flaky",
		Enabled: true,
		Credentials: map[string]any{
			"_test_failures": float64(0),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopClients(ctx)

	msg := &bus.OutboundMessage{ClientID: "x", To: bus.Recipient{ChatID: "c"}, Text: "hi"}
	if err := m.HandleOutbound(ctx, msg); err != nil {
		t.Fatal(err)
	}
}

func TestHandleOutboundSendFails(t *testing.T) {
	mb := bus.NewMessageBus()
	m := NewManager(mb, nil)
	ctx := context.Background()
	err := m.StartClients(ctx, []config.ClientConfig{{
		ID:      "x",
		Driver:  "retrytest_flaky",
		Enabled: true,
		Credentials: map[string]any{
			"_test_failures": float64(1),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopClients(ctx)

	msg := &bus.OutboundMessage{ClientID: "x", To: bus.Recipient{ChatID: "c"}, Text: "hi"}
	err = m.HandleOutbound(ctx, msg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTemporary) {
		t.Fatalf("want ErrTemporary, got %v", err)
	}
}

func TestOutboundSendNotifyFailure(t *testing.T) {
	mb := bus.NewMessageBus()
	var got *bus.OutboundMessage
	var gotErr error
	var gotMID string
	notify := func(ctx context.Context, info OutboundSendNotifyInfo) {
		_ = ctx
		got = info.Message
		gotErr = info.Err
		gotMID = info.MessageID
	}
	m := NewManager(mb, nil, WithOutboundSendNotify(notify))
	ctx := context.Background()
	err := m.StartClients(ctx, []config.ClientConfig{{
		ID:      "x",
		Driver:  "retrytest_flaky",
		Enabled: true,
		Credentials: map[string]any{
			"_test_failures": float64(1),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopClients(ctx)

	msg := &bus.OutboundMessage{ClientID: "x", To: bus.Recipient{ChatID: "c"}, Text: "hi"}
	_ = m.HandleOutbound(ctx, msg)
	if got == nil {
		t.Fatal("expected notify")
	}
	if got.Text != msg.Text || got.ClientID != msg.ClientID {
		t.Fatalf("notify message mismatch")
	}
	if gotErr == nil || !errors.Is(gotErr, ErrTemporary) {
		t.Fatalf("want ErrTemporary in notify, got %v", gotErr)
	}
	if gotMID != "" {
		t.Fatalf("want empty MessageID on failure, got %q", gotMID)
	}
}

type retryTestWithMIDDriver struct{}

func newRetryTestWithMID(ctx context.Context, cfg config.ClientConfig, deps Deps) (Driver, error) {
	_ = ctx
	_ = cfg
	_ = deps
	return &retryTestWithMIDDriver{}, nil
}

func (d *retryTestWithMIDDriver) Name() string                           { return "retrytest_with_mid" }
func (d *retryTestWithMIDDriver) Start(ctx context.Context) error        { return nil }
func (d *retryTestWithMIDDriver) Stop(ctx context.Context) error         { return nil }
func (d *retryTestWithMIDDriver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	_ = ctx
	_ = msg
	return "platform-msg-42", nil
}

func TestOutboundSendNotifySuccessWithMessageID(t *testing.T) {
	mb := bus.NewMessageBus()
	var gotErr error
	var gotMID string
	notify := func(ctx context.Context, info OutboundSendNotifyInfo) {
		_ = ctx
		gotErr = info.Err
		gotMID = info.MessageID
	}
	m := NewManager(mb, nil, WithOutboundSendNotify(notify))
	ctx := context.Background()
	err := m.StartClients(ctx, []config.ClientConfig{{
		ID:     "x",
		Driver: "retrytest_with_mid",
		Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopClients(ctx)

	msg := &bus.OutboundMessage{ClientID: "x", To: bus.Recipient{ChatID: "c"}, Text: "hi"}
	if err := m.HandleOutbound(ctx, msg); err != nil {
		t.Fatal(err)
	}
	if gotErr != nil {
		t.Fatalf("want nil Err on success notify, got %v", gotErr)
	}
	if gotMID != "platform-msg-42" {
		t.Fatalf("MessageID: want platform-msg-42, got %q", gotMID)
	}
}

type retryTestTwiceFirstFailDriver struct {
	sendCount atomic.Int32
}

var lastTwiceFirstFail atomic.Pointer[retryTestTwiceFirstFailDriver]

func newRetryTestTwiceFirstFail(ctx context.Context, cfg config.ClientConfig, deps Deps) (Driver, error) {
	_ = ctx
	_ = cfg
	_ = deps
	d := &retryTestTwiceFirstFailDriver{}
	lastTwiceFirstFail.Store(d)
	return d, nil
}

func (d *retryTestTwiceFirstFailDriver) Name() string { return "retrytest_twice_first_fail" }
func (d *retryTestTwiceFirstFailDriver) Start(ctx context.Context) error { return nil }
func (d *retryTestTwiceFirstFailDriver) Stop(ctx context.Context) error  { return nil }

func (d *retryTestTwiceFirstFailDriver) Send(ctx context.Context, msg *bus.OutboundMessage) (string, error) {
	_ = ctx
	_ = msg
	n := d.sendCount.Add(1)
	if n == 1 {
		return "", fmt.Errorf("first: %w", ErrTemporary)
	}
	return "mid-after-retry", nil
}

func TestOutboundSendNotifyRepublishOutbound(t *testing.T) {
	mb := bus.NewMessageBus()
	m := NewManager(mb, nil, WithOutboundSendNotify(func(ctx context.Context, info OutboundSendNotifyInfo) {
		if info.Err != nil && errors.Is(info.Err, ErrTemporary) {
			_ = mb.PublishOutbound(ctx, info.Message)
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := m.StartClients(ctx, []config.ClientConfig{{
		ID:     "x",
		Driver: "retrytest_twice_first_fail",
		Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopClients(ctx)

	go func() { _ = mb.RunOutboundDispatch(ctx, m.HandleOutbound) }()

	msg := &bus.OutboundMessage{ClientID: "x", To: bus.Recipient{ChatID: "c"}, Text: "hi"}
	if err := mb.PublishOutbound(ctx, msg); err != nil {
		t.Fatal(err)
	}

	td := lastTwiceFirstFail.Load()
	if td == nil {
		t.Fatal("driver not constructed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if td.sendCount.Load() >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected 2 Send calls after republish, got %d", td.sendCount.Load())
}
