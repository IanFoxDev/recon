package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/money"
)

func readSQL(ctx context.Context, dir string, side config.Source, currencies money.Currencies) ([]Row, []Input, error) {
	dsn := os.Getenv(side.SQL.DSNEnv)
	if dsn == "" {
		return nil, nil, fmt.Errorf("sql: environment variable %s is empty", side.SQL.DSNEnv)
	}
	query := side.SQL.Query
	name := "sql"
	if side.SQL.QueryFile != "" {
		path := side.SQL.QueryFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("sql.query_file: %w", err)
		}
		query, name = string(data), filepath.Base(path)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("sql: connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// A read-only transaction: recon never writes to the ledger.
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, fmt.Errorf("sql: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Every value comes back as text, so numeric is never read through a float.
	result, err := tx.Query(ctx, query, pgx.QueryResultFormats{pgx.TextFormatCode})
	if err != nil {
		return nil, nil, fmt.Errorf("sql: %w", err)
	}
	defer result.Close()

	var header []string
	for _, f := range result.FieldDescriptions() {
		header = append(header, f.Name)
	}
	b, err := newBuilder(side, currencies, header, money.Format{Decimal: '.'})
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	var rows []Row
	n := 0
	for result.Next() {
		n++
		raw := result.RawValues()
		values := make([]string, len(raw))
		for i, v := range raw {
			values[i] = string(v)
		}
		row, keep, err := b.row(values, Origin{File: name, Line: n})
		if err != nil {
			return nil, nil, err
		}
		if keep {
			rows = append(rows, row)
		}
	}
	if err := result.Err(); err != nil {
		return nil, nil, fmt.Errorf("sql: %w", err)
	}
	return rows, []Input{{Name: name, Rows: len(rows)}}, nil
}
