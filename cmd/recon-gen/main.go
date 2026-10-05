// Command recon-gen writes a demo ledger (SQL) and a provider export (CSV) with known
// differences, and the list of those differences: go run ./cmd/recon-gen -out examples/stripe/data
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/ianfoxdev/recon/internal/demo"
)

func main() {
	out := flag.String("out", "demo", "directory to write into")
	seed := flag.Uint64("seed", 1, "random seed; the same seed gives the same files")
	n := flag.Int("payments", 1000, "payments that reconcile")
	planted := flag.Int("planted", 3, "differences of each category")
	flag.Parse()

	d := demo.Generate(*seed, *n, *planted)
	expected, err := json.MarshalIndent(d.Expected, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	files := map[string][]byte{
		"ledger.sql":                 []byte(d.SQL),
		"exports/stripe_balance.csv": d.CSV,
		"expected.json":              append(expected, '\n'),
	}
	for name, data := range files {
		p := filepath.Join(*out, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
