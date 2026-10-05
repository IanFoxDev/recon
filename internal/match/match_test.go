package match

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/source"
)

func row(key, currency string, amount, fee int64, line int) source.Row {
	return source.Row{Key: []string{key}, Currency: currency, Amount: amount, Fee: fee, Origin: source.Origin{File: "f.csv", Line: line}}
}

func categories(r Result) map[string][]Category {
	m := map[string][]Category{}
	for _, d := range r.Differences {
		m[d.Key[0]] = append(m[d.Key[0]], d.Category)
	}
	return m
}

func TestCategories(t *testing.T) {
	ledger := []source.Row{
		row("ok", "EUR", 1000, 30, 2),
		row("only-ledger", "EUR", 500, 0, 3),
		row("amount", "EUR", 1000, 30, 4),
		row("fee", "EUR", 1000, 30, 5),
		row("both", "EUR", 1000, 30, 6),
		row("currency", "EUR", 1000, 0, 7),
		row("dup", "EUR", 700, 0, 8),
		row("dup", "EUR", 700, 0, 9),
		row("within", "EUR", 1000, 30, 10),
	}
	provider := []source.Row{
		row("ok", "EUR", 1000, 30, 2),
		row("only-provider", "EUR", 250, 10, 3),
		row("amount", "EUR", 999, 30, 4),
		row("fee", "EUR", 1000, 31, 5),
		row("both", "EUR", 900, 40, 6),
		row("currency", "USD", 1000, 0, 7),
		row("dup", "EUR", 700, 0, 8),
		row("within", "EUR", 1001, 30, 9),
	}
	got := categories(Run(ledger, provider, config.Match{Tolerance: config.Tolerance{Amount: 1}}, true))
	want := map[string][]Category{
		"only-ledger":   {MissingInProvider},
		"only-provider": {MissingInLedger},
		"both":          {AmountMismatch, FeeMismatch},
		"fee":           {FeeMismatch},
		"currency":      {CurrencyMismatch},
		"dup":           {Duplicate},
	}
	// "amount" differs by 1 and "within" by 1: inside the tolerance of 1.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	res := Run(ledger, provider, config.Match{Tolerance: config.Tolerance{Amount: 1}}, true)
	if res.Matched != 3 { // ok, amount, within
		t.Errorf("matched %d", res.Matched)
	}
	for _, d := range res.Differences {
		if d.Category == AmountMismatch && d.Delta != -100 {
			t.Errorf("delta %d, want provider minus ledger = -100", d.Delta)
		}
	}
}

func TestExactByDefault(t *testing.T) {
	res := Run([]source.Row{row("a", "EUR", 1000, 0, 2)}, []source.Row{row("a", "EUR", 1001, 0, 2)}, config.Match{}, false)
	if len(res.Differences) != 1 || res.Differences[0].Category != AmountMismatch {
		t.Fatalf("%+v", res)
	}
}

func TestGroupSumsRowsOfOneKey(t *testing.T) {
	// One charge at the provider, booked as two entries in the ledger.
	ledger := []source.Row{row("pi_1", "EUR", 600, 0, 2), row("pi_1", "EUR", 400, 0, 3)}
	provider := []source.Row{row("pi_1", "EUR", 1000, 0, 2)}
	if res := Run(ledger, provider, config.Match{Group: true}, false); res.Matched != 1 || len(res.Differences) != 0 {
		t.Fatalf("grouped: %+v", res)
	}
	if res := Run(ledger, provider, config.Match{}, false); len(res.Differences) != 1 || res.Differences[0].Category != Duplicate {
		t.Fatalf("not grouped: %+v", res)
	}
}

func TestAmountsInDifferentCurrenciesAreNeverAdded(t *testing.T) {
	ledger := []source.Row{row("k", "EUR", 600, 0, 2), row("k", "USD", 400, 0, 3)}
	provider := []source.Row{row("k", "EUR", 1000, 0, 2)}
	res := Run(ledger, provider, config.Match{Group: true}, false)
	if len(res.Differences) != 1 || res.Differences[0].Category != CurrencyMismatch || res.Differences[0].Ledger.Currency != "EUR+USD" || res.Differences[0].Ledger.Amount != 0 {
		t.Fatalf("%+v", res.Differences)
	}
	// Missing on one side, grouped: one difference per currency.
	res = Run(ledger, nil, config.Match{Group: true}, false)
	if len(res.Differences) != 2 || res.Differences[0].Ledger.Currency != "EUR" || res.Differences[1].Ledger.Amount != 400 {
		t.Fatalf("%+v", res.Differences)
	}
}

func TestMissingRowsAreListedOneByOneWithoutGroup(t *testing.T) {
	ledger := []source.Row{row("k", "EUR", 1, 0, 2), row("k", "EUR", 2, 0, 3)}
	res := Run(ledger, nil, config.Match{}, false)
	if len(res.Differences) != 2 || res.Counts()[MissingInProvider] != 2 {
		t.Fatalf("%+v", res.Differences)
	}
}

// The order rows arrive in does not change the result.
func TestOrderOfInputDoesNotMatter(t *testing.T) {
	var ledger, provider []source.Row
	for i := 0; i < 200; i++ {
		k := string(rune('a'+i%26)) + string(rune('a'+i/26))
		ledger = append(ledger, row(k, "EUR", int64(i*10), 0, i+2))
		if i%7 != 0 {
			provider = append(provider, row(k, "EUR", int64(i*10+i%3), 0, i+2))
		}
	}
	ledger = append(ledger, row("aa", "EUR", 5, 0, 300)) // a duplicate key
	want := Run(ledger, provider, config.Match{Group: true}, false)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		l := append([]source.Row(nil), ledger...)
		p := append([]source.Row(nil), provider...)
		r.Shuffle(len(l), func(a, b int) { l[a], l[b] = l[b], l[a] })
		r.Shuffle(len(p), func(a, b int) { p[a], p[b] = p[b], p[a] })
		if got := Run(l, p, config.Match{Group: true}, false); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffle %d changed the result", i)
		}
	}
}
