// 列出已注册 onboarding 驱动，或打印某一驱动的说明（instructions_only 含 Payload）。
//
//	go run ./examples/onboarding-print -list
//	go run ./examples/onboarding-print -driver slack
//	go run ./examples/onboarding-print -drive slack  // 同 -driver
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/lengzhao/clawbridge/client"
	_ "github.com/lengzhao/clawbridge/drivers"
)

func main() {
	list := flag.Bool("list", false, "列出已注册 onboarding 的 driver 名（需 import drivers）")
	var driverName string
	flag.StringVar(&driverName, "driver", "", "driver 名：telegram / slack / feishu / webchat / noop / weixin 等")
	flag.StringVar(&driverName, "drive", "", "简写，与 -driver 相同")
	flag.Parse()

	if *list {
		for _, d := range client.ListOnboardingDrivers() {
			fmt.Println(d)
		}
		return
	}

	if driverName == "" {
		slog.Error("需要 -driver / -drive，或 -list")
		flag.Usage()
		os.Exit(2)
	}

	flow, err := client.NewOnboardingFlow(driverName, nil, nil)
	if err != nil {
		slog.Error("NewOnboardingFlow", "err", err)
		os.Exit(1)
	}

	desc := flow.Descriptor()
	fmt.Printf("Driver:       %s\n", desc.Driver)
	fmt.Printf("Kind:         %s\n", desc.Kind)
	fmt.Printf("DisplayName:  %s\n", desc.DisplayName)
	if desc.HelpURL != "" {
		fmt.Printf("HelpURL:      %s\n", desc.HelpURL)
	}
	if len(desc.ParamsHelp) > 0 {
		fmt.Println("\nParams (Onboarding.Params):")
		for _, o := range desc.ParamsHelp {
			line := fmt.Sprintf("  - %s (%s)", o.Key, o.Type)
			if o.DefaultValue != "" {
				line += fmt.Sprintf(" default=%s", o.DefaultValue)
			}
			fmt.Println(line)
			if o.Description != "" {
				fmt.Printf("      %s\n", o.Description)
			}
		}
	}
	if len(desc.Fields) > 0 {
		fmt.Println("\nOptions keys (config after login):")
		for _, f := range desc.Fields {
			sec := ""
			if f.Secret {
				sec = " (secret)"
			}
			fmt.Printf("  - %s%s\n", f.Key, sec)
		}
	}

	ctx := context.Background()

	if desc.Kind != client.OnboardingInstructionsOnly {
		fmt.Println("\n该驱动需要交互式引导（非纯说明）。例如 weixin:")
		fmt.Println("  go run ./examples/weixin-onboard -listen 127.0.0.1:8769")
		fmt.Println("\n或使用统一 API: client.RunOnboarding → client.ReportOnboarding")
		return
	}

	sess, err := flow.Start(ctx)
	if err != nil {
		slog.Error("Start", "err", err)
		os.Exit(1)
	}

	fmt.Println("\n--- Payload (instructions & links) ---")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(sess.Payload); err != nil {
		slog.Error("encode payload", "err", err)
		os.Exit(1)
	}

	_, err = flow.Wait(ctx, sess)
	if errors.Is(err, client.ErrManualOnboarding) {
		fmt.Println("\n(OnboardingFlow.Wait 返回 ErrManualOnboarding；高层请用 RunOnboarding + Phase=manual)")
	} else if err != nil {
		slog.Error("Wait", "err", err)
		os.Exit(1)
	}
}
