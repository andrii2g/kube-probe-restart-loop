// Package report renders deterministic results without rerunning the simulation.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrii2g/kube-probe-restart-loop/internal/sim"
)

func Summary(results []sim.Result) string {
	var b strings.Builder
	b.WriteString("Probe simulation (fixed restart delay; no cluster required)\n")
	for _, r := range results {
		m := r.Metrics
		first := "never"
		if m.FirstReady != nil {
			first = m.FirstReady.String()
		}
		fmt.Fprintf(&b, "\n%s\n  Restarts: %d (%.2f/min)\n  First ready: %s\n  Unavailable: %s\n  Ready but unable to serve: %s\n  Simulated service availability: %.2f%%\n", r.Config.Name, m.Restarts, m.RestartsPerMinute, first, m.Unavailable, m.ReadyButUnable, m.Availability*100)
		for _, k := range sim.Kinds {
			fmt.Fprintf(&b, "  %s failures: %d (timeouts: %d)\n", k, m.Failed[k], m.Timeouts[k])
		}
	}
	return b.String()
}

// Write emits one comparison event document and a readable summary.
func Write(dir string, results []sim.Result) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "events.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "summary.txt"), []byte(Summary(results)), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "timeline.svg"), []byte(SVG(results)), 0644)
}
