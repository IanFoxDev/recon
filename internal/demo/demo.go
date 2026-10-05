// Package demo generates a ledger and a provider export with known differences, for
// end-to-end tests and the example. The same seed gives the same files.
package demo

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/ianfoxdev/recon/internal/match"
	"github.com/ianfoxdev/recon/internal/money"
)

// Data is what Generate produces.
type Data struct {
	// SQL creates the table recon_demo_payments and fills it.
	SQL string
	// CSV is a Stripe Balance report export (balance_change_from_activity.itemized).
	CSV []byte
	// Expected lists the keys planted in each category, sorted.
	Expected map[match.Category][]string
}

type payment struct {
	key      string
	currency string
	amount   int64 // minor units
	fee      int64
}

var currencies = []string{"USD", "EUR", "JPY"}

// Generate makes n payments that reconcile and `planted` differences of each
// category. The ledger has no duplicates of matching rows, so the run is with
// match.group false.
func Generate(seed uint64, n, planted int) Data {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	exps, _ := money.NewCurrencies(nil)
	next := 0
	newPayment := func() payment {
		next++
		amount := 1 + r.Int64N(50000) // minor units: up to 500.00 USD or 50000 JPY
		return payment{key: fmt.Sprintf("pi_%06d", next), currency: currencies[r.IntN(len(currencies))], amount: amount, fee: amount * 29 / 1000}
	}

	var ledger, provider []payment
	expected := map[match.Category][]string{}
	plant := func(c match.Category, key string) { expected[c] = append(expected[c], key) }

	for i := 0; i < n; i++ {
		p := newPayment()
		ledger = append(ledger, p)
		provider = append(provider, p)
	}
	for i := 0; i < planted; i++ {
		p := newPayment()
		provider = append(provider, p)
		plant(match.MissingInLedger, p.key)

		p = newPayment()
		ledger = append(ledger, p)
		plant(match.MissingInProvider, p.key)

		p = newPayment()
		ledger = append(ledger, p)
		q := p
		q.amount += 1 + r.Int64N(500)
		provider = append(provider, q)
		plant(match.AmountMismatch, p.key)

		p = newPayment()
		ledger = append(ledger, p)
		q = p
		q.fee++
		provider = append(provider, q)
		plant(match.FeeMismatch, p.key)

		p = newPayment()
		ledger = append(ledger, p)
		q = p
		q.currency = "USD"
		if p.currency == "USD" {
			q.currency = "EUR"
		}
		provider = append(provider, q)
		plant(match.CurrencyMismatch, p.key)

		p = newPayment()
		ledger = append(ledger, p, p) // booked twice
		provider = append(provider, p)
		plant(match.Duplicate, p.key)
	}
	for _, keys := range expected {
		sort.Strings(keys)
	}
	// Shuffle both sides: the order rows come in must not matter.
	r.Shuffle(len(ledger), func(i, j int) { ledger[i], ledger[j] = ledger[j], ledger[i] })
	r.Shuffle(len(provider), func(i, j int) { provider[i], provider[j] = provider[j], provider[i] })

	return Data{SQL: ledgerSQL(ledger), CSV: stripeCSV(provider, exps, r), Expected: expected}
}

func ledgerSQL(ps []payment) string {
	var b strings.Builder
	b.WriteString("drop table if exists recon_demo_payments;\n")
	b.WriteString("create table recon_demo_payments (\n  id bigserial primary key,\n  psp_reference text not null,\n  amount_minor bigint not null,\n  fee_minor bigint not null,\n  currency char(3) not null\n);\n")
	for _, p := range ps {
		fmt.Fprintf(&b, "insert into recon_demo_payments (psp_reference, amount_minor, fee_minor, currency) values ('%s', %d, %d, '%s');\n", p.key, p.amount, p.fee, p.currency)
	}
	return b.String()
}

func stripeCSV(ps []payment, exps money.Currencies, r *rand.Rand) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"balance_transaction_id", "created", "available_on", "currency", "gross", "fee", "net", "reporting_category", "source_id", "payment_intent_id", "charge_id"})
	for i, p := range ps {
		exp, _ := exps.Exponent(p.currency)
		_ = w.Write([]string{
			fmt.Sprintf("txn_%06d", i+1), "2026-10-01 09:00:00", "2026-10-03 00:00:00",
			strings.ToLower(p.currency),
			money.FormatMinor(p.amount, exp), money.FormatMinor(p.fee, exp), money.FormatMinor(p.amount-p.fee, exp),
			"charge", "ch_" + p.key[3:], p.key, "ch_" + p.key[3:],
		})
		if r.IntN(10) == 0 {
			// A payout between charges, which the filter must drop.
			_ = w.Write([]string{fmt.Sprintf("txn_po_%06d", i+1), "2026-10-02 08:00:00", "2026-10-04 00:00:00", "usd", "-100.00", "0.00", "-100.00", "payout", fmt.Sprintf("po_%06d", i+1), "", ""})
		}
	}
	w.Flush()
	return buf.Bytes()
}
