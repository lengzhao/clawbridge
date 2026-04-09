package weixin

import "github.com/lengzhao/clawbridge/client"

func init() {
	client.RegisterDriver("weixin", New)
}
