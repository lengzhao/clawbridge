package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLocalBackendDefaultTempDir(t *testing.T) {
	for _, root := range []string{"", "  ", "\t"} {
		b, err := NewLocalBackend(root)
		if err != nil {
			t.Fatalf("root %q: %v", root, err)
		}
		want := filepath.Join(os.TempDir(), "clawbridge")
		if b.Root != want {
			t.Fatalf("root %q: got Root %q, want %q", root, b.Root, want)
		}
	}
}

func TestNewLocalBackendCustomRoot(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "my-media")
	b, err := NewLocalBackend("  " + custom + "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(strings.TrimSpace(custom))
	if err != nil {
		t.Fatal(err)
	}
	if b.Root != want {
		t.Fatalf("Root %q, want %q", b.Root, want)
	}
}
