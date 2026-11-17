package config

import (
	"strings"
	"testing"
)

const valid = `
version: 1
currencies: {USDT: 6}
ledger:
  sql:
    dsn_env: LEDGER_DSN
    query: select psp_reference, amount_minor, currency from payments
  columns:
    key: psp_reference
    amount: amount_minor
    currency: currency
provider:
  csv:
    path: exports/stripe_*.csv
    profile: stripe_balance
  columns:
    key: payment_intent_id
  filter:
    - column: reporting_category
      in: [charge, refund]
match:
  group: true
  tolerance: {amount: 1}
report:
  html: out/report.html
  json: out/report.json
`

func TestParseValid(t *testing.T) {
	c, err := Parse([]byte(strings.Replace(valid, "    currency: currency\n", "    currency: currency\n    fee: fee_minor\n", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if c.Ledger.Amounts != "minor" || c.Provider.Amounts != "major" {
		t.Errorf("amount units: ledger %q, provider %q", c.Ledger.Amounts, c.Provider.Amounts)
	}
	p := c.Provider.Columns
	if p.Amount != "gross" || p.Fee != "fee" || p.Currency != "currency" || len(p.Key) != 1 || p.Key[0] != "payment_intent_id" {
		t.Errorf("stripe_balance profile not applied: %+v", p)
	}
	if c.Provider.CSV.Delimiter != "," || c.Provider.CSV.Decimal != "." {
		t.Errorf("csv defaults: %+v", c.Provider.CSV)
	}
	if !c.Match.Group || c.Match.Tolerance.Amount != 1 || c.Report.FailOn != "any" || c.Currencies["USDT"] != 6 {
		t.Errorf("match or report: %+v %+v", c.Match, c.Report)
	}
}

func TestKeyAsList(t *testing.T) {
	in := strings.Replace(valid, "key: psp_reference", "key: [account_id, psp_reference]", 1)
	in = strings.Replace(in, "key: payment_intent_id", "key: [source_id, payment_intent_id]", 1)
	in = strings.Replace(in, "    currency: currency\n", "    currency: currency\n    fee: fee_minor\n", 1)
	c, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Ledger.Columns.Key) != 2 || c.Ledger.Columns.Key[1] != "psp_reference" {
		t.Errorf("key list: %v", c.Ledger.Columns.Key)
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name, from, to, want string
	}{
		{"unknown key", "match:\n", "macth:\n", "field macth not found"},
		{"version", "version: 1", "version: 2", "version: want 1"},
		{"both sources", "  sql:\n", "  csv: {path: x.csv}\n  sql:\n", "set one of sql, csv, ianfoxdev_ledger"},
		{"no dsn env", "    dsn_env: LEDGER_DSN\n", "", "dsn_env"},
		{"unknown profile", "profile: stripe_balance", "profile: stripe", `unknown "stripe", known: stripe_balance`},
		{"amount units", "  columns:\n    key: psp_reference", "  amounts: cents\n  columns:\n    key: psp_reference", "want major or minor"},
		{"no report", "report:\n  html: out/report.html\n  json: out/report.json\n", "report: {}\n", "set at least one of html, json, csv"},
		{"fee on one side", "", "", "columns.fee: set it on both sides"},
		{"negative tolerance", "{amount: 1}", "{amount: -1}", "must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(strings.Replace(valid, tt.from, tt.to, 1)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// The same broken file gives the same error text, in the same order, every time.
func TestErrorsAreStable(t *testing.T) {
	in := strings.Replace(valid, "profile: stripe_balance", "profile: stripe_balance\n    delimiter: ';;'\n    decimal: ''", 1)
	first := ""
	for i := 0; i < 20; i++ {
		_, err := Parse([]byte(in))
		if err == nil {
			t.Fatal("accepted")
		}
		if i == 0 {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("errors changed between runs:\n%s\n---\n%s", first, err.Error())
		}
	}
}

const ledgerSource = `
version: 1
ledger:
  ianfoxdev_ledger: {dsn_env: LEDGER_DSN, account: "cash:psp", key: payment}
provider:
  csv: {path: balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
report: {json: out.json}
`

func TestLedgerSourceFillsColumns(t *testing.T) {
	c, err := Parse([]byte(ledgerSource))
	if err != nil {
		t.Fatal(err)
	}
	l := c.Ledger.Columns
	if c.Ledger.Amounts != "minor" || l.Amount != "amount" || l.Currency != "currency" || len(l.Key) != 1 || l.Key[0] != "key" {
		t.Errorf("columns %+v, amounts %q", l, c.Ledger.Amounts)
	}
	// The Stripe profile compares fees, so the ledger side gets a fee column.
	if l.Fee != "fee" {
		t.Errorf("fee column %q", l.Fee)
	}

	withFee := strings.Replace(ledgerSource, "key: payment}", `key: payment, fee_account: "fees:psp"}`, 1)
	withFee = strings.Replace(withFee, "{path: balance.csv, profile: stripe_balance}\n  columns: {key: payment_intent_id}",
		"{path: bank.csv}\n  columns: {key: ref, amount: amount, currency: ccy, fee: fee}", 1)
	if c, err = Parse([]byte(withFee)); err != nil || c.Ledger.Columns.Fee != "fee" {
		t.Errorf("fee account: %v %+v", err, c.Ledger.Columns)
	}

	generic := strings.Replace(ledgerSource, "{path: balance.csv, profile: stripe_balance}\n  columns: {key: payment_intent_id}",
		"{path: bank.csv}\n  columns: {key: ref, amount: amount, currency: ccy}", 1)
	c, err = Parse([]byte(generic))
	if err != nil {
		t.Fatal(err)
	}
	if c.Ledger.Columns.Fee != "" {
		t.Errorf("no fees on the other side, fee column %q", c.Ledger.Columns.Fee)
	}
}

func TestLedgerSourceRejects(t *testing.T) {
	tests := []struct {
		name, from, to, want string
	}{
		{"no account", `account: "cash:psp", `, "", "ianfoxdev_ledger.account"},
		{"no key", ", key: payment}", "}", "ianfoxdev_ledger.key"},
		{"no dsn env", "dsn_env: LEDGER_DSN, ", "", "ianfoxdev_ledger.dsn_env"},
		{"columns", "key: payment}\n", "key: payment}\n  columns: {key: id, amount: a, currency: c}\n", "sets the columns itself"},
		{"amounts", "key: payment}\n", "key: payment}\n  amounts: major\n", "reads minor units"},
		{"fee account is the account", `key: payment}`, `key: payment, fee_account: "cash:psp"}`, "must differ from account"},
		{"with sql", "key: payment}\n", "key: payment}\n  sql: {dsn_env: X, query: select 1}\n", "set one of sql, csv, ianfoxdev_ledger"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.Replace(ledgerSource, tt.from, tt.to, 1)
			if in == ledgerSource {
				t.Fatal("replacement did not apply")
			}
			_, err := Parse([]byte(in))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
