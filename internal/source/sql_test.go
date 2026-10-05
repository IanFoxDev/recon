package source

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Runs against PostgreSQL when RECON_TEST_PG_DSN is set (make postgres-up sets it up).
func TestSQL(t *testing.T) {
	dsn := os.Getenv("RECON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("RECON_TEST_PG_DSN not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, `
		drop table if exists recon_payments;
		create table recon_payments (psp_reference text, amount_minor bigint, amount numeric(12,2), fee_minor int, currency char(3));
		insert into recon_payments values
			('pi_001', 1099, 10.99, 62, 'usd'),
			('pi_002', 100, 100, 0, 'ISK'),
			('pi_003', 9007199254740993, 1.10, 0, 'EUR');`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RECON_LEDGER_DSN", dsn)

	run := func(columns, amounts string) []Row {
		t.Helper()
		c := load(t, `
version: 1
ledger:
  sql: {dsn_env: RECON_LEDGER_DSN, query: "select * from recon_payments order by psp_reference"}
  amounts: `+amounts+`
  columns: {key: psp_reference, `+columns+`, currency: currency, fee: fee_minor}
provider:
  csv: {path: stripe_balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
report: {json: out.json}
`)
		rows, inputs, err := Read(ctx, c, c.Ledger, currencies(t))
		if err != nil {
			t.Fatal(err)
		}
		if len(inputs) != 1 || inputs[0].Rows != 3 {
			t.Fatalf("inputs %+v", inputs)
		}
		return rows
	}

	minor := run("amount: amount_minor", "minor")
	// 9007199254740993 is 2^53+1: a float64 would turn it into ...992.
	if minor[0].Amount != 1099 || minor[0].Fee != 62 || minor[1].Amount != 100 || minor[2].Amount != 9007199254740993 || minor[0].Currency != "USD" {
		t.Fatalf("minor units: %+v", minor)
	}

	major := run("amount: amount", "major")
	if major[0].Amount != 1099 || major[1].Amount != 100 || major[2].Amount != 110 {
		t.Fatalf("numeric as text in major units: %+v", major)
	}
}

func TestSQLWithoutDSN(t *testing.T) {
	t.Setenv("RECON_EMPTY_DSN", "")
	c := load(t, `
version: 1
ledger: {sql: {dsn_env: RECON_EMPTY_DSN, query: "select 1"}, columns: {key: id, amount: a, currency: c, fee: f}}
provider: {csv: {path: stripe_balance.csv, profile: stripe_balance}, columns: {key: payment_intent_id}}
report: {json: out.json}
`)
	_, _, err := Read(context.Background(), c, c.Ledger, currencies(t))
	if err == nil || !strings.Contains(err.Error(), "RECON_EMPTY_DSN is empty") {
		t.Fatalf("error %v", err)
	}
}
