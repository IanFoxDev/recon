// Package source reads the rows of one side: a CSV export or a SQL query.
package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/money"
)

// Row is one money movement, amounts in minor units.
type Row struct {
	Key      []string
	Amount   int64
	Fee      int64
	Currency string
	Origin   Origin
}

// Origin says where a row came from, for error messages and the report.
type Origin struct {
	File string // file name, or "sql"
	Line int    // line in the file (header is line 1), or row number in the result
}

func (o Origin) String() string { return fmt.Sprintf("%s:%d", o.File, o.Line) }

// Input describes what was read, so a report can say what it was built from.
type Input struct {
	Name   string
	SHA256 string // empty for SQL
	Rows   int
}

// Read reads one side as the configuration describes it.
func Read(ctx context.Context, cfg config.Config, side config.Source, currencies money.Currencies) ([]Row, []Input, error) {
	switch {
	case side.CSV != nil:
		return readCSV(cfg.Dir, side, currencies)
	case side.SQL != nil:
		return readSQL(ctx, cfg.Dir, side, currencies)
	}
	return nil, nil, fmt.Errorf("no source configured")
}

// builder turns named string values into a Row, the same way for CSV and SQL.
type builder struct {
	side       config.Source
	currencies money.Currencies
	format     money.Format
	index      map[string]int
}

func newBuilder(side config.Source, currencies money.Currencies, header []string, format money.Format) (*builder, error) {
	index := make(map[string]int, len(header))
	for i, h := range header {
		index[strings.TrimSpace(h)] = i
	}
	b := &builder{side: side, currencies: currencies, format: format, index: index}
	need := append([]string{side.Columns.Amount, side.Columns.Currency}, side.Columns.Key...)
	if side.Columns.Fee != "" {
		need = append(need, side.Columns.Fee)
	}
	for _, f := range side.Filter {
		need = append(need, f.Column)
	}
	for _, n := range need {
		if _, ok := index[n]; !ok {
			return nil, fmt.Errorf("column %q not found; columns are: %s", n, strings.Join(header, ", "))
		}
	}
	return b, nil
}

// row returns the Row, false if a filter drops it, or an error naming the column.
func (b *builder) row(values []string, origin Origin) (Row, bool, error) {
	get := func(col string) string { return strings.TrimSpace(values[b.index[col]]) }
	for _, f := range b.side.Filter {
		keep := false
		for _, v := range f.In {
			if get(f.Column) == v {
				keep = true
				break
			}
		}
		if !keep {
			return Row{}, false, nil
		}
	}
	r := Row{Origin: origin, Currency: strings.ToUpper(get(b.side.Columns.Currency))}
	exp, ok := b.currencies.Exponent(r.Currency)
	if !ok {
		return Row{}, false, fmt.Errorf("%s: column %s: unknown currency %q; add it under currencies", origin, b.side.Columns.Currency, r.Currency)
	}
	for _, k := range b.side.Columns.Key {
		v := get(k)
		if v == "" {
			return Row{}, false, fmt.Errorf("%s: column %s: empty key", origin, k)
		}
		r.Key = append(r.Key, v)
	}
	var err error
	if r.Amount, err = b.amount(get(b.side.Columns.Amount), exp); err != nil {
		return Row{}, false, fmt.Errorf("%s: column %s: %w", origin, b.side.Columns.Amount, err)
	}
	if b.side.Columns.Fee != "" {
		if r.Fee, err = b.amount(get(b.side.Columns.Fee), exp); err != nil {
			return Row{}, false, fmt.Errorf("%s: column %s: %w", origin, b.side.Columns.Fee, err)
		}
	}
	return r, true, nil
}

// amount parses a value in major units, or an integer count of minor units.
func (b *builder) amount(v string, exp int) (int64, error) {
	if b.side.Amounts == "minor" {
		return money.Parse(v, 0, money.Format{})
	}
	return money.Parse(v, exp, b.format)
}
