package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/money"
)

func load(t *testing.T, yaml string) config.Config {
	t.Helper()
	c, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	c.Dir = "testdata"
	return c
}

const stripeConfig = `
version: 1
ledger:
  csv: {path: bank_eu.csv, delimiter: ";", decimal: ",", thousands: "."}
  columns: {key: reference, amount: amount, currency: currency, fee: amount}
provider:
  csv: {path: stripe_balance.csv, profile: stripe_balance}
  columns: {key: payment_intent_id}
  filter: [{column: reporting_category, in: [charge, refund]}]
report: {json: out.json}
`

func currencies(t *testing.T) money.Currencies {
	t.Helper()
	c, err := money.NewCurrencies(nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStripeBalanceReport(t *testing.T) {
	c := load(t, stripeConfig)
	rows, inputs, err := Read(context.Background(), c, c.Provider, currencies(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		key         string
		amount, fee int64
		currency    string
		line        int
	}{
		{"pi_001", 1099, 62, "USD", 2},
		{"pi_001", -1099, 0, "USD", 3},
		{"pi_002", 100, 0, "ISK", 4}, // "100.00" for a currency without minor units
	}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d (the payout must be filtered out): %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		r := rows[i]
		if r.Key[0] != w.key || r.Amount != w.amount || r.Fee != w.fee || r.Currency != w.currency || r.Origin.Line != w.line {
			t.Errorf("row %d = %+v, want %+v", i, r, w)
		}
	}
	if len(inputs) != 1 || inputs[0].Name != "stripe_balance.csv" || len(inputs[0].SHA256) != 64 || inputs[0].Rows != 3 {
		t.Errorf("inputs %+v", inputs)
	}
}

func TestEuropeanFormat(t *testing.T) {
	c := load(t, stripeConfig)
	rows, _, err := Read(context.Background(), c, c.Ledger, currencies(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Amount != 123450 || rows[1].Amount != 99 || rows[1].Currency != "EUR" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestErrorsNameThePlace(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("decimals.csv", "id,amount,currency\na,10.99,EUR\nb,10.999,EUR\n")
	write("currency.csv", "id,amount,currency\na,1,XYZ\n")
	write("columns.csv", "id,sum,currency\na,1,EUR\n")
	write("key.csv", "id,amount,currency\n,1,EUR\n")
	tests := map[string]string{
		"decimals.csv": "decimals.csv:3: column amount: invalid amount: \"10.999\" has 3 decimal places, the currency has 2",
		"currency.csv": "currency.csv:2: column currency: unknown currency \"XYZ\"",
		"columns.csv":  "columns.csv: column \"amount\" not found; columns are: id, sum, currency",
		"key.csv":      "key.csv:2: column id: empty key",
	}
	for file, want := range tests {
		c, err := config.Parse([]byte(`
version: 1
ledger: {csv: {path: ` + file + `}, columns: {key: id, amount: amount, currency: currency}}
provider: {csv: {path: ` + file + `}, columns: {key: id, amount: amount, currency: currency}}
report: {json: out.json}
`))
		if err != nil {
			t.Fatal(err)
		}
		c.Dir = dir
		_, _, err = Read(context.Background(), c, c.Ledger, currencies(t))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %v, want it to contain %q", file, err, want)
		}
	}
}
