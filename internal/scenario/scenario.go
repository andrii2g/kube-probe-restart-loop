// Package scenario loads the simulator's own YAML format, not Kubernetes manifests.
package scenario

import (
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/andrii2g/kube-probe-restart-loop/internal/sim"
	"go.yaml.in/yaml/v3"
)

type probeFields struct {
	InitialDelay *int `yaml:"initialDelaySeconds"`
	Period       *int `yaml:"periodSeconds"`
	Timeout      *int `yaml:"timeoutSeconds"`
	Failure      *int `yaml:"failureThreshold"`
	Success      *int `yaml:"successThreshold"`
}

func (p probeFields) resolved() sim.Probe {
	r := sim.DefaultProbe()
	if p.InitialDelay != nil {
		r.InitialDelaySeconds = *p.InitialDelay
	}
	if p.Period != nil {
		r.PeriodSeconds = *p.Period
	}
	if p.Timeout != nil {
		r.TimeoutSeconds = *p.Timeout
	}
	if p.Failure != nil {
		r.FailureThreshold = *p.Failure
	}
	if p.Success != nil {
		r.SuccessThreshold = *p.Success
	}
	return r
}

type variant struct {
	Name   string                   `yaml:"name"`
	Probes map[sim.Kind]probeFields `yaml:"probes"`
}
type document struct {
	Name             string          `yaml:"name"`
	Duration         time.Duration   `yaml:"duration"`
	TerminationDelay time.Duration   `yaml:"terminationDelay"`
	RestartDelay     time.Duration   `yaml:"restartDelay"`
	Application      sim.Application `yaml:"application"`
	Variants         []variant       `yaml:"variants"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Load rejects unknown fields, duplicate keys, extra documents, and unsafe variant names.
func Load(r io.Reader) ([]sim.Config, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	d := document{RestartDelay: time.Second}
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("decode scenario: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected exactly one YAML document")
	}
	if d.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(d.Variants) == 0 || len(d.Variants) > 32 {
		return nil, fmt.Errorf("require 1 to 32 variants")
	}
	seen := map[string]bool{}
	configs := make([]sim.Config, 0, len(d.Variants))
	for _, v := range d.Variants {
		if !safeName.MatchString(v.Name) || seen[v.Name] {
			return nil, fmt.Errorf("variant names must be unique safe identifiers: %q", v.Name)
		}
		seen[v.Name] = true
		c := sim.Config{Name: v.Name, Duration: d.Duration, TerminationDelay: d.TerminationDelay, RestartDelay: d.RestartDelay, Application: d.Application, Probes: map[sim.Kind]sim.Probe{}}
		for k, p := range v.Probes {
			c.Probes[k] = p.resolved()
		}
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("variant %s: %w", v.Name, err)
		}
		configs = append(configs, c)
	}
	return configs, nil
}
