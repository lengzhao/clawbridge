package client

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	qrterminal "github.com/mdp/qrterminal/v3"
)

// OnboardingHooks lets embedders replace terminal output and logs during onboarding. Nil function fields mean
// "no output" for that channel. [RunOnboarding] defaults nil [Onboarding.Hooks] to [DefaultTerminalOnboardingHooks].
type OnboardingHooks struct {
	// Info is diagnostic logging (e.g. slog-style message + optional key-value args).
	Info func(msg string, args ...any)
	// Notify is short user-visible text (e.g. one line to stdout).
	Notify func(msg string)
	// ShowQR renders the iLink QR payload string (same content as session qr_link).
	ShowQR func(qrPayload string)
	// LocalQRPage is called when an HTTP page URL is available (browser QR). Nil means use Notify with a default banner.
	LocalQRPage func(pageURL string)
}

// SilentOnboardingHooks is an empty hook set: driver emits no terminal QR, banners, or hook-driven logs.
func SilentOnboardingHooks() *OnboardingHooks {
	return &OnboardingHooks{}
}

// DefaultTerminalOnboardingHooks matches historical CLI behavior: stdout banners + terminal QR + slog.Info for Info.
func DefaultTerminalOnboardingHooks() *OnboardingHooks {
	return &OnboardingHooks{
		Info: func(msg string, args ...any) {
			slog.Info(msg, args...)
		},
		Notify: func(msg string) {
			fmt.Println(msg)
		},
		ShowQR: func(qrPayload string) {
			PrintQRLinkToTerminal(os.Stdout, qrPayload)
		},
		LocalQRPage: defaultLocalQRPageBanner,
	}
}

func defaultLocalQRPageBanner(pageURL string) {
	fmt.Println("\n=======================================================")
	fmt.Println("本地二维码页面（浏览器打开）:")
	fmt.Println(pageURL)
	fmt.Println("=======================================================")
}

// PrintQRLinkToTerminal renders qrPayload as an ASCII QR code to w and prints the raw link line (iLink-style).
func PrintQRLinkToTerminal(w io.Writer, qrPayload string) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintln(w, "\n=======================================================")
	fmt.Fprintln(w, "Please scan the following QR code with WeChat to login:")
	fmt.Fprintln(w, "=======================================================")
	fmt.Fprintln(w)

	cfg := qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     w,
		HalfBlocks: true,
	}
	qrterminal.GenerateWithConfig(qrPayload, cfg)

	fmt.Fprintf(w, "\nQR Code Link: %s\n\n", qrPayload)
}

// LogInfo emits a structured info line when hooks define Info.
func (h *OnboardingHooks) LogInfo(msg string, args ...any) {
	if h != nil && h.Info != nil {
		h.Info(msg, args...)
	}
}

// PrintNotify emits one user-visible line when hooks define Notify.
func (h *OnboardingHooks) PrintNotify(msg string) {
	if h != nil && h.Notify != nil {
		h.Notify(msg)
	}
}

// DisplayQR invokes ShowQR when set.
func (h *OnboardingHooks) DisplayQR(qrPayload string) {
	if h != nil && h.ShowQR != nil {
		h.ShowQR(qrPayload)
	}
}

// BannerLocalPage shows the browser QR URL; uses LocalQRPage or falls back to Notify lines.
func (h *OnboardingHooks) BannerLocalPage(pageURL string) {
	if h == nil {
		return
	}
	if h.LocalQRPage != nil {
		h.LocalQRPage(pageURL)
		return
	}
	if h.Notify != nil {
		h.Notify("\n=======================================================")
		h.Notify("本地二维码页面（浏览器打开）:")
		h.Notify(pageURL)
		h.Notify("=======================================================")
	}
}
