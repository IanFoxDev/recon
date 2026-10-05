// Package config reads recon.yaml: the two sides to compare and how to match them.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config is a whole recon.yaml.
type Config struct {
	Version    int            `yaml:"version"`
	Currencies map[string]int `yaml:"currencies"`
	Ledger     Source         `yaml:"ledger"`
	Provider   Source         `yaml:"provider"`
	Match      Match          `yaml:"match"`
	Report     Report         `yaml:"report"`

	// Dir is the directory of the file; relative paths in it are resolved against it.
	Dir string `yaml:"-"`
}

// Source is one side of the comparison.
type Source struct {
	SQL     *SQL     `yaml:"sql"`
	CSV     *CSV     `yaml:"csv"`
	Columns Columns  `yaml:"columns"`
	Amounts string   `yaml:"amounts"` // "major" (decimal strings) or "minor" (integers)
	Filter  []Filter `yaml:"filter"`
}

// SQL reads rows from PostgreSQL. The DSN comes from an environment variable, never
// from the file.
type SQL struct {
	DSNEnv    string `yaml:"dsn_env"`
	Query     string `yaml:"query"`
	QueryFile string `yaml:"query_file"`
}

// CSV reads rows from one or more files.
type CSV struct {
	Path      string `yaml:"path"` // a glob: exports/stripe_*.csv
	Profile   string `yaml:"profile"`
	Delimiter string `yaml:"delimiter"`
	Decimal   string `yaml:"decimal"`
	Thousands string `yaml:"thousands"`
}

// Columns names the columns that hold each value.
type Columns struct {
	Key      Keys   `yaml:"key"`
	Amount   string `yaml:"amount"`
	Fee      string `yaml:"fee"`
	Currency string `yaml:"currency"`
}

// Keys is one column name or a list of them.
type Keys []string

// UnmarshalYAML accepts "key: id" as well as "key: [account, id]".
func (k *Keys) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*k = Keys{n.Value}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*k = list
	return nil
}

// Filter keeps the rows whose column has one of the values.
type Filter struct {
	Column string   `yaml:"column"`
	In     []string `yaml:"in"`
}

// Match says how rows are paired.
type Match struct {
	// Group sums rows with the same key on each side before comparing: one payment
	// at the provider can be several entries in the ledger.
	Group     bool      `yaml:"group"`
	Tolerance Tolerance `yaml:"tolerance"`
}

// Tolerance in minor units. Zero means an exact match.
type Tolerance struct {
	Amount int64 `yaml:"amount"`
	Fee    int64 `yaml:"fee"`
}

// Report says where results go.
type Report struct {
	HTML   string `yaml:"html"`
	JSON   string `yaml:"json"`
	CSV    string `yaml:"csv"`
	FailOn string `yaml:"fail_on"` // "any" (default) or "none"
}

// Load reads and checks a configuration file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c.Dir = filepath.Dir(path)
	return c, nil
}

// Parse decodes and checks a configuration. Unknown keys are errors with the line
// they are on.
func Parse(data []byte) (Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, err
	}
	c.applyDefaults()
	if errs := c.check(); len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

func (c *Config) applyDefaults() {
	for _, s := range []*Source{&c.Ledger, &c.Provider} {
		if s.CSV != nil {
			if p, ok := profiles[s.CSV.Profile]; ok {
				p.apply(s)
			}
			if s.CSV.Delimiter == "" {
				s.CSV.Delimiter = ","
			}
			if s.CSV.Decimal == "" {
				s.CSV.Decimal = "."
			}
		}
		if s.Amounts == "" {
			if s.SQL != nil {
				s.Amounts = "minor"
			} else {
				s.Amounts = "major"
			}
		}
	}
	if c.Report.FailOn == "" {
		c.Report.FailOn = "any"
	}
}

func (c *Config) check() []error {
	var errs []error
	if c.Version != 1 {
		errs = append(errs, fmt.Errorf("version: want 1, got %d", c.Version))
	}
	errs = append(errs, c.Ledger.check("ledger")...)
	errs = append(errs, c.Provider.check("provider")...)
	if c.Match.Tolerance.Amount < 0 || c.Match.Tolerance.Fee < 0 {
		errs = append(errs, errors.New("match.tolerance: must not be negative"))
	}
	if c.Report.FailOn != "any" && c.Report.FailOn != "none" {
		errs = append(errs, fmt.Errorf("report.fail_on: want any or none, got %q", c.Report.FailOn))
	}
	if c.Report.HTML == "" && c.Report.JSON == "" && c.Report.CSV == "" {
		errs = append(errs, errors.New("report: set at least one of html, json, csv"))
	}
	if (c.Ledger.Columns.Fee == "") != (c.Provider.Columns.Fee == "") {
		errs = append(errs, errors.New("columns.fee: set it on both sides or on neither"))
	}
	if len(c.Ledger.Columns.Key) != len(c.Provider.Columns.Key) {
		errs = append(errs, errors.New("columns.key: both sides need the same number of key columns"))
	}
	return errs
}

func (s *Source) check(side string) []error {
	var errs []error
	switch {
	case s.SQL == nil && s.CSV == nil:
		errs = append(errs, fmt.Errorf("%s: set sql or csv", side))
	case s.SQL != nil && s.CSV != nil:
		errs = append(errs, fmt.Errorf("%s: set sql or csv, not both", side))
	case s.SQL != nil:
		if s.SQL.DSNEnv == "" {
			errs = append(errs, fmt.Errorf("%s.sql.dsn_env: the name of the environment variable with the DSN", side))
		}
		if (s.SQL.Query == "") == (s.SQL.QueryFile == "") {
			errs = append(errs, fmt.Errorf("%s.sql: set query or query_file", side))
		}
	case s.CSV != nil:
		if s.CSV.Path == "" {
			errs = append(errs, fmt.Errorf("%s.csv.path: required", side))
		}
		if s.CSV.Profile != "" {
			if _, ok := profiles[s.CSV.Profile]; !ok {
				errs = append(errs, fmt.Errorf("%s.csv.profile: unknown %q, known: %s", side, s.CSV.Profile, strings.Join(profileNames(), ", ")))
			}
		}
		for _, sep := range [][2]string{{"delimiter", s.CSV.Delimiter}, {"decimal", s.CSV.Decimal}} {
			if len([]rune(sep[1])) != 1 {
				errs = append(errs, fmt.Errorf("%s.csv.%s: one character, got %q", side, sep[0], sep[1]))
			}
		}
		if len([]rune(s.CSV.Thousands)) > 1 {
			errs = append(errs, fmt.Errorf("%s.csv.thousands: one character or empty, got %q", side, s.CSV.Thousands))
		}
		if s.CSV.Thousands != "" && s.CSV.Thousands == s.CSV.Decimal {
			errs = append(errs, fmt.Errorf("%s.csv: thousands and decimal separators are the same", side))
		}
	}
	if len(s.Columns.Key) == 0 {
		errs = append(errs, fmt.Errorf("%s.columns.key: required", side))
	}
	if s.Columns.Amount == "" {
		errs = append(errs, fmt.Errorf("%s.columns.amount: required", side))
	}
	if s.Columns.Currency == "" {
		errs = append(errs, fmt.Errorf("%s.columns.currency: required", side))
	}
	if s.Amounts != "major" && s.Amounts != "minor" {
		errs = append(errs, fmt.Errorf("%s.amounts: want major or minor, got %q", side, s.Amounts))
	}
	for i, f := range s.Filter {
		if f.Column == "" || len(f.In) == 0 {
			errs = append(errs, fmt.Errorf("%s.filter[%d]: set column and in", side, i))
		}
	}
	return errs
}
