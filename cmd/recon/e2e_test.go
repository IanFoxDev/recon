package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/recon/internal/demo"
	"github.com/ianfoxdev/recon/internal/match"
)

// A generated ledger in PostgreSQL against a generated Stripe export: recon must find
// exactly the planted differences, no more and no fewer. Needs RECON_TEST_PG_DSN.
func TestEndToEndOnPostgres(t *testing.T) {
	dsn := os.Getenv("RECON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("RECON_TEST_PG_DSN not set")
	}
	for _, seed := range []uint64{1, 2, 3} {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			const payments = 2000
			d := demo.Generate(seed, payments, 3)

			ctx := context.Background()
			conn, err := pgx.Connect(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close(ctx) }()
			if _, err := conn.Exec(ctx, d.SQL); err != nil {
				t.Fatal(err)
			}

			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "exports"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "exports", "stripe_balance.csv"), d.CSV, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := `version: 1
ledger:
  sql:
    dsn_env: RECON_TEST_PG_DSN
    query: select psp_reference, amount_minor, fee_minor, currency from recon_demo_payments
  columns: {key: psp_reference, amount: amount_minor, fee: fee_minor, currency: currency}
provider:
  csv: {path: exports/stripe_balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
  filter: [{column: reporting_category, in: [charge]}]
report: {json: report.json}
`
			if err := os.WriteFile(filepath.Join(dir, "recon.yaml"), []byte(cfg), 0o600); err != nil {
				t.Fatal(err)
			}

			code, out, errOut := runRecon(t, "run", "-c", filepath.Join(dir, "recon.yaml"))
			if code != exitDifferences {
				t.Fatalf("code %d, stdout %s, stderr %s", code, out, errOut)
			}
			var rep struct {
				Matched     int
				Differences []struct {
					Category match.Category
					Key      []string
				}
			}
			b, err := os.ReadFile(filepath.Join(dir, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &rep); err != nil {
				t.Fatal(err)
			}
			found := map[match.Category][]string{}
			for _, diff := range rep.Differences {
				found[diff.Category] = append(found[diff.Category], diff.Key[0])
			}
			for _, keys := range found {
				sort.Strings(keys)
			}
			if !reflect.DeepEqual(found, d.Expected) {
				t.Fatalf("found\n%v\nplanted\n%v", found, d.Expected)
			}
			if rep.Matched != payments {
				t.Fatalf("matched %d, want %d", rep.Matched, payments)
			}
		})
	}
}
