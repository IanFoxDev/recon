// Command recon reconciles an application's ledger with a payment provider's export.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/match"
	"github.com/ianfoxdev/recon/internal/money"
	"github.com/ianfoxdev/recon/internal/report"
	"github.com/ianfoxdev/recon/internal/source"
)

// version is set at build time with -ldflags "-X main.version=v0.1.0".
var version = "dev"

// Exit codes.
const (
	exitOK          = 0
	exitDifferences = 1
	exitError       = 2
)

const usage = `usage:
  recon run -c recon.yaml [--stamp]   compare the ledger with the provider export
  recon version

exit codes: 0 everything reconciles, 1 differences found, 2 the run failed`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, time.Now)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, now func() time.Time) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitError
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version)
		return exitOK
	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		fs.SetOutput(stderr)
		path := fs.String("c", "recon.yaml", "configuration file")
		stamp := fs.Bool("stamp", false, "write the time of the run into the report")
		if err := fs.Parse(args[1:]); err != nil {
			return exitError
		}
		generatedAt := ""
		if *stamp {
			generatedAt = now().UTC().Format(time.RFC3339)
		}
		total, err := reconcile(ctx, *path, generatedAt, stdout)
		if err != nil {
			fmt.Fprintln(stderr, "recon:", err)
			return exitError
		}
		return total
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s\n", args[0], usage)
		return exitError
	}
}

// reconcile does one run and returns the exit code.
func reconcile(ctx context.Context, path, generatedAt string, stdout io.Writer) (int, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return 0, err
	}
	currencies, err := money.NewCurrencies(cfg.Currencies)
	if err != nil {
		return 0, err
	}
	ledger, ledgerInputs, err := source.Read(ctx, cfg, cfg.Ledger, currencies)
	if err != nil {
		return 0, fmt.Errorf("ledger: %w", err)
	}
	provider, providerInputs, err := source.Read(ctx, cfg, cfg.Provider, currencies)
	if err != nil {
		return 0, fmt.Errorf("provider: %w", err)
	}
	withFee := cfg.Ledger.Columns.Fee != ""
	res := match.Run(ledger, provider, cfg.Match, withFee)
	rep := report.Build(version, generatedAt, report.Inputs{Ledger: ledgerInputs, Provider: providerInputs}, res, currencies, withFee)

	outputs := []struct {
		path   string
		render func() ([]byte, error)
	}{
		{cfg.Report.JSON, rep.JSON},
		{cfg.Report.CSV, rep.CSV},
		{cfg.Report.HTML, rep.HTML},
	}
	for _, o := range outputs {
		if o.path == "" {
			continue
		}
		data, err := o.render()
		if err != nil {
			return 0, err
		}
		p := o.path
		if !filepath.IsAbs(p) {
			p = filepath.Join(cfg.Dir, p)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return 0, err
		}
	}

	fmt.Fprintf(stdout, "reconciled %d, differences %d", res.Matched, rep.Total())
	for _, c := range rep.Counts {
		if c.Count > 0 {
			fmt.Fprintf(stdout, ", %s %d", c.Category, c.Count)
		}
	}
	fmt.Fprintln(stdout)
	if rep.Total() > 0 && cfg.Report.FailOn == "any" {
		return exitDifferences, nil
	}
	return exitOK, nil
}
