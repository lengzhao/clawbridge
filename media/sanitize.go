package media

import "strings"

func sanitizeScope(scope string) string {
	// One directory segment per scope string; avoid path escape.
	repl := strings.NewReplacer("..", "_", "/", "_", "\\", "_", ":", "_")
	return repl.Replace(scope)
}
