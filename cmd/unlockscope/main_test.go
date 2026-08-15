package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shui1iao/UnlockScope/internal/model"
)

func TestCLIValidationAndVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"--version"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "v0.1.0" {
		t.Fatalf("version output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"--list"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "netflix") {
		t.Fatal("--list did not include netflix")
	}
	for _, args := range [][]string{{"--scope", "nope"}, {"--ip", "bad"}, {"--concurrency", "0"}, {"--timeout", "0s"}} {
		out.Reset()
		errOut.Reset()
		if err := run(args, &out, &errOut); err == nil {
			t.Fatalf("args %v unexpectedly accepted", args)
		}
	}
}

func TestJSONSchemaFields(t *testing.T) {
	input := []model.Result{{ID: "demo", Service: "Demo", Category: "streaming", Regions: []string{"hk"}, State: model.Available, Region: "hk", Note: "ok", DurationMS: 4}}
	var buf bytes.Buffer
	if err := writeJSON(&buf, input); err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "service", "category", "regions", "state", "region", "note", "duration_ms", "checked_at"} {
		if _, ok := decoded[0][key]; !ok {
			t.Errorf("JSON missing %q", key)
		}
	}
}

func TestNormalizeRegion(t *testing.T) {
	cases := map[string]string{"HK": "hk", "us": "na", "AU": "oc", "de": "eu", "eu": "eu", "": ""}
	for input, want := range cases {
		if got := normalizeRegion(input); got != want {
			t.Errorf("normalizeRegion(%q) = %q, want %q", input, got, want)
		}
	}
}
