// Package match pairs ledger rows with provider rows by key and reports every
// difference. The result depends only on the rows, not on the order they came in.
package match

import (
	"sort"
	"strings"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/source"
)

// Category of a difference.
type Category string

// The categories, in the order a report lists them.
const (
	MissingInLedger   Category = "missing_in_ledger"
	MissingInProvider Category = "missing_in_provider"
	AmountMismatch    Category = "amount_mismatch"
	FeeMismatch       Category = "fee_mismatch"
	CurrencyMismatch  Category = "currency_mismatch"
	Duplicate         Category = "duplicate"
)

// Categories lists every category in report order.
var Categories = []Category{MissingInLedger, MissingInProvider, AmountMismatch, FeeMismatch, CurrencyMismatch, Duplicate}

// Side is what one side has for a key: totals and the rows behind them.
type Side struct {
	Currency string
	Amount   int64
	Fee      int64
	Rows     []source.Origin
}

// Difference is one thing that does not reconcile.
type Difference struct {
	Category Category
	Key      []string
	Ledger   *Side // nil when the ledger has nothing for the key
	Provider *Side
	// Delta is provider minus ledger for amount and fee mismatches, in minor units.
	Delta int64
}

// Result of a run.
type Result struct {
	Matched     int // keys that reconcile
	Differences []Difference
}

// Counts the differences by category.
func (r Result) Counts() map[Category]int {
	c := make(map[Category]int, len(Categories))
	for _, d := range r.Differences {
		c[d.Category]++
	}
	return c
}

// Run compares the two sides.
func Run(ledger, provider []source.Row, m config.Match, withFee bool) Result {
	l := byKey(ledger)
	p := byKey(provider)

	keys := make([]string, 0, len(l)+len(p))
	for k := range l {
		keys = append(keys, k)
	}
	for k := range p {
		if _, ok := l[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var res Result
	for _, k := range keys {
		lr, pr := l[k], p[k]
		key := splitKey(k)
		switch {
		case len(pr) == 0:
			res.Differences = append(res.Differences, missing(MissingInProvider, key, lr, m.Group)...)
			continue
		case len(lr) == 0:
			res.Differences = append(res.Differences, missing(MissingInLedger, key, pr, m.Group)...)
			continue
		}
		if mixed(lr) || mixed(pr) || lr[0].Currency != pr[0].Currency {
			// Amounts in different currencies are never added or compared.
			res.Differences = append(res.Differences, Difference{Category: CurrencyMismatch, Key: key, Ledger: rowsOnly(lr), Provider: rowsOnly(pr)})
			continue
		}
		if !m.Group && (len(lr) > 1 || len(pr) > 1) {
			d := Difference{Category: Duplicate, Key: key}
			if len(lr) > 1 {
				d.Ledger = total(lr)
			}
			if len(pr) > 1 {
				d.Provider = total(pr)
			}
			res.Differences = append(res.Differences, d)
			continue
		}
		ls, ps := total(lr), total(pr)
		ok := true
		if abs(ps.Amount-ls.Amount) > m.Tolerance.Amount {
			res.Differences = append(res.Differences, Difference{Category: AmountMismatch, Key: key, Ledger: ls, Provider: ps, Delta: ps.Amount - ls.Amount})
			ok = false
		}
		if withFee && abs(ps.Fee-ls.Fee) > m.Tolerance.Fee {
			res.Differences = append(res.Differences, Difference{Category: FeeMismatch, Key: key, Ledger: ls, Provider: ps, Delta: ps.Fee - ls.Fee})
			ok = false
		}
		if ok {
			res.Matched++
		}
	}
	return res
}

const sep = "\x1f"

func byKey(rows []source.Row) map[string][]source.Row {
	m := make(map[string][]source.Row)
	for _, r := range rows {
		k := strings.Join(r.Key, sep)
		m[k] = append(m[k], r)
	}
	for _, rs := range m {
		// Rows of one key in an order defined by their content, so a SQL query
		// without ORDER BY gives the same report.
		sort.SliceStable(rs, func(i, j int) bool {
			a, b := rs[i], rs[j]
			if a.Currency != b.Currency {
				return a.Currency < b.Currency
			}
			if a.Amount != b.Amount {
				return a.Amount < b.Amount
			}
			if a.Fee != b.Fee {
				return a.Fee < b.Fee
			}
			if a.Origin.File != b.Origin.File {
				return a.Origin.File < b.Origin.File
			}
			return a.Origin.Line < b.Origin.Line
		})
	}
	return m
}

func splitKey(k string) []string { return strings.Split(k, sep) }

// missing reports rows that have no counterpart: one difference per key when rows
// are grouped, one per row otherwise.
func missing(c Category, key []string, rows []source.Row, group bool) []Difference {
	side := func(d *Difference, s *Side) {
		if c == MissingInProvider {
			d.Ledger = s
		} else {
			d.Provider = s
		}
	}
	var out []Difference
	if group {
		// One difference per currency: amounts in different currencies are not added.
		for _, rs := range byCurrency(rows) {
			d := Difference{Category: c, Key: key}
			side(&d, total(rs))
			out = append(out, d)
		}
		return out
	}
	for _, r := range rows {
		d := Difference{Category: c, Key: key}
		side(&d, total([]source.Row{r}))
		out = append(out, d)
	}
	return out
}

// mixed reports whether rows of one key are in more than one currency.
func mixed(rows []source.Row) bool {
	for _, r := range rows[1:] {
		if r.Currency != rows[0].Currency {
			return true
		}
	}
	return false
}

// byCurrency splits rows by currency, in currency order (rows are sorted by currency first).
func byCurrency(rows []source.Row) [][]source.Row {
	var out [][]source.Row
	for i, r := range rows {
		if i == 0 || r.Currency != rows[i-1].Currency {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], r)
	}
	return out
}

// rowsOnly lists the rows of a key without adding up amounts that may be in
// different currencies. Currency names them all, joined with "+".
func rowsOnly(rows []source.Row) *Side {
	s := &Side{}
	var currencies []string
	for _, r := range rows {
		s.Rows = append(s.Rows, r.Origin)
		if len(currencies) == 0 || currencies[len(currencies)-1] != r.Currency {
			currencies = append(currencies, r.Currency)
		}
	}
	s.Currency = strings.Join(currencies, "+")
	if len(currencies) == 1 {
		for _, r := range rows {
			s.Amount += r.Amount
			s.Fee += r.Fee
		}
	}
	return s
}

// total sums rows of one key, all in one currency.
func total(rows []source.Row) *Side {
	s := &Side{Currency: rows[0].Currency}
	for _, r := range rows {
		s.Amount += r.Amount
		s.Fee += r.Fee
		s.Rows = append(s.Rows, r.Origin)
	}
	return s
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
