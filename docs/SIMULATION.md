# Simulation contract

## Responsibilities

`internal/sim` owns application observations, independent probe streaks, container
lifecycle, the virtual event queue, and interval accounting. `internal/scenario`
owns strict YAML parsing and configuration defaults. `internal/report` projects
recorded results into text, JSON, and SVG. `cmd/kube-probe-restart-loop` coordinates
those parts and handles CLI failures with a nonzero exit code.

The engine has no filesystem, network, real clock, sleeping, or goroutines.
Integer `time.Duration` values determine timestamps. Rendering does not rerun the
simulation. Stable variant order, probe order (startup, liveness, readiness), and
event insertion order produce repeatable artifacts.

## Timing rules

- Observation covers `[0, duration)`. Events exactly at the final horizon do not run.
- Container start resets initialization and all probe streaks. Initial readiness
  is false. Without a readiness probe it becomes true when the startup gate opens,
  even if the application cannot actually serve.
- When configured, startup is the only active probe until it succeeds. It stops
  permanently for that container instance after success. Liveness and readiness
  initial delays begin at that success timestamp. Without startup, they begin at
  container start. This convention follows the v1.35 documentation baseline.
- The first probe starts at its gate timestamp plus `initialDelaySeconds`, including
  a possible immediate first attempt at zero delay. There is no worker jitter.
- Application windows and initialization are evaluated at probe start; the sampled
  response does not change while in flight. The result arrives after the configured
  latency or timeout, whichever is shorter. A latency strictly greater than timeout
  fails at the deadline. A response exactly at the deadline is allowed to succeed.
- Each probe has its own periodic schedule. Executions never overlap. If a response
  occupies one or more full periods, skip those slots and use the next strictly
  later slot. Readiness has no special accelerated retries.
- Consecutive successes reset failure streaks and vice versa. Readiness remains
  unready until its success threshold is reached, and remains ready until its failure
  threshold is reached. Startup/liveness success thresholds must be one.
- Startup/liveness failure thresholds begin termination, immediately withdraw
  readiness, and cancel further effective checks. After `terminationDelay`, the
  container stops; after `restartDelay`, a new instance starts. The application is
  treated as unable to serve throughout termination. This is a simplified lifecycle.
- Queued results and boundaries from old instances have no effect on new instances.
  Probe attempts canceled by termination or the horizon do not count as failures.

For equal timestamps, lifecycle/application boundaries run before completed results,
which run before new probe attempts. Ties within a class use insertion order.
Zero-latency results run before the next queued attempt. Thus a liveness termination
can prevent a readiness attempt scheduled for that same instant. Application windows
are half-open and initialization succeeds at its exact completion boundary.

Do not infer exact restart times solely from `failureThreshold × periodSeconds`:
first-attempt timing, initial delay, response duration, termination, and restart delay
all affect the result.

## Metrics and intervals

State intervals cover the entire observation window without gaps or overlaps.
Adjacent identical intervals are merged. Each records container phase, application
state label, readiness, actual serving ability, and container instance. Metrics
integrate these intervals; point transitions have zero duration.

`unavailable` measures time not ready. `readyButUnable` measures time ready while
unable to serve. Availability is the remaining fraction:

```text
availability = (duration - unavailable - readyButUnable) / duration
```

Floating-point availability may have normal rounding error. Readiness and serving
are independent, so unavailable duration is intentionally not identical to time
the application is unhealthy. A single Pod's readiness represents normal Service
traffic eligibility under the stated assumptions, not real EndpointSlice propagation.

## Bounds and exclusions

One scenario supports 1–32 variants and at most 128 application windows. Duration
and individual delays are limited to 24 hours. Probe counts and field ranges are
validated before execution. A variant exceeding 250,000 scheduled events fails
with an actionable error rather than running indefinitely. Restart delay is at
least 1ms to prevent infinitely many immediate restarts at a single timestamp.

The model supports successful, failed, and timed-out responses. Kubernetes `Unknown`
results and transport-specific behavior (HTTP, TCP, gRPC, exec) are outside the MVP.
No cluster is contacted and no Kubernetes objects are modified. Actual kubelet
timing, grace handling, backoff, readiness acceleration, and Service convergence
may differ; reports are demonstrations, not cluster behavior predictions.

Reference: [Kubernetes v1.35 probe configuration](https://v1-35.docs.kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/).
