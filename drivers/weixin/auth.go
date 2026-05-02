package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lengzhao/clawbridge/client"
)

// AuthFlowOpts configures the interactive QR login flow.
type AuthFlowOpts struct {
	BaseURL string
	BotType string
	Timeout time.Duration
	Proxy   string
}

func normalizeAuthFlowOpts(opts *AuthFlowOpts) {
	if opts.BaseURL == "" {
		opts.BaseURL = "https://ilinkai.weixin.qq.com/"
	}
	if opts.BotType == "" {
		opts.BotType = "3" // Default iLink Bot Type
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
}

func printQRToTerminal(linkPayload string) {
	client.PrintQRLinkToTerminal(os.Stdout, linkPayload)
}

// pollQRCodeLogin polls iLink until QR login confirms or context ends.
func pollQRCodeLogin(ctx context.Context, opts AuthFlowOpts, initial *ApiClient, qrCodeKey string, hooks *client.OnboardingHooks) (botToken, userID, accountID, baseUrl string, err error) {
	pollAPI := initial
	pollTicker := time.NewTicker(2 * time.Second)
	defer pollTicker.Stop()

	scannedPrinted := false

	for {
		select {
		case <-ctx.Done():
			return "", "", "", "", fmt.Errorf("login timeout")
		case <-pollTicker.C:
			statusResp, err := pollAPI.GetQRCodeStatus(ctx, qrCodeKey)
			if err != nil {
				continue
			}

			switch statusResp.Status {
			case "wait":
			case "scaned":
				if !scannedPrinted {
					hooks.PrintNotify("👀 QR Code scanned! Please confirm login on your WeChat app...")
					scannedPrinted = true
				}
			case "confirmed":
				if statusResp.BotToken == "" || statusResp.IlinkBotID == "" {
					return "", "", "", "", fmt.Errorf("login confirmed but missing bot_token or ilink_bot_id")
				}
				hooks.LogInfo("weixin: login successful", "account_id", statusResp.IlinkBotID)

				return statusResp.BotToken, statusResp.IlinkUserID, statusResp.IlinkBotID, statusResp.Baseurl, nil
			case "scaned_but_redirect":
				if statusResp.RedirectHost == "" {
					slog.Warn("weixin: scaned_but_redirect without redirect_host; continuing on current host")
					continue
				}
				nextBaseURL := "https://" + statusResp.RedirectHost + "/"
				nextAPI, nextErr := NewApiClient(nextBaseURL, "", opts.Proxy)
				if nextErr != nil {
					slog.Warn("weixin: switch QR polling host failed", "redirect_host", statusResp.RedirectHost, "err", nextErr)
					continue
				}
				pollAPI = nextAPI
				hooks.LogInfo("weixin: switched QR polling host", "redirect_host", statusResp.RedirectHost)
			case "expired":
				return "", "", "", "", fmt.Errorf("qrcode expired, please try again")
			default:
				slog.Warn("weixin: unknown QR code status", "status", statusResp.Status)
			}
		}
	}
}

// PerformLoginInteractive starts the Weixin QR login flow and blocks until login is successful or times out.
// It prints a QR code to the terminal for the user to scan.
// Returns the BotToken, UserID, AccountID, and BaseUrl on success.
func PerformLoginInteractive(
	ctx context.Context,
	opts AuthFlowOpts,
) (botToken, userID, accountID, baseUrl string, err error) {
	normalizeAuthFlowOpts(&opts)

	api, err := NewApiClient(opts.BaseURL, "", opts.Proxy)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to create api client: %w", err)
	}

	slog.Info("weixin: requesting QR code for interactive login")
	qrResp, err := api.GetQRCode(ctx, opts.BotType)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to get qrcode: %w", err)
	}

	printQRToTerminal(qrResp.QrcodeImgContent)
	fmt.Println("Waiting for scan...")

	timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	cliHooks := client.DefaultTerminalOnboardingHooks()
	return pollQRCodeLogin(timeoutCtx, opts, api, qrResp.Qrcode, cliHooks)
}
