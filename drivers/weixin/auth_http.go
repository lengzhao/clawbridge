package weixin

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/lengzhao/clawbridge/client"
	"github.com/skip2/go-qrcode"
)

// AuthHTTPDisplayOpts configures the local HTTP page that shows the login QR code.
type AuthHTTPDisplayOpts struct {
	// Listen is the TCP address for the helper HTTP server (e.g. "127.0.0.1:8769").
	// Empty defaults to "127.0.0.1:0" (random port on loopback only).
	Listen string
}

var weixinOnboardPageTmpl = template.Must(template.New("weixin-onboard").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>微信 iLink 扫码登录</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;max-width:32rem;margin:2rem auto;padding:0 1rem;text-align:center;color:#1a1a1a}
h1{font-size:1.25rem;font-weight:600}
img{display:inline-block;border:1px solid #e5e5e5;border-radius:8px;padding:12px;background:#fff}
p.note{color:#666;font-size:.875rem;margin-top:1.5rem;line-height:1.5}
code{font-size:.75rem;word-break:break-all;background:#f5f5f5;padding:.125rem .25rem;border-radius:4px}
</style>
</head>
<body>
<h1>微信 iLink 扫码登录</h1>
<p>请使用微信扫描下方二维码（与终端中的二维码相同）。</p>
<img src="/qr.png" width="280" height="280" alt="登录二维码">
<p class="note">扫码后在微信中确认登录；完成后在运行本向导的<strong>终端</strong>等待结果。</p>
</body>
</html>`))

// startWeixinQRHTTPServer serves HTML + /qr.png for the given QR payload string on listen (e.g. "127.0.0.1:0").
// shutdown must be called when polling finishes.
func startWeixinQRHTTPServer(payload string, listen string) (pageURL string, shutdown func(), err error) {
	if listen == "" {
		listen = "127.0.0.1:0"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := weixinOnboardPageTmpl.Execute(w, nil); err != nil {
			slog.Warn("weixin: onboard page render", "err", err)
		}
	})
	mux.HandleFunc("/qr.png", func(w http.ResponseWriter, r *http.Request) {
		png, encErr := qrcode.Encode(payload, qrcode.Medium, 280)
		if encErr != nil {
			http.Error(w, encErr.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
	})

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return "", nil, fmt.Errorf("listen %s: %w", listen, err)
	}

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Warn("weixin: http server exited", "err", serveErr)
		}
	}()

	shutdown = func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("weixin: http server shutdown", "err", err)
		}
	}

	pageURL = pageURLFromListener(ln.Addr())
	return pageURL, shutdown, nil
}

// PerformLoginInteractiveWithHTTP runs the same QR login flow as [PerformLoginInteractive], but also
// serves a local HTTP page at loopback with a PNG QR code so you can scan from the browser view.
func PerformLoginInteractiveWithHTTP(
	ctx context.Context,
	opts AuthFlowOpts,
	httpOpts AuthHTTPDisplayOpts,
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

	listen := httpOpts.Listen
	if listen == "" {
		listen = "127.0.0.1:0"
	}

	payload := qrResp.QrcodeImgContent
	pageURL, shutdown, err := startWeixinQRHTTPServer(payload, listen)
	if err != nil {
		return "", "", "", "", err
	}
	defer shutdown()

	cliHooks := client.DefaultTerminalOnboardingHooks()
	cliHooks.LogInfo("weixin: local QR page", "url", pageURL)
	cliHooks.BannerLocalPage(pageURL)
	cliHooks.DisplayQR(payload)
	cliHooks.PrintNotify("Waiting for scan...")

	timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	botToken, userID, accountID, baseUrl, err = pollQRCodeLogin(timeoutCtx, opts, api, qrResp.Qrcode, cliHooks)
	if err != nil {
		return "", "", "", "", err
	}
	return botToken, userID, accountID, baseUrl, nil
}

func pageURLFromListener(addr net.Addr) string {
	switch a := addr.(type) {
	case *net.TCPAddr:
		if a.IP.IsLoopback() && a.IP.To4() == nil && !a.IP.IsUnspecified() {
			return fmt.Sprintf("http://[%s]:%d/", a.IP, a.Port)
		}
		host := a.IP.String()
		if a.IP.IsUnspecified() {
			host = "127.0.0.1"
		}
		return fmt.Sprintf("http://%s:%d/", host, a.Port)
	default:
		return fmt.Sprintf("http://%s/", addr.String())
	}
}
