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
// integer id, an opening balance without a payment, a capture the provider took a fee
// from, and the user's side of each.
const ledgerRows = `
insert into ledger_accounts (code, type, currency, allow_negative) values
	('cash:psp', 'asset', 'USD', false),
	('user:1', 'liability', 'USD', false),
	('opening', 'equity', 'USD', true),
	('fees:psp', 'expense', 'USD', false),
	('fees:eur', 'expense', 'EUR', false),
	('bank', 'asset', 'USD', false);
insert into ledger_transactions (idempotency_key, hash, type, meta, created_at)
overriding system value values
	('psp:pi_1:captured', repeat('a', 64), 'deposit', '{"payment":"pi_1","user":"1"}', '2026-11-17 10:00:00'),
	('psp:re_1', repeat('b', 64), 'deposit.refund', '{"payment":"pi_1","refund":"re_1"}', '2026-11-17 10:05:00'),
	('psp:7:captured', repeat('c', 64), 'deposit', '{"payment":7}', '2026-11-17 10:10:00'),
	('opening:2026', repeat('d', 64), 'transaction', '{}', '2026-11-17 09:00:00'),
	('psp:pi_2:captured', repeat('e', 64), 'deposit', '{"payment":"pi_2"}', '2026-11-17 10:15:00');
insert into ledger_postings (account, sequence, transaction_id, position, side, amount, balance_after) values
	('cash:psp', 1, 4, 0, 'D', 100000, 100000),
	('opening', 1, 4, 1, 'C', 100000, 100000),
	('cash:psp', 2, 1, 0, 'D', 5000, 105000),
	('user:1', 1, 1, 1, 'C', 5000, 5000),
	('cash:psp', 3, 2, 0, 'C', 1500, 103500),
	('user:1', 2, 2, 1, 'D', 1500, 3500),
	('cash:psp', 4, 3, 0, 'D', 999, 104499),
	('user:1', 3, 3, 1, 'C', 999, 4499),
	('cash:psp', 5, 5, 0, 'D', 2871, 107370),
	('fees:psp', 1, 5, 1, 'D', 129, 129),
	('user:1', 4, 5, 2, 'C', 3000, 7499);`

// ledgerDB loads the ledger's schema and rows into a schema of their own, so tests of
// other packages running at the same time do not touch them, and returns a DSN for it.
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
	for _, sql := range []string{
		"drop schema if exists recon_source_ledger cascade",
		"create schema recon_source_ledger",
		"set search_path to recon_source_ledger",
		string(schema),
		ledgerRows,
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return withSearchPath(dsn, "recon_source_ledger")
}

func withSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&search_path=" + schema
	}
	return dsn + "?search_path=" + schema
}

func TestLedger(t *testing.T) {
	t.Setenv("RECON_LEDGER_DSN", ledgerDB(t))
	read := func(account, fee string) ([]Row, error) {
		c := load(t, `
version: 1
ledger:
  ianfoxdev_ledger: {dsn_env: RECON_LEDGER_DSN, account: "`+account+`", key: payment, fee_account: "`+fee+`"}
provider:
  csv: {path: stripe_balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
report: {json: out.json}
`)
		rows, _, err := Read(context.Background(), c, c.Ledger, currencies(t))
		return rows, err
	}

	lines := func(rows []Row) string {
		got := make([]string, 0, len(rows))
		for _, r := range rows {
			got = append(got, strings.Join(r.Key, ",")+" "+itoa(r.Amount)+" "+itoa(r.Fee)+" "+r.Currency+" "+r.Origin.String())
		}
		return strings.Join(got, "\n")
	}
	check := func(account, fee string, want ...string) {
		t.Helper()
		rows, err := read(account, fee)
		if err != nil {
			t.Fatal(err)
		}
		if lines(rows) != strings.Join(want, "\n") {
			t.Fatalf("%s\ngot:\n%s\nwant:\n%s", account, lines(rows), strings.Join(want, "\n"))
		}
	}

	// In the order of the transactions; the opening balance has no payment.
	check("cash:psp", "",
		"pi_1 5000 0 USD ledger:cash:psp:1",
		"pi_1 -1500 0 USD ledger:cash:psp:2",
		"7 999 0 USD ledger:cash:psp:3",
		"opening:2026 100000 0 USD ledger:cash:psp:4",
		"pi_2 2871 0 USD ledger:cash:psp:5",
	)
	// With the fee account, the capture is the gross the provider reports and its fee.
	check("cash:psp", "fees:psp",
		"pi_1 5000 0 USD ledger:cash:psp:1",
		"pi_1 -1500 0 USD ledger:cash:psp:2",
		"7 999 0 USD ledger:cash:psp:3",
		"opening:2026 100000 0 USD ledger:cash:psp:4",
		"pi_2 3000 129 USD ledger:cash:psp:5",
	)

	// A liability is positive on the credit side: the deposit raises the wallet.
	check("user:1", "",
		"pi_1 5000 0 USD ledger:user:1:1",
		"pi_1 -1500 0 USD ledger:user:1:2",
		"7 999 0 USD ledger:user:1:3",
		"pi_2 3000 0 USD ledger:user:1:4",
	)
	check("bank", "")

	for _, tt := range []struct{ account, fee, want string }{
		{"cash:pps", "", `account "cash:pps" is not in ledger_accounts`},
		{"cash:psp", "fees", `fee account "fees" is not in ledger_accounts`},
		{"cash:psp", "opening", `fee account "opening" is equity, want expense`},
		{"cash:psp", "fees:eur", `fee account "fees:eur" is in EUR, account "cash:psp" in USD`},
	} {
		if _, err := read(tt.account, tt.fee); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s with fee account %q: %v, want %q", tt.account, tt.fee, err, tt.want)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
