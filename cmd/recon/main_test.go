package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func fixedNow() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }

func runRecon(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, fixedNow)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	if code, out, _ := runRecon(t, "version"); code != 0 || out != "dev\n" {
		t.Fatalf("code %d, out %q", code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	if code, _, _ := runRecon(t, "reconcile"); code != exitError {
		t.Fatalf("code %d", code)
	}
}

// copyShop puts the example shop into a temporary directory, so reports are written there.
func copyShop(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.Walk("testdata/shop", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("testdata/shop", p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunReportsEveryKindOfDifference(t *testing.T) {
	dir := copyShop(t)
	code, out, errOut := runRecon(t, "run", "-c", filepath.Join(dir, "recon.yaml"))
	if code != exitDifferences {
		t.Fatalf("code %d, stderr %s", code, errOut)
	}
	want := "reconciled 2, differences 5, missing_in_ledger 1, missing_in_provider 1, amount_mismatch 1, fee_mismatch 1, currency_mismatch 1\n"
	if out != want {
		t.Fatalf("stdout\n%s\nwant\n%s", out, want)
	}
	for _, name := range []string{"report.json", "differences.csv", "report.html"} {
		got, err := os.ReadFile(filepath.Join(dir, "out", name))
		if err != nil {
			t.Fatal(err)
		}
		golden := filepath.Join("testdata", "golden", name)
		if *update {
			if err := os.WriteFile(golden, got, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from %s; run go test ./cmd/recon -update if the change is intended", name, golden)
		}
	}
}

func TestSameInputSameReport(t *testing.T) {
	dir := copyShop(t)
	read := func() []byte {
		runRecon(t, "run", "-c", filepath.Join(dir, "recon.yaml"))
		b, err := os.ReadFile(filepath.Join(dir, "out", "report.html"))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if first, second := read(), read(); !bytes.Equal(first, second) {
		t.Fatal("two runs on the same files wrote different reports")
	}
}

func TestStampAndFailOnNone(t *testing.T) {
	dir := copyShop(t)
	cfg := filepath.Join(dir, "recon.yaml")
	data, _ := os.ReadFile(cfg)
	if err := os.WriteFile(cfg, []byte(strings.Replace(string(data), "report:\n", "report:\n  fail_on: none\n", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runRecon(t, "run", "-c", cfg, "--stamp"); code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "out", "report.json"))
	if !strings.Contains(string(b), `"generated_at": "2026-10-05T12:00:00Z"`) {
		t.Fatalf("no stamp in %s", b)
	}
}

func TestBrokenInputIsExitTwo(t *testing.T) {
	dir := copyShop(t)
	if err := os.WriteFile(filepath.Join(dir, "ledger.csv"), []byte("order_id,psp_reference,amount,fee,currency\n1,pi_ok,10.999,0,USD\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runRecon(t, "run", "-c", filepath.Join(dir, "recon.yaml"))
	if code != exitError || !strings.Contains(errOut, "ledger: ledger.csv:2: column amount") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}
