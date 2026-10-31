package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/andrii2g/kube-probe-restart-loop/internal/report"
	"github.com/andrii2g/kube-probe-restart-loop/internal/scenario"
	"github.com/andrii2g/kube-probe-restart-loop/internal/sim"
)

const usage = "usage: kube-probe-restart-loop <run|compare> SCENARIO.yaml [--variant NAME] [--out DIR]\n"

func execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stdout, usage)
		return nil
	}
	if len(args) < 2 || (args[0] != "run" && args[0] != "compare") {
		return fmt.Errorf("%s", usage)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "artifacts/run", "output directory")
	selected := fs.String("variant", "", "variant name (run only; default first)")
	if err := fs.Parse(args[2:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if args[0] == "compare" && *selected != "" {
		return fmt.Errorf("--variant applies only to run")
	}
	f, err := os.Open(args[1])
	if err != nil {
		return err
	}
	configs, err := scenario.Load(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if args[0] == "run" {
		found := false
		for _, c := range configs {
			if *selected == "" || c.Name == *selected {
				configs = []sim.Config{c}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown variant %q", *selected)
		}
	}
	results := make([]sim.Result, 0, len(configs))
	for _, c := range configs {
		r, err := sim.Run(c)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		results = append(results, r)
	}
	if err := report.Write(*out, results); err != nil {
		return err
	}
	fmt.Fprint(stdout, report.Summary(results))
	fmt.Fprintf(stdout, "\nReports: %s\n", *out)
	return nil
}
func main() {
	if err := execute(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
