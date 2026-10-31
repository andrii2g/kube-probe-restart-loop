package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareAndRun(t *testing.T) {
	for _, mode := range []string{"run", "compare"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			var out, errout bytes.Buffer
			if err := execute([]string{mode, "../../examples/slow-start.yaml", "--out", dir}, &out, &errout); err != nil {
				t.Fatal(err, errout.String())
			}
			data, err := os.ReadFile(filepath.Join(dir, "events.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "aggressive-liveness") {
				t.Fatal("missing first variant")
			}
			if _, err := os.Stat(filepath.Join(dir, "timeline.svg")); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "startup-protected") != (mode == "compare") {
				t.Fatal(out.String())
			}
		})
	}
}
func TestCLIRejectsInvalidRequests(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown", "file"}, {"run", "missing.yaml"}, {"run", "../../examples/slow-start.yaml", "--variant", "missing"}, {"compare", "../../examples/slow-start.yaml", "--variant", "startup-protected"}, {"run", "../../examples/slow-start.yaml", "extra"}} {
		var out bytes.Buffer
		if err := execute(args, &out, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestSelectedVariant(t *testing.T) {
	var out bytes.Buffer
	if err := execute([]string{"run", "../../examples/slow-start.yaml", "--variant", "startup-protected", "--out", t.TempDir()}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Restarts: 0") || !strings.Contains(out.String(), "First ready: 30s") {
		t.Fatal(out.String())
	}
}
