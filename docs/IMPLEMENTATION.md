# Implementation steps

Each completed step is committed separately. This file records scope and validation;
the final behavior and limitations are described in the README and simulation contract.

1. **Simulation core:** virtual clock, probe state machines, lifecycle, event log,
   interval metrics, and behavior tests. Validation: `go test ./internal/sim`.
2. **Scenario CLI:** strict YAML decoding, shared application/comparison variants,
   six bundled examples, text/JSON output, and CLI tests. Validation: `go test ./...`;
   slow-start comparison yields 10 restarts and never ready without startup protection,
   versus zero restarts, first readiness at 30s, and 75% availability with protection.
3. **Reports and delivery:** SVG timelines, comparison reports, full usage and
   simulation contract, CI, and end-to-end verification.
