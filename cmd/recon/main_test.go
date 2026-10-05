package main

import (
	"bytes"
	"testing"
)

func TestVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, errOut.String())
	}
	if out.String() != "dev\n" {
		t.Fatalf("version %q", out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"reconcile"}, &out, &errOut); code != 2 {
		t.Fatalf("exit code %d", code)
	}
}
