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

func TestExampleOutcomes(t *testing.T) {
	expected := map[string]struct {
		restarts int
		first    time.Duration
		never    bool
	}{
		"aggressive-liveness": {10, 0, true}, "startup-protected": {0, 30 * time.Second, false},
		"readiness-withdrawal": {0, 5 * time.Second, false}, "readiness-only": {0, 5 * time.Second, false},
		"overload-causes-restarts": {4, 5 * time.Second, false}, "timeout-too-short": {4, 0, true},
		"timeout-sufficient": {0, 1500 * time.Millisecond, false}, "responsive": {0, 0, false},
		"dampened": {0, 2 * time.Second, false}, "insufficient-budget": {10, 0, true},
		"sufficient-budget": {0, 30 * time.Second, false},
	}
	paths, _ := filepath.Glob("../../examples/*.yaml")
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		configs, err := Load(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range configs {
			r, err := sim.Run(c)
			if err != nil {
				t.Fatal(err)
			}
			want, ok := expected[c.Name]
			if !ok {
				t.Fatalf("no expected outcome for %s", c.Name)
			}
			m := r.Metrics
			if m.Restarts != want.restarts || (m.FirstReady == nil) != want.never {
				t.Fatalf("%s: %+v", c.Name, m)
			}
			if !want.never && *m.FirstReady != want.first {
				t.Fatalf("%s: first ready %s", c.Name, *m.FirstReady)
			}
			derived := 1 - float64(m.Unavailable+m.ReadyButUnable)/float64(c.Duration)
			if diff := m.Availability - derived; diff < -1e-10 || diff > 1e-10 {
				t.Fatalf("%s: availability accounting mismatch", c.Name)
			}
		}
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
