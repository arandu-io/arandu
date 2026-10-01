package unit_test

import (
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/framework/foundation/bootstrap"
)

// httpConfig loads the application configuration with the HTTP variables set as
// given and every other one cleared, so a value exported in a shell cannot
// answer for one a case did not set.
func httpConfig(t *testing.T, vars map[string]string) (appconfig.Config, error) {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	for _, key := range []string{"HTTP_MAX_BODY_BYTES", "TRUSTED_PROXIES"} {
		t.Setenv(key, "")
	}
	for key, value := range vars {
		t.Setenv(key, value)
	}

	base, err := bootstrap.LoadConfiguration()
	if err != nil {
		t.Fatalf("loading the framework configuration: %v", err)
	}
	return appconfig.From(base)
}

func TestTheBodyLimitDefaultsToAFormSizedCeiling(t *testing.T) {
	cfg, err := httpConfig(t, nil)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if cfg.HTTP.MaxBodyBytes != appconfig.DefaultMaxBodyBytes || appconfig.DefaultMaxBodyBytes != 4<<20 {
		t.Fatalf("MaxBodyBytes = %d, want the 4 MiB default", cfg.HTTP.MaxBodyBytes)
	}

	cfg, err = httpConfig(t, map[string]string{"HTTP_MAX_BODY_BYTES": "52428800"})
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if cfg.HTTP.MaxBodyBytes != 50<<20 {
		t.Fatalf("MaxBodyBytes = %d, want the 50 MiB that was written", cfg.HTTP.MaxBodyBytes)
	}
}

// TestABodyLimitThatCannotBeUsedIsReportedAtBoot. Zero is refused rather than
// read as "no limit": there is no unbounded body to ask for.
func TestABodyLimitThatCannotBeUsedIsReportedAtBoot(t *testing.T) {
	for _, value := range []string{"0", "-1", "4MB", "lots"} {
		t.Run(value, func(t *testing.T) {
			_, err := httpConfig(t, map[string]string{"HTTP_MAX_BODY_BYTES": value})
			if err == nil {
				t.Fatalf("From accepted HTTP_MAX_BODY_BYTES=%q", value)
			}
			if !strings.Contains(err.Error(), "HTTP_MAX_BODY_BYTES") || !strings.Contains(err.Error(), `"`+value+`"`) {
				t.Errorf("error = %q, want it to name the variable and quote %q", err, value)
			}
		})
	}
}

// TestNoProxyIsTrustedUnlessOneIsListed. An empty list is what makes the
// forwarded headers inert, and it has to be what a deployment gets by writing
// nothing.
func TestNoProxyIsTrustedUnlessOneIsListed(t *testing.T) {
	cfg, err := httpConfig(t, nil)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if len(cfg.HTTP.TrustedProxies) != 0 {
		t.Fatalf("TrustedProxies = %v with TRUSTED_PROXIES unset, want none", cfg.HTTP.TrustedProxies)
	}
}

func TestTrustedProxiesAreReadAsPrefixes(t *testing.T) {
	cfg, err := httpConfig(t, map[string]string{
		"TRUSTED_PROXIES": " 10.0.0.0/8, 192.0.2.7 ,fd00::/8,2001:db8::1,172.16.5.9/12",
	})
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.0.2.7/32"),
		netip.MustParsePrefix("fd00::/8"),
		netip.MustParsePrefix("2001:db8::1/128"),
		netip.MustParsePrefix("172.16.0.0/12"),
	}
	if !slices.Equal(cfg.HTTP.TrustedProxies, want) {
		t.Fatalf("TrustedProxies = %v, want %v", cfg.HTTP.TrustedProxies, want)
	}
}

// TestATrustedProxyListThatCannotBeUsedIsReportedAtBoot. The wildcard is
// refused with the malformed entries: trusting every peer is trusting every
// client to say which address it is throttled under.
func TestATrustedProxyListThatCannotBeUsedIsReportedAtBoot(t *testing.T) {
	for _, value := range []string{"*", "10.0.0.0/8,*", "10.0.0.0/33", "proxy.internal", "10.0.0"} {
		t.Run(value, func(t *testing.T) {
			_, err := httpConfig(t, map[string]string{"TRUSTED_PROXIES": value})
			if err == nil {
				t.Fatalf("From accepted TRUSTED_PROXIES=%q", value)
			}
			if !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
				t.Errorf("error = %q, want it to name the variable", err)
			}
		})
	}
}
