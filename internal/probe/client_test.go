package probe

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	for _, cfg := range []Config{
		{Family: Family("bad")},
		{Family: IPv4, SourceIP: "2001:db8::1"},
		{Family: IPv6, SourceIP: "192.0.2.1"},
		{SourceIP: "not-an-ip"},
		{Interface: "unlockscope-interface-that-does-not-exist"},
		{Proxy: "ftp://127.0.0.1:21"},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("config %+v unexpectedly accepted", cfg)
		}
	}
}

func TestConfigAcceptsFamiliesAndSource(t *testing.T) {
	for _, cfg := range []Config{
		{Family: Auto, Timeout: time.Second},
		{Family: IPv4, SourceIP: "127.0.0.1", Timeout: time.Second},
		{Family: IPv6, SourceIP: "::1", Timeout: time.Second},
	} {
		if _, err := New(cfg); err != nil {
			t.Fatalf("config %+v: %v", cfg, err)
		}
	}
}
