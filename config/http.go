package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// DefaultMaxBodyBytes is the request body limit when HTTP_MAX_BODY_BYTES is not
// written: four mebibytes.
//
// It is sized for forms -- a sign-in, a settings page, a long text field -- with
// room for a small attachment. It is not sized for uploads, and that is the
// point: a body nobody bounded is one POST away from taking the process down.
const DefaultMaxBodyBytes int64 = 4 << 20

// HTTP is how the server treats a request before any handler sees it: how much
// of a body it will read, and which peers may say where a request came from.
type HTTP struct {
	// MaxBodyBytes is the largest request body the application reads, in bytes.
	//
	// It is a ceiling for every route. A request that declares a larger body is
	// answered 413 before the session, the CSRF check or the handler does any
	// work, and a body that does not declare its length is cut off at the limit.
	//
	// Because it is a ceiling, a route cannot raise it: an upload route that
	// takes more needs this value raised to the largest upload the application
	// accepts. The form routes can then keep a tighter bound of their own with
	// middleware.LimitBodySize from hesape/http/middleware on their group.
	MaxBodyBytes int64

	// TrustedProxies are the peers whose X-Forwarded-For and X-Forwarded-Proto
	// are believed.
	//
	// Empty is the default and means nobody is: a process listening on the
	// internet directly must not let a client name its own address, because the
	// sign-in throttle and every rate limit key on that address. Behind a load
	// balancer, list the balancer's addresses -- otherwise every request arrives
	// from the balancer, every visitor shares one budget, and the first person to
	// be throttled throttles everybody.
	TrustedProxies []netip.Prefix
}

func loadHTTP() (HTTP, error) {
	limit, err := envBodyBytes("HTTP_MAX_BODY_BYTES", DefaultMaxBodyBytes)
	if err != nil {
		return HTTP{}, err
	}
	proxies, err := parseTrustedProxies(env("TRUSTED_PROXIES", ""))
	if err != nil {
		return HTTP{}, err
	}
	return HTTP{MaxBodyBytes: limit, TrustedProxies: proxies}, nil
}

// envBodyBytes reads a size in bytes. Zero and a negative are refused rather
// than read as "no limit": there is no unbounded body to ask for, and leaving
// the variable out is how the default is asked for.
func envBodyBytes(key string, fallback int64) (int64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid integer value %q: %w", key, v, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be a positive number of bytes, got %q; leave it unset to keep the default", key, v)
	}
	return n, nil
}

// parseTrustedProxies reads a comma-separated list of CIDR prefixes. A bare
// address is accepted and trusted alone, as a /32 or a /128.
//
// A wildcard is refused. Trusting every peer is trusting every client to say
// who it is, which is the failure the list exists to prevent; a deployment that
// truly sits behind a proxy on every address writes 0.0.0.0/0,::/0 and owns
// that sentence.
func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "*" || part == "**" {
			return nil, fmt.Errorf("TRUSTED_PROXIES does not take %q: list the proxies' addresses or CIDR prefixes, because trusting every peer lets any client choose the address it is throttled under", part)
		}
		if prefix, err := netip.ParsePrefix(part); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES has invalid entry %q: want an address or a CIDR prefix such as 10.0.0.0/8", part)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}
