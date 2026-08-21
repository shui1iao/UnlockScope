package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shui1iao/UnlockScope/internal/model"
	"github.com/shui1iao/UnlockScope/internal/probe"
)

func TestCatalogIsUniqueAndCoversRequiredServices(t *testing.T) {
	all := All()
	if len(all) < 120 {
		t.Fatalf("provider count = %d, want at least 120", len(all))
	}
	seen := make(map[string]bool, len(all))
	required := []string{"tiktok", "netflix", "disney-plus", "youtube-premium", "youtube-cdn", "prime-video", "spotify", "dazn", "max", "hulu", "paramount-plus", "peacock", "chatgpt", "claude", "gemini", "copilot", "grok", "perplexity", "meta-ai", "poe", "reddit", "wikipedia-editability", "steam-currency"}
	for _, p := range all {
		d := p.Definition()
		if d.ID == "" || d.Service == "" || d.URL == "" {
			t.Errorf("incomplete definition: %+v", d)
		}
		if seen[d.ID] {
			t.Errorf("duplicate provider ID %q", d.ID)
		}
		seen[d.ID] = true
	}
	for _, id := range required {
		if !seen[id] {
			t.Errorf("required provider %q missing", id)
		}
	}
	for _, region := range []string{"hk", "tw", "jp", "kr", "na", "eu", "sa", "af", "oc"} {
		matched := Filter(all, region, region)
		if len(matched) == 0 {
			t.Errorf("region %s has no providers", region)
		}
	}
	if got := len(Filter(all, "global", "")); got == 0 {
		t.Error("global scope is empty")
	}
	if got := len(Filter(all, "ai", "")); got < 8 {
		t.Errorf("AI scope = %d, want at least 8", got)
	}
	if got := len(Filter(all, "games", "")); got < 5 {
		t.Errorf("games scope = %d, want at least 5", got)
	}
}

func TestFindPreservesOrderAndRejectsUnknown(t *testing.T) {
	got, err := Find([]string{"reddit", "tiktok", "reddit"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Definition().ID != "reddit" || got[1].Definition().ID != "tiktok" {
		t.Fatalf("unexpected order: %#v", got)
	}
	if _, err := Find([]string{"not-a-provider"}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestCheckCommonHTTPSignals(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		kind       string
		want       model.State
	}{
		{"available", "<html>watch now</html>", http.StatusOK, "html", model.Available},
		{"ambiguous-200", "<html>hello</html>", http.StatusOK, "html", model.Unknown},
		{"region", "not available in your country", http.StatusOK, "html", model.RegionOnly},
		{"forbidden", "access denied", http.StatusForbidden, "html", model.Unknown},
		{"forbidden-region", "country or region restriction", http.StatusForbidden, "html", model.RegionOnly},
		{"rate-limit", "slow down", http.StatusTooManyRequests, "html", model.Unknown},
		{"server-error", "oops", http.StatusBadGateway, "html", model.Failed},
		{"bad-json", "<html>login</html>", http.StatusOK, "json", model.Unknown},
		{"good-json", `{"ok":true}`, http.StatusOK, "json", model.Available},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client, err := probe.New(probe.Config{Family: probe.Auto})
			if err != nil {
				t.Fatal(err)
			}
			p := serviceProvider{def: Definition{ID: tc.name, Service: tc.name, URL: srv.URL, Kind: tc.kind, Category: "test", AvailableWords: []string{"watch now"}, RegionWords: commonRegionWords, UnavailableWords: commonUnavailableWords}}
			result := p.Check(context.Background(), client, "hk")
			if result.State != tc.want {
				t.Fatalf("state = %s, want %s (note %s)", result.State, tc.want, result.Note)
			}
			if result.DurationMS < 0 || result.CheckedAt.IsZero() {
				t.Fatalf("missing timing: %+v", result)
			}
		})
	}
}

func TestCheckRedirectAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/ok":
			_, _ = w.Write([]byte("ready"))
		case "/slow":
			time.Sleep(150 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		}
	}))
	defer server.Close()
	client, err := probe.New(probe.Config{})
	if err != nil {
		t.Fatal(err)
	}
	redirect := serviceProvider{def: Definition{ID: "redirect", Service: "redirect", URL: server.URL + "/redirect", Kind: "html", AvailableWords: []string{"ready"}}}
	if got := redirect.Check(context.Background(), client, "").State; got != model.Available {
		t.Fatalf("redirect state = %s", got)
	}
	slow := serviceProvider{def: Definition{ID: "slow", Service: "slow", URL: server.URL + "/slow", Kind: "html"}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := slow.Check(ctx, client, "")
	if result.State != model.Failed || !strings.Contains(result.Note, "超时") {
		t.Fatalf("timeout result = %+v", result)
	}
}

func TestAmbiguousSuccessStaysUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("generic landing page"))
	}))
	defer server.Close()
	client, err := probe.New(probe.Config{})
	if err != nil {
		t.Fatal(err)
	}
	item := serviceProvider{def: Definition{ID: "ambiguous", Service: "Ambiguous", URL: server.URL, Kind: "html"}}
	if result := item.Check(context.Background(), client, ""); result.State != model.Unknown {
		t.Fatalf("ambiguous HTTP 200 state = %s, want unknown", result.State)
	}
}

func TestCheckPreservesSelectedRegionWhenPageContainsUntrustedRegionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<script>window.region='AF'</script> watch now"))
	}))
	defer server.Close()

	client, err := probe.New(probe.Config{})
	if err != nil {
		t.Fatal(err)
	}
	item := serviceProvider{def: Definition{
		ID:             "untrusted-region",
		Service:        "Untrusted region",
		URL:            server.URL,
		Kind:           "html",
		AvailableWords: []string{"watch now"},
	}}
	result := item.Check(context.Background(), client, "na")
	if result.Region != "na" {
		t.Fatalf("region = %q, want selected egress region %q", result.Region, "na")
	}
}
