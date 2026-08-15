package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shui1iao/UnlockScope/internal/model"
	"github.com/shui1iao/UnlockScope/internal/probe"
	"github.com/shui1iao/UnlockScope/internal/provider"
)

var version = "v0.1.0"

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			*s = append(*s, item)
		}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "unlockscope:", err)
		os.Exit(2)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("unlockscope", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var ids stringList
	fs.Var(&ids, "provider", "provider ID (repeatable or comma-separated)")
	fs.Var(&ids, "p", "alias for --provider")
	scope := fs.String("scope", "auto", "scope: auto, all, global, ai, social, tw, hk, jp, kr, na, sa, eu, af, oc, sports, games")
	region := fs.String("region", "", "override detected egress region (country code or scope)")
	family := fs.String("ip", "auto", "IP family: auto, ipv4, ipv6")
	proxy := fs.String("proxy", "", "HTTP(S) or SOCKS5 proxy URL")
	interfaceName := fs.String("interface", "", "source network interface")
	sourceIP := fs.String("source", "", "source IP address")
	perTimeout := fs.Duration("timeout", 8*time.Second, "per-provider timeout")
	totalTimeout := fs.Duration("total-timeout", 60*time.Second, "global timeout")
	globalTimeout := fs.Duration("global-timeout", 0, "alias for --total-timeout")
	concurrency := fs.Int("concurrency", 8, "maximum concurrent checks")
	jsonOutput := fs.Bool("json", false, "emit JSON array")
	noColor := fs.Bool("no-color", false, "disable ANSI colors")
	plain := fs.Bool("plain", false, "alias for --no-color")
	list := fs.Bool("list-providers", false, "list providers and exit")
	listAlias := fs.Bool("list", false, "alias for --list-providers")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	if *list || *listAlias {
		return printProviders(stdout, provider.All())
	}
	if *globalTimeout > 0 {
		*totalTimeout = *globalTimeout
	}
	if *perTimeout <= 0 || *totalTimeout <= 0 {
		return errors.New("timeouts must be positive")
	}
	if *concurrency < 1 || *concurrency > 64 {
		return errors.New("concurrency must be between 1 and 64")
	}
	if *totalTimeout < *perTimeout {
		return errors.New("total-timeout must be at least timeout")
	}
	if !validScope(strings.ToLower(strings.TrimSpace(*scope))) {
		return fmt.Errorf("invalid scope %q", *scope)
	}

	parsedFamily, err := parseFamily(*family)
	if err != nil {
		return err
	}
	client, err := probe.New(probe.Config{Family: parsedFamily, Proxy: *proxy, Interface: *interfaceName, SourceIP: *sourceIP, Timeout: *perTimeout})
	if err != nil {
		return err
	}
	selected, err := provider.Find(ids)
	if err != nil {
		return err
	}
	actualRegion := normalizeRegion(*region)
	if actualRegion == "" && strings.EqualFold(strings.TrimSpace(*scope), "auto") {
		geoCtx, cancel := context.WithTimeout(context.Background(), minDuration(*perTimeout, 3*time.Second))
		actualCountry, geoErr := client.DetectRegion(geoCtx)
		cancel()
		if geoErr == nil {
			actualRegion = normalizeRegion(actualCountry)
		}
	}
	chosen := selected
	if len(ids) == 0 {
		chosen = provider.Filter(selected, *scope, actualRegion)
	}
	if len(chosen) == 0 {
		return errors.New("no providers match the requested scope")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *totalTimeout)
	defer cancel()
	results := checkAll(ctx, client, chosen, actualRegion, *perTimeout, *concurrency)
	if *jsonOutput {
		return writeJSON(stdout, results)
	}
	return writeText(stdout, results, *noColor || *plain)
}

