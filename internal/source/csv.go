package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/ianfoxdev/recon/internal/config"
	"github.com/ianfoxdev/recon/internal/money"
)

func readCSV(dir string, side config.Source, currencies money.Currencies) ([]Row, []Input, error) {
	pattern := side.CSV.Path
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(dir, pattern)
	}
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("csv.path %q: %w", side.CSV.Path, err)
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("csv.path %q: no files match", side.CSV.Path)
	}
	sort.Strings(files)

	format := money.Format{Decimal: []rune(side.CSV.Decimal)[0]}
	if side.CSV.Thousands != "" {
		format.Thousands = []rune(side.CSV.Thousands)[0]
	}
	var rows []Row
	var inputs []Input
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.Sum256(data)
		fileRows, err := parseCSV(data, filepath.Base(file), []rune(side.CSV.Delimiter)[0], side, currencies, format)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, fileRows...)
		inputs = append(inputs, Input{Name: filepath.Base(file), SHA256: hex.EncodeToString(sum[:]), Rows: len(fileRows)})
	}
	return rows, inputs, nil
}

func parseCSV(data []byte, name string, delimiter rune, side config.Source, currencies money.Currencies, format money.Format) ([]Row, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // a BOM from spreadsheet exports
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delimiter
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: empty file", name)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	b, err := newBuilder(side, currencies, header, format)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	var rows []Row
	for {
		values, err := r.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		line, _ := r.FieldPos(0)
		row, keep, err := b.row(values, Origin{File: name, Line: line})
		if err != nil {
			return nil, err
		}
		if keep {
			rows = append(rows, row)
		}
	}
}
