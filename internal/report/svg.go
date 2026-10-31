package report

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/andrii2g/kube-probe-restart-loop/internal/sim"
)

// SVG renders comparison variants against the same time scale. Text is XML-escaped.
func SVG(results []sim.Result) string {
	var b strings.Builder
	height := 150 + len(results)*320
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="%d" viewBox="0 0 1280 %d" role="img" aria-labelledby="title desc">`, height, height)
	b.WriteString(`<title id="title">Kubernetes probe lifecycle comparison</title><desc id="desc">Aligned application, probe, and lifecycle timelines. Hover over events for causes. Green means success or ready; red means failure or restart; amber means timeout or initialization.</desc><style>text{font-family:ui-sans-serif,system-ui,Segoe UI,sans-serif;fill:#1e293b;font-size:12px}.heading{font-size:24px;font-weight:700}.name{font-size:17px;font-weight:700}.muted{fill:#64748b}.small{font-size:10px}</style><rect width="100%" height="100%" fill="#f1f5f9"/><text x="32" y="40" class="heading">Probe decisions → container consequences</text><text x="32" y="65" class="muted">Deterministic virtual time · one Pod / one container · simplified fixed restart delay</text>`)
	// Shared axis even when callers provide different observation windows.
	duration := time.Duration(1)
	for _, r := range results {
		if r.Config.Duration > duration {
			duration = r.Config.Duration
		}
	}
	x := func(t time.Duration) float64 { return 190 + float64(t)/float64(duration)*1040 }
	text := func(px, py float64, s, cls string) {
		fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" class="%s">%s</text>`, px, py, cls, html.EscapeString(s))
	}
	rect := func(from, to time.Duration, y, h float64, color, title string) {
		fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="3" fill="%s"><title>%s</title></rect>`, x(from), y, x(to)-x(from), h, color, html.EscapeString(title))
	}
	for i, r := range results {
		top := 100 + float64(i)*320
		fmt.Fprintf(&b, `<rect x="24" y="%.0f" width="1232" height="304" rx="12" fill="white"/>`, top)
		name := r.Config.Name
		if len([]rune(name)) > 32 {
			name = string([]rune(name)[:29]) + "…"
		}
		text(42, top+28, name, "name")
		first := "never"
		if r.Metrics.FirstReady != nil {
			first = r.Metrics.FirstReady.String()
		}
		text(450, top+28, fmt.Sprintf("%d restarts  |  first ready: %s  |  availability: %.1f%%", r.Metrics.Restarts, first, r.Metrics.Availability*100), "muted")
		labels := []string{"APPLICATION", "startup", "liveness", "readiness", "CONTAINER", "SERVICE TRAFFIC"}
		ys := []float64{top + 58, top + 102, top + 132, top + 162, top + 205, top + 243}
		for j, label := range labels {
			text(42, ys[j]+15, label, "small")
			fmt.Fprintf(&b, `<path d="M190 %.0f H1230" stroke="#e2e8f0"/>`, ys[j]+12)
		}
		for tick := 0; tick <= 6; tick++ {
			t := time.Duration(int64(duration) * int64(tick) / 6)
			px := x(t)
			fmt.Fprintf(&b, `<path d="M%.2f %.0f V%.0f" stroke="#e2e8f0" stroke-dasharray="2 4"/>`, px, top+48, top+266)
			text(px-10, top+286, fmt.Sprintf("%.0fs", t.Seconds()), "small muted")
		}
		for _, s := range r.Spans {
			color := "#94a3b8"
			if s.State == "serving" {
				color = "#10b981"
			} else if s.State == "initializing" {
				color = "#fbbf24"
			} else if s.CanServe {
				color = "#38bdf8"
			} else if s.State != "stopped" && s.State != "restart wait" && s.State != "terminating" {
				color = "#fb923c"
			}
			title := fmt.Sprintf("%s–%s: %s; can serve=%t; instance=%d", s.From, s.To, s.State, s.CanServe, s.Instance)
			rect(s.From, s.To, ys[0], 24, color, title)
			if width := x(s.To) - x(s.From); width > 75 {
				label := []rune(s.State)
				limit := int((width - 12) / 6)
				if len(label) > limit {
					label = append(label[:limit-1], '…')
				}
				text(x(s.From)+6, ys[0]+16, string(label), "small")
			}
			phaseColor, phase := "#cbd5e1", "waiting"
			if s.Phase == "terminating" {
				phaseColor, phase = "#fb7185", "terminating"
			} else if s.Phase == "running" {
				phaseColor, phase = "#bfdbfe", "running"
			}
			rect(s.From, s.To, ys[4], 22, phaseColor, fmt.Sprintf("container #%d: %s", s.Instance, phase))
			trafficColor, traffic := "#e2e8f0", "unready: no normal Service traffic"
			if s.Ready {
				trafficColor, traffic = "#6ee7b7", "ready and able to serve"
				if !s.CanServe {
					trafficColor, traffic = "#fda4af", "ready BUT unable to serve"
				}
			}
			rect(s.From, s.To, ys[5], 20, trafficColor, traffic+fmt.Sprintf(" (%s–%s)", s.From, s.To))
		}
		pending := map[sim.Kind]time.Duration{}
		for _, e := range r.Events {
			row := ys[1]
			if e.Probe == sim.Liveness {
				row = ys[2]
			}
			if e.Probe == sim.Readiness {
				row = ys[3]
			}
			if e.Type == "probe_started" {
				pending[e.Probe] = e.At
			}
			if e.Type == "probe_result" {
				color, symbol := "#059669", "✓"
				if strings.HasPrefix(e.Detail, "failure") {
					color, symbol = "#dc2626", "×"
				}
				if strings.HasPrefix(e.Detail, "timeout") {
					color, symbol = "#d97706", "!"
				}
				title := fmt.Sprintf("%s %s #%d: %s", e.At, e.Probe, e.Instance, e.Detail)
				if from, ok := pending[e.Probe]; ok && from < e.At {
					rect(from, e.At, row+8, 8, color, title)
				}
				delete(pending, e.Probe)
				fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.0f" r="8" fill="%s"><title>%s</title></circle><text x="%.2f" y="%.0f" text-anchor="middle" style="fill:white;font-size:10px">%s</text>`, x(e.At), row+12, color, html.EscapeString(title), x(e.At), row+16, symbol)
			}
			if e.Type == "termination_started" {
				pending = map[sim.Kind]time.Duration{}
				fmt.Fprintf(&b, `<path d="M%.2f %.0f V%.0f" stroke="#e11d48" stroke-width="2" stroke-dasharray="4 3"><title>%s</title></path>`, x(e.At), top+50, top+265, html.EscapeString(fmt.Sprintf("%s: %s — %s", e.At, e.Probe, e.Detail)))
			}
			if e.Type == "container_started" && e.Instance > 1 {
				text(x(e.At)+3, ys[4]+15, fmt.Sprintf("#%d", e.Instance), "small")
			}
		}
	}
	text(32, float64(height)-15, "✓ success    × failure    ! timeout    dashed red: termination    green traffic: ready    pink traffic: ready but unable to serve", "small muted")
	b.WriteString(`</svg>`)
	return b.String()
}
