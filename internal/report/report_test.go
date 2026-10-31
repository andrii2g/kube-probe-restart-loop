package report

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/andrii2g/kube-probe-restart-loop/internal/sim"
)

func TestSVGValidEscapedAndDeterministic(t *testing.T) {
	c := sim.Config{Name: `<script>&"example`, Duration: time.Minute, RestartDelay: time.Second, Application: sim.Application{StartupDuration: 20 * time.Second}, Probes: map[sim.Kind]sim.Probe{sim.Readiness: sim.DefaultProbe()}}
	r, err := sim.Run(c)
	if err != nil {
		t.Fatal(err)
	}
	svg := SVG([]sim.Result{r, r})
	if svg != SVG([]sim.Result{r, r}) {
		t.Fatal("unstable SVG")
	}
	if strings.Contains(svg, "<script>") || !strings.Contains(svg, "&lt;script&gt;") {
		t.Fatal("unescaped label")
	}
	dec := xml.NewDecoder(strings.NewReader(svg))
	count := 0
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if tag, ok := token.(xml.StartElement); ok && tag.Name.Local == "svg" {
			count++
		}
	}
	if count != 1 || !strings.Contains(svg, "SERVICE TRAFFIC") {
		t.Fatal("missing SVG structure")
	}
}
