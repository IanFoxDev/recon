package source

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The tables of ianfoxdev/ledger with the rows its PHP code would write: a deposit
// keyed by the provider's payment, a partial refund of it, a deposit whose meta has an
// integer id, an opening balance without a payment, and the user's side of each.
const ledgerRows = `
insert into ledger_accounts (code, type, currency, allow_negative) values
	('cash:psp', 'asset', 'USD', false),
	('user:1', 'liability', 'USD', false),
	('opening', 'equity', 'USD', true),
	('fees:psp', 'expense', 'USD', false);
insert into ledger_transactions (idempotency_key, hash, type, meta, created_at)
overriding system value values
	('psp:pi_1:captured', repeat('a', 64), 'deposit', '{"payment":"pi_1","user":"1"}', '2026-11-17 10:00:00'),
	('psp:re_1', repeat('b', 64), 'deposit.refund', '{"payment":"pi_1","refund":"re_1"}', '2026-11-17 10:05:00'),
	('psp:7:captured', repeat('c', 64), 'deposit', '{"payment":7}', '2026-11-17 10:10:00'),
	('opening:2026', repeat('d', 64), 'transaction', '{}', '2026-11-17 09:00:00');
insert into ledger_postings (account, sequence, transaction_id, position, side, amount, balance_after) values
	('cash:psp', 1, 4, 0, 'D', 100000, 100000),
	('opening', 1, 4, 1, 'C', 100000, 100000),
	('cash:psp', 2, 1, 0, 'D', 5000, 105000),
	('user:1', 1, 1, 1, 'C', 5000, 5000),
	('cash:psp', 3, 2, 0, 'C', 1500, 103500),
	('user:1', 2, 2, 1, 'D', 1500, 3500),
	('cash:psp', 4, 3, 0, 'D', 999, 104499),
	('user:1', 3, 3, 1, 'C', 999, 4499);`

func ledgerDB(t *testing.T) string {
	t.Helper()
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
	schema, err := os.ReadFile("testdata/ianfoxdev_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"drop table if exists ledger_postings, ledger_transactions, ledger_accounts", string(schema), ledgerRows} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return dsn
}

func TestLedger(t *testing.T) {
	t.Setenv("RECON_LEDGER_DSN", ledgerDB(t))
	read := func(account string) ([]Row, error) {
		c := load(t, `
version: 1
ledger:
  ianfoxdev_ledger: {dsn_env: RECON_LEDGER_DSN, account: "`+account+`", key: payment}
provider:
  csv: {path: stripe_balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
report: {json: out.json}
`)
		rows, _, err := Read(context.Background(), c, c.Ledger, currencies(t))
		return rows, err
	}

	cash, err := read("cash:psp")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(cash))
	for _, r := range cash {
		got = append(got, strings.Join(r.Key, ",")+" "+itoa(r.Amount)+" "+itoa(r.Fee)+" "+r.Currency+" "+r.Origin.String())
	}
	want := []string{
		"opening:2026 100000 0 USD ledger:cash:psp:1",
		"pi_1 5000 0 USD ledger:cash:psp:2",
		"pi_1 -1500 0 USD ledger:cash:psp:3",
		"7 999 0 USD ledger:cash:psp:4",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cash:psp\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A liability is positive on the credit side: the deposit raises the wallet.
	wallet, err := read("user:1")
	if err != nil {
		t.Fatal(err)
	}
	if len(wallet) != 3 || wallet[0].Amount != 5000 || wallet[1].Amount != -1500 {
		t.Fatalf("user:1 %+v", wallet)
	}

	empty, err := read("fees:psp")
	if err != nil || len(empty) != 0 {
		t.Fatalf("an account without postings: %+v %v", empty, err)
	}

	if _, err := read("cash:pps"); err == nil || !strings.Contains(err.Error(), `account "cash:pps" is not in ledger_accounts`) {
		t.Fatalf("misspelled account: %v", err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
