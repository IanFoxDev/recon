package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A ledger kept with ianfoxdev/ledger against a Stripe Balance report, end to end. One
// capture had its fee taken at once (cash got the net), another had it booked later;
// both reconcile with the gross and the fee. The provider has a payment whose callback
// was lost, the ledger has an opening balance the provider never saw.
func TestIanfoxdevLedgerAgainstStripe(t *testing.T) {
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
	schema, err := os.ReadFile("../../internal/source/testdata/ianfoxdev_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"drop schema if exists recon_e2e_ledger cascade",
		"create schema recon_e2e_ledger",
		"set search_path to recon_e2e_ledger",
		string(schema),
		`insert into ledger_accounts (code, type, currency, allow_negative) values
			('cash:psp', 'asset', 'USD', false), ('fees:psp', 'expense', 'USD', false),
			('user:1', 'liability', 'USD', false), ('opening', 'equity', 'USD', true)`,
		`insert into ledger_transactions (idempotency_key, hash, type, meta, created_at)
		overriding system value values
			('opening', repeat('0', 64), 'transaction', '{}', '2026-11-01 00:00:00'),
			('psp:pi_1:captured', repeat('1', 64), 'deposit', '{"payment":"pi_1"}', '2026-11-17 10:00:00'),
			('psp:re_1', repeat('2', 64), 'deposit.refund', '{"payment":"pi_1"}', '2026-11-17 11:00:00'),
			('psp:pi_2:captured', repeat('3', 64), 'deposit', '{"payment":"pi_2"}', '2026-11-17 12:00:00'),
			('psp:pi_2:fee', repeat('4', 64), 'fee', '{"payment":"pi_2"}', '2026-11-17 12:00:01')`,
		`insert into ledger_postings (account, sequence, transaction_id, position, side, amount, balance_after) values
			('cash:psp', 1, 1, 0, 'D', 10000, 10000), ('opening', 1, 1, 1, 'C', 10000, 10000),
			('cash:psp', 2, 2, 0, 'D', 2871, 12871), ('fees:psp', 1, 2, 1, 'D', 129, 129), ('user:1', 1, 2, 2, 'C', 3000, 3000),
			('cash:psp', 3, 3, 0, 'C', 1000, 11871), ('user:1', 2, 3, 1, 'D', 1000, 2000),
			('cash:psp', 4, 4, 0, 'D', 5000, 16871), ('user:1', 3, 4, 1, 'C', 5000, 7000),
			('cash:psp', 5, 5, 0, 'C', 175, 16696), ('fees:psp', 2, 5, 1, 'D', 175, 304)`,
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	t.Setenv("RECON_E2E_LEDGER_DSN", dsn+sep+"search_path=recon_e2e_ledger")

	dir := t.TempDir()
	files := map[string]string{
		"balance.csv": `balance_transaction_id,created,available_on,currency,gross,fee,net,reporting_category,source_id,payment_intent_id,charge_id
txn_1,2026-11-17 10:00:00,2026-11-19 00:00:00,usd,30.00,1.29,28.71,charge,ch_1,pi_1,ch_1
txn_2,2026-11-17 11:00:00,2026-11-19 00:00:00,usd,-10.00,0.00,-10.00,refund,re_1,pi_1,ch_1
txn_3,2026-11-17 12:00:00,2026-11-19 00:00:00,usd,50.00,1.75,48.25,charge,ch_2,pi_2,ch_2
txn_4,2026-11-17 13:00:00,2026-11-19 00:00:00,usd,20.00,0.88,19.12,charge,ch_3,pi_3,ch_3
`,
		"recon.yaml": `version: 1
ledger:
  ianfoxdev_ledger: {dsn_env: RECON_E2E_LEDGER_DSN, account: "cash:psp", key: payment, fee_account: "fees:psp"}
provider:
  csv: {path: balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
match: {group: true}
report: {json: report.json}
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	code, out, errOut := runRecon(t, "run", "-c", filepath.Join(dir, "recon.yaml"))
	want := "reconciled 2, differences 2, missing_in_ledger 1, missing_in_provider 1\n"
	if code != exitDifferences || out != want {
		t.Fatalf("code %d\nstdout %q\nwant %q\nstderr %s", code, out, want, errOut)
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"pi_3"`, `"opening"`} {
		if !strings.Contains(string(report), key) {
			t.Errorf("report.json does not name %s", key)
		}
	}
}
