package sim

import (
	"container/heap"
	"fmt"
	"time"
)

type Event struct {
	At       time.Duration `json:"at"`
	Instance int           `json:"instance"`
	Type     string        `json:"type"`
	Probe    Kind          `json:"probe,omitempty"`
	Detail   string        `json:"detail"`
}
type Span struct {
	Phase    string        `json:"phase"`
	From     time.Duration `json:"from"`
	To       time.Duration `json:"to"`
	Instance int           `json:"instance"`
	State    string        `json:"state"`
	Ready    bool          `json:"ready"`
	CanServe bool          `json:"canServe"`
}
type Metrics struct {
	Restarts          int            `json:"restarts"`
	FirstReady        *time.Duration `json:"firstReady"`
	Unavailable       time.Duration  `json:"unavailable"`
	ReadyButUnable    time.Duration  `json:"readyButUnable"`
	Failed            map[Kind]int   `json:"failedProbes"`
	Timeouts          map[Kind]int   `json:"timeouts"`
	RestartsPerMinute float64        `json:"restartsPerMinute"`
	Availability      float64        `json:"availability"`
}
type Result struct {
	Config  Config  `json:"config"`
	Events  []Event `json:"events"`
	Spans   []Span  `json:"spans"`
	Metrics Metrics `json:"metrics"`
}
type scheduled struct {
	at                      time.Duration
	priority, seq, instance int
	typ                     string
	kind                    Kind
	success, timeout        bool
}
type queue []scheduled

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	if q[i].priority != q[j].priority {
		return q[i].priority < q[j].priority
	}
	return q[i].seq < q[j].seq
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(scheduled)) }
func (q *queue) Pop() any     { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

type streak struct{ successes, failures int }
type engine struct {
	c             Config
	r             Result
	q             queue
	seq, instance int
	now, started  time.Duration
	phase         string
	ready         bool
	streaks       map[Kind]streak
}

// Run uses no wall clock or randomness. Events at duration are excluded: [0,duration).
func Run(c Config) (Result, error) {
	if err := c.Validate(); err != nil {
		return Result{}, err
	}
	e := engine{c: c, phase: "stopped", r: Result{Config: c, Events: []Event{}, Spans: []Span{}, Metrics: Metrics{Failed: map[Kind]int{}, Timeouts: map[Kind]int{}}}}
	for _, k := range Kinds {
		e.r.Metrics.Failed[k] = 0
		e.r.Metrics.Timeouts[k] = 0
	}
	e.push(scheduled{typ: "start"})
	processed := 0
	for e.q.Len() > 0 {
		processed++
		if processed > 250000 {
			return Result{}, fmt.Errorf("simulation exceeds 250000 scheduled events; shorten duration or increase probe/restart intervals")
		}
		x := heap.Pop(&e.q).(scheduled)
		if x.at >= c.Duration {
			break
		}
		e.integrate(x.at)
		if x.typ != "start" && x.instance != e.instance {
			continue
		}
		switch x.typ {
		case "start":
			e.start()
		case "boundary": // integration evaluates application state at the next interval
		case "stop":
			e.phase = "waiting"
			e.log("container_stopped", "", "termination finished; restart delay begins")
			e.push(scheduled{at: e.now + c.RestartDelay, typ: "start"})
		case "probe":
			if e.phase == "running" {
				e.probe(x.kind)
			}
		case "result":
			if e.phase == "running" {
				e.complete(x)
			}
		}
	}
	e.integrate(c.Duration)
	e.r.Metrics.RestartsPerMinute = float64(e.r.Metrics.Restarts) / c.Duration.Minutes()
	return e.r, nil
}
func (e *engine) push(x scheduled) {
	e.seq++
	x.seq = e.seq
	x.instance = e.instance
	switch x.typ {
	case "start", "stop", "boundary":
		x.priority = 0
	case "result":
		x.priority = 1
	case "probe":
		x.priority = 2
	}
	if x.at < e.c.Duration {
		heap.Push(&e.q, x)
	}
}
func (e *engine) log(typ string, k Kind, detail string) {
	e.r.Events = append(e.r.Events, Event{At: e.now, Instance: e.instance, Type: typ, Probe: k, Detail: detail})
}
func (e *engine) setReady(ready bool, reason string) {
	if ready == e.ready {
		return
	}
	e.ready = ready
	typ := "unready"
	if ready {
		typ = "ready"
		if e.r.Metrics.FirstReady == nil {
			t := e.now
			e.r.Metrics.FirstReady = &t
		}
	}
	e.log(typ, Readiness, reason)
}
func (e *engine) start() {
	e.instance++
	if e.instance > 1 {
		e.r.Metrics.Restarts++
	}
	e.started = e.now
	e.phase = "running"
	e.ready = false
	e.streaks = map[Kind]streak{}
	e.log("container_started", "", "initialization and probe counters reset")
	e.push(scheduled{at: e.now + e.c.Application.StartupDuration, typ: "boundary"})
	for _, w := range e.c.Application.Windows {
		offset := time.Duration(0)
		if w.Clock == "container" {
			offset = e.started
		}
		for _, t := range []time.Duration{w.From + offset, w.To + offset} {
			if t >= e.now {
				e.push(scheduled{at: t, typ: "boundary"})
			}
		}
	}
	if _, ok := e.c.Probes[Startup]; ok {
		e.schedule(Startup, e.now)
	} else {
		e.unlock()
	}
}
func seconds(n int) time.Duration { return time.Duration(n) * time.Second }
func (e *engine) schedule(k Kind, base time.Duration) {
	if p, ok := e.c.Probes[k]; ok {
		e.push(scheduled{at: base + seconds(p.InitialDelaySeconds), typ: "probe", kind: k})
	}
}
func (e *engine) unlock() {
	e.schedule(Liveness, e.now)
	e.schedule(Readiness, e.now)
	if _, ok := e.c.Probes[Readiness]; !ok {
		e.setReady(true, "no readiness probe; startup gate open")
	}
}
func (e *engine) probe(k Kind) {
	p := e.c.Probes[k]
	_, _, r := e.c.Application.observe(e.now, e.now-e.started, k)
	duration := r.Latency
	timeout := duration > seconds(p.TimeoutSeconds)
	if timeout {
		duration = seconds(p.TimeoutSeconds)
	}
	e.log("probe_started", k, fmt.Sprintf("response latency %s; timeout %ds", r.Latency, p.TimeoutSeconds))
	e.push(scheduled{at: e.now + duration, typ: "result", kind: k, success: r.Success && !timeout, timeout: timeout})
	// Per-probe executions never overlap. Missed periodic slots are skipped.
	period := seconds(p.PeriodSeconds)
	next := e.now + period
	if duration >= period {
		next = e.now + (duration/period+1)*period
	}
	e.push(scheduled{at: next, typ: "probe", kind: k})
}
func (e *engine) complete(x scheduled) {
	k := x.kind
	p := e.c.Probes[k]
	s := e.streaks[k]
	if x.success {
		s.successes++
		s.failures = 0
	} else {
		s.failures++
		s.successes = 0
		e.r.Metrics.Failed[k]++
	}
	detail := fmt.Sprintf("success; consecutive successes %d/%d", s.successes, p.SuccessThreshold)
	if !x.success {
		detail = fmt.Sprintf("failure; consecutive failures %d/%d", s.failures, p.FailureThreshold)
	}
	if x.timeout {
		e.r.Metrics.Timeouts[k]++
		detail = "timeout; " + detail
	}
	e.log("probe_result", k, detail)
	e.streaks[k] = s
	if k == Startup && x.success {
		e.log("startup_succeeded", k, "startup checks disabled; other probe initial delays begin")
		// Remove all future startup events, including the already scheduled next tick.
		filtered := e.q[:0]
		for _, item := range e.q {
			if item.kind != Startup {
				filtered = append(filtered, item)
			}
		}
		e.q = filtered
		heap.Init(&e.q)
		e.unlock()
		return
	}
	if k == Readiness {
		if s.successes >= p.SuccessThreshold {
			e.setReady(true, "readiness success threshold reached")
		}
		if s.failures >= p.FailureThreshold {
			e.setReady(false, "readiness failure threshold reached")
		}
	} else if s.failures >= p.FailureThreshold {
		e.phase = "terminating"
		e.setReady(false, string(k)+" failure threshold reached")
		e.log("termination_started", k, fmt.Sprintf("%d consecutive failures; restart requested", s.failures))
		e.push(scheduled{at: e.now + e.c.TerminationDelay, typ: "stop"})
	}
}
func (e *engine) integrate(to time.Duration) {
	if to <= e.now {
		return
	}
	state, serve := "stopped", false
	if e.phase == "running" {
		state, serve, _ = e.c.Application.observe(e.now, e.now-e.started, Readiness)
	} else if e.phase == "terminating" {
		state = "terminating"
	} else if e.phase == "waiting" {
		state = "restart wait"
	}
	span := Span{Phase: e.phase, From: e.now, To: to, Instance: e.instance, State: state, Ready: e.ready, CanServe: serve}
	n := len(e.r.Spans)
	if n > 0 && e.r.Spans[n-1].To == span.From && e.r.Spans[n-1].Phase == span.Phase && e.r.Spans[n-1].Instance == span.Instance && e.r.Spans[n-1].State == state && e.r.Spans[n-1].Ready == span.Ready && e.r.Spans[n-1].CanServe == serve {
		e.r.Spans[n-1].To = to
	} else {
		e.r.Spans = append(e.r.Spans, span)
	}
	d := to - e.now
	if !e.ready {
		e.r.Metrics.Unavailable += d
	}
	if e.ready && !serve {
		e.r.Metrics.ReadyButUnable += d
	}
	if e.ready && serve {
		e.r.Metrics.Availability += float64(d) / float64(e.c.Duration)
	}
	e.now = to
}
