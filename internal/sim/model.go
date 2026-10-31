// Package sim implements a deterministic, deliberately bounded probe simulator.
package sim

import (
	"fmt"
	"time"
)

type Kind string

const (
	Startup   Kind = "startup"
	Liveness  Kind = "liveness"
	Readiness Kind = "readiness"
)

var Kinds = []Kind{Startup, Liveness, Readiness}

type Probe struct {
	InitialDelaySeconds int `yaml:"initialDelaySeconds" json:"initialDelaySeconds"`
	PeriodSeconds       int `yaml:"periodSeconds" json:"periodSeconds"`
	TimeoutSeconds      int `yaml:"timeoutSeconds" json:"timeoutSeconds"`
	FailureThreshold    int `yaml:"failureThreshold" json:"failureThreshold"`
	SuccessThreshold    int `yaml:"successThreshold" json:"successThreshold"`
}

// DefaultProbe supplies Kubernetes defaults. Scenario decoding overlays explicit fields.
func DefaultProbe() Probe {
	return Probe{PeriodSeconds: 10, TimeoutSeconds: 1, FailureThreshold: 3, SuccessThreshold: 1}
}
func (p Probe) Validate(k Kind) error {
	if p.InitialDelaySeconds < 0 || p.PeriodSeconds < 1 || p.TimeoutSeconds < 1 || p.FailureThreshold < 1 || p.SuccessThreshold < 1 {
		return fmt.Errorf("%s: invalid probe field range", k)
	}
	if k != Readiness && p.SuccessThreshold != 1 {
		return fmt.Errorf("%s: successThreshold must be 1", k)
	}
	// Bound input arithmetic and event volume for this small utility.
	if p.InitialDelaySeconds > 86400 || p.PeriodSeconds > 86400 || p.TimeoutSeconds > 86400 || p.FailureThreshold > 100000 || p.SuccessThreshold > 100000 {
		return fmt.Errorf("%s: probe field exceeds simulator limits", k)
	}
	return nil
}

type Response struct {
	Success bool          `yaml:"success" json:"success"`
	Latency time.Duration `yaml:"latency" json:"latency"`
}
type Window struct {
	From      time.Duration     `yaml:"from" json:"from"`
	To        time.Duration     `yaml:"to" json:"to"`
	Clock     string            `yaml:"clock" json:"clock"` // scenario or container
	State     string            `yaml:"state" json:"state"`
	CanServe  *bool             `yaml:"canServe" json:"canServe,omitempty"`
	Responses map[Kind]Response `yaml:"responses" json:"responses,omitempty"`
}
type Application struct {
	StartupDuration time.Duration `yaml:"startupDuration" json:"startupDuration"`
	ResponseLatency time.Duration `yaml:"responseLatency" json:"responseLatency"`
	Windows         []Window      `yaml:"windows" json:"windows"`
}
type Config struct {
	Name             string         `json:"name"`
	Duration         time.Duration  `json:"duration"`
	TerminationDelay time.Duration  `json:"terminationDelay"`
	RestartDelay     time.Duration  `json:"restartDelay"`
	Application      Application    `json:"application"`
	Probes           map[Kind]Probe `json:"probes"`
}

func (c Config) Validate() error {
	if c.Duration <= 0 || c.Duration > 24*time.Hour {
		return fmt.Errorf("duration must be in (0, 24h]")
	}
	if c.TerminationDelay < 0 || c.RestartDelay < time.Millisecond || c.TerminationDelay > 24*time.Hour || c.RestartDelay > 24*time.Hour {
		return fmt.Errorf("termination delay must be in [0, 24h]; restart delay in [1ms, 24h]")
	}
	a := c.Application
	if a.StartupDuration < 0 || a.StartupDuration > 24*time.Hour || a.ResponseLatency < 0 || a.ResponseLatency > 24*time.Hour {
		return fmt.Errorf("application durations must be in [0, 24h]")
	}
	for k, p := range c.Probes {
		if k != Startup && k != Liveness && k != Readiness {
			return fmt.Errorf("unknown probe %q", k)
		}
		if err := p.Validate(k); err != nil {
			return err
		}
	}
	for i, w := range a.Windows {
		if w.From < 0 || w.To <= w.From || w.To > 24*time.Hour {
			return fmt.Errorf("window %d: require 0 <= from < to <= 24h", i)
		}
		if w.Clock != "scenario" && w.Clock != "container" {
			return fmt.Errorf("window %d: clock must be scenario or container", i)
		}
		for k, r := range w.Responses {
			if k != Startup && k != Liveness && k != Readiness {
				return fmt.Errorf("window %d: unknown response %q", i, k)
			}
			if r.Latency < 0 || r.Latency > 24*time.Hour {
				return fmt.Errorf("window %d: invalid latency", i)
			}
		}
	}
	return nil
}

func (a Application) observe(now, age time.Duration, k Kind) (string, bool, Response) {
	initialized := age >= a.StartupDuration
	state, serve := "initializing", false
	if initialized {
		state, serve = "serving", true
	}
	response := Response{Success: initialized, Latency: a.ResponseLatency}
	for _, w := range a.Windows {
		t := now
		if w.Clock == "container" {
			t = age
		}
		if t < w.From || t >= w.To {
			continue
		}
		if w.State != "" {
			state = w.State
		}
		if w.CanServe != nil {
			serve = initialized && *w.CanServe
		}
		if r, ok := w.Responses[k]; ok {
			response = r
		}
	}
	return state, serve, response
}
