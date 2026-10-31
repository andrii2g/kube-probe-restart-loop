package sim

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func base() Config {
	return Config{Name: "test", Duration: 60 * time.Second, RestartDelay: time.Second, Application: Application{StartupDuration: 20 * time.Second}, Probes: map[Kind]Probe{}}
}
func probe(period, failures int) Probe {
	p := DefaultProbe()
	p.PeriodSeconds = period
	p.FailureThreshold = failures
	return p
}
func run(t *testing.T, c Config) Result {
	t.Helper()
	r, err := Run(c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func boolp(b bool) *bool { return &b }
func TestSlowStartupLoopAndProtection(t *testing.T) {
	c := base()
	c.Probes[Liveness] = probe(5, 3)
	c.Probes[Readiness] = probe(5, 1)
	bad := run(t, c)
	if bad.Metrics.Restarts != 5 || bad.Metrics.FirstReady != nil || bad.Metrics.Availability != 0 {
		t.Fatalf("bad configuration: %+v", bad.Metrics)
	}
	c.Probes[Startup] = probe(5, 6)
	good := run(t, c)
	if good.Metrics.Restarts != 0 || good.Metrics.FirstReady == nil || *good.Metrics.FirstReady != 20*time.Second {
		t.Fatalf("protected: %+v", good.Metrics)
	}
	for _, e := range good.Events {
		if (e.Probe == Liveness || e.Probe == Readiness) && e.At < 20*time.Second {
			t.Fatalf("probe escaped startup gate: %+v", e)
		}
	}
}
func TestOverloadOnlyWithdrawsReadiness(t *testing.T) {
	c := base()
	c.Application.StartupDuration = 0
	c.Probes[Liveness] = probe(5, 1)
	c.Probes[Readiness] = probe(5, 1)
	c.Application.Windows = []Window{{From: 10 * time.Second, To: 20 * time.Second, Clock: "scenario", State: "overloaded", CanServe: boolp(false), Responses: map[Kind]Response{Readiness: {Success: false}}}}
	r := run(t, c)
	if r.Metrics.Restarts != 0 || r.Metrics.Unavailable != 10*time.Second || r.Metrics.Failed[Readiness] != 2 {
		t.Fatalf("%+v", r.Metrics)
	}
}
func TestReadinessSuccessStreakAndReset(t *testing.T) {
	c := base()
	c.Application.StartupDuration = 0
	p := probe(2, 1)
	p.SuccessThreshold = 2
	c.Probes[Readiness] = p
	c.Application.Windows = []Window{{From: 2 * time.Second, To: 4 * time.Second, Clock: "scenario", Responses: map[Kind]Response{Readiness: {Success: false}}}}
	r := run(t, c)
	if r.Metrics.FirstReady == nil || *r.Metrics.FirstReady != 6*time.Second {
		t.Fatalf("%+v", r.Metrics)
	}
}
func TestLivenessFailuresMustBeConsecutive(t *testing.T) {
	c := base()
	c.Application.StartupDuration = 0
	c.Probes[Liveness] = probe(2, 2)
	for _, from := range []time.Duration{0, 4 * time.Second} {
		c.Application.Windows = append(c.Application.Windows, Window{From: from, To: from + time.Second, Clock: "scenario", Responses: map[Kind]Response{Liveness: {Success: false}}})
	}
	if r := run(t, c); r.Metrics.Restarts != 0 {
		t.Fatal(r.Metrics)
	}
}
func TestTimeoutOccursAtDeadline(t *testing.T) {
	c := base()
	c.Application.StartupDuration = 0
	c.Application.ResponseLatency = 3 * time.Second
	c.Probes[Liveness] = probe(5, 1)
	c.RestartDelay = 100 * time.Second
	r := run(t, c)
	for _, e := range r.Events {
		if e.Type == "termination_started" {
			if e.At != time.Second {
				t.Fatal(e)
			}
			return
		}
	}
	t.Fatal("missing termination")
}
func TestResponseExactlyAtTimeoutSucceeds(t *testing.T) {
	c := base()
	c.Application.StartupDuration = 0
	c.Application.ResponseLatency = time.Second
	c.Probes[Readiness] = probe(2, 1)
	r := run(t, c)
	if r.Metrics.Timeouts[Readiness] != 0 || *r.Metrics.FirstReady != time.Second {
		t.Fatal(r.Metrics)
	}
}
func TestRestartDiscardsOldResponses(t *testing.T) {
	c := base()
	c.Duration = 8 * time.Second
	c.Probes[Liveness] = probe(5, 1)
	p := probe(5, 1)
	p.TimeoutSeconds = 10
	c.Probes[Readiness] = p
	c.Application.Windows = []Window{{From: 0, To: time.Hour, Clock: "scenario", Responses: map[Kind]Response{Readiness: {Success: true, Latency: 3 * time.Second}}}}
	r := run(t, c)
	if r.Metrics.FirstReady != nil {
		t.Fatalf("stale response made new instance ready: %+v", r.Metrics)
	}
}
func TestAbsentReadinessDoesNotProveServing(t *testing.T) {
	r := run(t, base())
	if *r.Metrics.FirstReady != 0 || r.Metrics.ReadyButUnable != 20*time.Second || r.Metrics.Unavailable != 0 {
		t.Fatal(r.Metrics)
	}
	if diff := r.Metrics.Availability - 2.0/3; diff < -1e-10 || diff > 1e-10 {
		t.Fatal(r.Metrics.Availability)
	}
}
func TestStartupFailureRestarts(t *testing.T) {
	c := base()
	c.Probes[Startup] = probe(2, 2)
	c.RestartDelay = time.Second
	c.TerminationDelay = time.Second
	r := run(t, c)
	if r.Metrics.Restarts == 0 || r.Metrics.FirstReady != nil {
		t.Fatal(r.Metrics)
	}
}
func TestStartupUnlockDelay(t *testing.T) {
	c := base()
	c.Probes[Startup] = probe(5, 10)
	p := probe(5, 1)
	p.InitialDelaySeconds = 3
	c.Probes[Readiness] = p
	r := run(t, c)
	if *r.Metrics.FirstReady != 23*time.Second {
		t.Fatal(r.Metrics)
	}
}
func TestDeterminismAndSpanCoverage(t *testing.T) {
	c := base()
	c.Probes[Startup] = probe(3, 3)
	c.RestartDelay = 2 * time.Second
	a := run(t, c)
	b := run(t, c)
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if !reflect.DeepEqual(aj, bj) {
		t.Fatal("nondeterministic output")
	}
	at := time.Duration(0)
	for _, s := range a.Spans {
		if s.From != at || s.To <= s.From {
			t.Fatal(s)
		}
		at = s.To
	}
	if at != c.Duration {
		t.Fatal(at)
	}
}
func TestValidation(t *testing.T) {
	for _, k := range []Kind{Startup, Liveness} {
		c := base()
		p := DefaultProbe()
		p.SuccessThreshold = 2
		c.Probes[k] = p
		if _, err := Run(c); err == nil {
			t.Fatal("accepted invalid success threshold")
		}
	}
	c := base()
	c.Duration = 0
	if _, err := Run(c); err == nil {
		t.Fatal("accepted zero duration")
	}
}
