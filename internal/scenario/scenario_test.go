package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrii2g/kube-probe-restart-loop/internal/sim"
)

func TestExamples(t *testing.T) {
	paths, err := filepath.Glob("../../examples/*.yaml")
	if err != nil || len(paths) != 6 {
		t.Fatalf("examples: %v %v", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			configs, err := Load(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range configs {
				if _, err := sim.Run(c); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

const minimal = "name: example\nduration: 30s\napplication: {startupDuration: 2s}\nvariants:\n  - name: default\n    probes:\n      readiness: {}\n"

func TestDefaults(t *testing.T) {
	configs, err := Load(strings.NewReader(minimal))
	if err != nil {
		t.Fatal(err)
	}
	c := configs[0]
	if c.RestartDelay != time.Second || c.Application.StartupDuration != 2*time.Second || c.Probes[sim.Readiness] != sim.DefaultProbe() {
		t.Fatal(c)
	}
}
func TestRejectInvalidInput(t *testing.T) {
	cases := []string{
		strings.Replace(minimal, "readiness: {}", "readiness: {periodSeconds: 0}", 1),
		strings.Replace(minimal, "readiness: {}", "readiness: {typo: 3}", 1),
		strings.Replace(minimal, "startupDuration: 2s", "startupDuraton: 2s", 1),
		strings.Replace(minimal, "name: default", "name: ../escape", 1),
		minimal + "---\nname: extra\n",
		minimal + "duration: 10s\n",
		minimal + "restartDelay: 0s\n",
		strings.Replace(minimal, "readiness: {}", "liveness: {successThreshold: 2}", 1),
	}
	for _, s := range cases {
		if _, err := Load(strings.NewReader(s)); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
