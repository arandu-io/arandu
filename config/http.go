package config

import (
	"fmt"
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

// HTTP is how the server treats a request before any handler sees it.
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
}

func loadHTTP() (HTTP, error) {
	limit, err := envBodyBytes("HTTP_MAX_BODY_BYTES", DefaultMaxBodyBytes)
	if err != nil {
		return HTTP{}, err
	}
	return HTTP{MaxBodyBytes: limit}, nil
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
