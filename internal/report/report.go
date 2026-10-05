// Package report writes the result of a run as JSON, CSV and a single HTML file.
// The same result gives the same bytes: no clock unless asked for, sorted output.
package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"

	"github.com/ianfoxdev/recon/internal/match"
	"github.com/ianfoxdev/recon/internal/money"
	"github.com/ianfoxdev/recon/internal/source"
)

// Report is the JSON document, and the data behind the HTML and CSV.
type Report struct {
	Version     string       `json:"recon_version"`
	GeneratedAt string       `json:"generated_at,omitempty"`
	Inputs      Inputs       `json:"inputs"`
	Matched     int          `json:"matched"`
	Counts      []Count      `json:"counts"`
	Differences []Difference `json:"differences"`
}

// Inputs is what each side was built from.
type Inputs struct {
	Ledger   []source.Input `json:"ledger"`
	Provider []source.Input `json:"provider"`
}

// Count of differences in one category; every category is listed, zeros included.
type Count struct {
	Category match.Category `json:"category"`
	Count    int            `json:"count"`
}

// Difference as written in the report.
type Difference struct {
	Category match.Category `json:"category"`
	Key      []string       `json:"key"`
	Ledger   *Side          `json:"ledger"`
	Provider *Side          `json:"provider"`
	Delta    *Amount        `json:"delta,omitempty"`
}

// Side of a difference.
type Side struct {
	Currency string   `json:"currency"`
	Amount   *Amount  `json:"amount,omitempty"`
	Fee      *Amount  `json:"fee,omitempty"`
	Rows     []string `json:"rows"`
}

// Amount in minor units and as a decimal string.
type Amount struct {
	Minor   int64  `json:"minor"`
	Decimal string `json:"decimal"`
}

// Build turns a match result into a report.
func Build(version, generatedAt string, inputs Inputs, res match.Result, currencies money.Currencies, withFee bool) Report {
	r := Report{Version: version, GeneratedAt: generatedAt, Inputs: inputs, Matched: res.Matched, Differences: []Difference{}}
	counts := res.Counts()
	for _, c := range match.Categories {
		r.Counts = append(r.Counts, Count{Category: c, Count: counts[c]})
	}
	for _, d := range res.Differences {
		out := Difference{
			Category: d.Category,
			Key:      d.Key,
			Ledger:   side(d.Ledger, currencies, withFee),
			Provider: side(d.Provider, currencies, withFee),
		}
		if d.Category == match.AmountMismatch || d.Category == match.FeeMismatch {
			out.Delta = amount(d.Delta, d.Provider.Currency, currencies)
		}
		r.Differences = append(r.Differences, out)
	}
	return r
}

func side(s *match.Side, currencies money.Currencies, withFee bool) *Side {
	if s == nil {
		return nil
	}
	out := &Side{Currency: s.Currency, Rows: make([]string, 0, len(s.Rows))}
	if !strings.Contains(s.Currency, "+") {
		out.Amount = amount(s.Amount, s.Currency, currencies)
		if withFee {
			out.Fee = amount(s.Fee, s.Currency, currencies)
		}
	}
	for _, o := range s.Rows {
		out.Rows = append(out.Rows, o.String())
	}
	return out
}

func amount(minor int64, currency string, currencies money.Currencies) *Amount {
	exp, _ := currencies.Exponent(currency)
	return &Amount{Minor: minor, Decimal: money.FormatMinor(minor, exp)}
}

// Total is the number of differences, which decides the exit code.
func (r Report) Total() int { return len(r.Differences) }

// JSON with two-space indentation and a final newline.
func (r Report) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// CSV with one line per difference.
func (r Report) CSV() ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"category", "key", "ledger_currency", "ledger_amount", "ledger_fee", "provider_currency", "provider_amount", "provider_fee", "delta", "ledger_rows", "provider_rows"})
	for _, d := range r.Differences {
		lc, la, lf, lr := sideColumns(d.Ledger)
		pc, pa, pf, pr := sideColumns(d.Provider)
		delta := ""
		if d.Delta != nil {
			delta = d.Delta.Decimal
		}
		_ = w.Write([]string{string(d.Category), strings.Join(d.Key, "|"), lc, la, lf, pc, pa, pf, delta, lr, pr})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func sideColumns(s *Side) (currency, amount, fee, rows string) {
	if s == nil {
		return "", "", "", ""
	}
	if s.Amount != nil {
		amount = s.Amount.Decimal
	}
	if s.Fee != nil {
		fee = s.Fee.Decimal
	}
	return s.Currency, amount, fee, strings.Join(s.Rows, " ")
}

// label turns a category into words for the HTML report.
func label(c match.Category) string {
	return strings.ReplaceAll(string(c), "_", " ")
}
