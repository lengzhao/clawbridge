// Package drivers registers all built-in IM drivers. Side effects only.
//
// Import this package instead of listing each driver:
//
//	import _ "github.com/lengzhao/clawbridge/drivers"
//
// Add new built-in drivers by appending a blank import in this file.
package drivers

import (
	_ "github.com/lengzhao/clawbridge/drivers/feishu"
	_ "github.com/lengzhao/clawbridge/drivers/webchat"
	_ "github.com/lengzhao/clawbridge/drivers/noop"
	_ "github.com/lengzhao/clawbridge/drivers/slack"
)