func checkAll(ctx context.Context, client *probe.Client, providers []provider.Provider, region string, timeout time.Duration, concurrency int) []model.Result {
	results := make([]model.Result, len(providers))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(index int, p provider.Provider) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[index] = cancelledResult(p, region)
				return
			}
			defer func() { <-sem }()
			itemCtx, cancel := context.WithTimeout(ctx, timeout)
			results[index] = p.Check(itemCtx, client, region)
			cancel()
		}(i, p)
	}
	wg.Wait()
	return results
}

func cancelledResult(p provider.Provider, region string) model.Result {
	d := p.Definition()
	return model.Result{ID: d.ID, Service: d.Service, Category: d.Category, Regions: append([]string{}, d.Regions...), Region: region, State: model.Failed, Note: "全局超时，未开始请求", CheckedAt: time.Now()}
}

func writeJSON(w io.Writer, results []model.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func writeText(w io.Writer, results []model.Result, plain bool) error {
	fmt.Fprintln(w, "STATE        SERVICE                         REGION  TIME   NOTE")
	fmt.Fprintln(w, "------------ ------------------------------- ------- ------ ------------------------------")
	for _, result := range results {
		state := string(result.State)
		if !plain {
			state = colorState(result.State)
		}
		fmt.Fprintf(w, "%-12s %-31s %-7s %5dms %s\n", state, truncate(result.Service, 31), result.Region, result.DurationMS, result.Note)
	}
	return nil
}

func printProviders(w io.Writer, providers []provider.Provider) error {
	for _, p := range providers {
		d := p.Definition()
		regions := strings.Join(d.Regions, ",")
		groups := strings.Join(d.Groups, ",")
		if _, err := fmt.Fprintf(w, "%-24s %-30s %-10s groups=%-20s regions=%-8s %s\n", d.ID, d.Service, d.Category, groups, regions, d.URL); err != nil {
			return err
		}
	}
	return nil
}

func colorState(state model.State) string {
	const reset = "\033[0m"
	colors := map[model.State]string{model.Available: "\033[32m", model.Unavailable: "\033[31m", model.RegionOnly: "\033[33m", model.Failed: "\033[35m", model.Unknown: "\033[36m"}
	if color, ok := colors[state]; ok {
		return color + string(state) + reset
	}
	return string(state)
}
func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func parseFamily(value string) (probe.Family, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return probe.Auto, nil
	case "4", "ipv4":
		return probe.IPv4, nil
	case "6", "ipv6":
		return probe.IPv6, nil
	default:
		return "", fmt.Errorf("invalid IP family %q (use auto, 4, or 6)", value)
	}
}

func normalizeRegion(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) == 2 {
		switch value {
		case "hk", "tw", "jp", "kr":
			return value
		case "us", "ca", "mx", "gl", "bm":
			return "na"
		case "br", "ar", "cl", "co", "pe", "ve", "uy", "py", "bo", "ec", "gy", "sr":
			return "sa"
		case "au", "nz", "fj", "pg", "ws", "to", "vu", "nc", "pf":
			return "oc"
		case "za", "ng", "ke", "gh", "eg", "ma", "dz", "tn", "et", "tz", "ug", "sn", "ci", "ao", "mz", "bw", "na", "zw", "zm":
			return "af"
		case "gb", "ie", "fr", "de", "nl", "be", "lu", "es", "pt", "it", "ch", "at", "pl", "cz", "sk", "hu", "ro", "bg", "gr", "cy", "mt", "dk", "se", "no", "fi", "is", "ee", "lv", "lt", "si", "hr", "rs", "ba", "me", "mk", "al", "md", "ua":
			return "eu"
		}
	}
	if validRegion(value) {
		return value
	}
	return ""
}
func validRegion(value string) bool {
	switch value {
	case "tw", "hk", "jp", "kr", "na", "sa", "eu", "af", "oc":
		return true
	}
	return false
}
func validScope(value string) bool {
	if value == "" {
		return false
	}
	if value == "auto" || value == "all" || value == "global" || value == "ai" || value == "social" || value == "sports" || value == "games" {
		return true
	}
	return validRegion(value)
}
