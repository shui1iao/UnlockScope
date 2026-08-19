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
	if strings.TrimSpace(out.String()) != "v0.1.1" {
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

func TestTextOutputUsesEnglishCategoriesAndUppercaseRegions(t *testing.T) {
	input := []model.Result{
		{Service: "Netflix", Category: "streaming", State: model.Available, Region: "jp", Note: "ok", DurationMS: 4},
		{Service: "Disney+", Category: "streaming", State: model.Unavailable, Region: "kr", DurationMS: 5},
		{Service: "Claude", Category: "ai", State: model.Unknown, DurationMS: 6},
		{Service: "Steam Store", Category: "games", State: model.RegionOnly, Region: "us", DurationMS: 7},
	}
	var buf bytes.Buffer
	if err := writeText(&buf, input, true); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"[ Streaming ]",
		"[ AI ]",
		"[ Games / Stores ]",
		"Netflix:",
		"可用（JP）",
		"不可用",
		"未知",
		"仅地区可用（US）",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("text output missing %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"[ 流媒体 ]", "可用 (JP)", "不可用（KR）", "（jp）", "（us）", "\x1b["} {
		if strings.Contains(output, unwanted) {
			t.Errorf("text output contains %q:\n%s", unwanted, output)
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
