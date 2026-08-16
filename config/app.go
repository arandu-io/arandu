// Package config is this application's configuration, one file per domain: ten
// files, named after what they configure.
//
// The difference is the one that matters: there is no config("app.name") lookup.
// Every setting is a field of a typed struct, so a wrong key is a compile error
// instead of a nil that surfaces on the first request that happens to need it.
//
// The framework parses what the kernel itself needs (APP_KEY, APP_ENV, the
// database block) and this package parses the rest. Config carries both, so
// there is one value to pass around and one place to look.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	framework "github.com/arandu-io/framework/config"
)

// Config is the whole configuration of this application: one field per file in
// this directory, plus what the framework parsed for the kernel.
type Config struct {
	// Framework is what kernel.New takes. It is not a copy of the fields below:
	// the kernel validates APP_KEY, APP_ENV and the connection at boot, and this
	// package never re-reads them.
	Framework framework.Config

	App         App
	Auth        Auth
	Cache       Cache
	Database    Database
	Filesystems Filesystems
	Logging     Logging
	Mail        Mail
	Queue       Queue
	Services    Services
	Session     Session
}

// App is the identity of the application.
type App struct {
	// Name appears in the page title, in the log and in outgoing mail.
	Name string
	// Env gates everything that must never run outside development.
	Env framework.Env
	// URL is the canonical address, used to build absolute links from a job or
	// a scheduled task, where there is no request to read the host from.
	URL string
	// HTTPAddr is what the server listens on.
	HTTPAddr string
	// Timezone is the zone dates are rendered in. Storage is always UTC.
	Timezone string
	// Locale is the default language tag.
	Locale string
}

// IsDev reports whether the debug surface is allowed to exist.
func (a App) IsDev() bool { return a.Env == framework.EnvDev }

// Load reads the environment and returns the whole configuration, or the first
// error. It fails at boot, never on a request.
func Load() (Config, error) {
	base, err := framework.Load()
	if err != nil {
		return Config{}, err
	}
	return From(base), nil
}

// From builds the application configuration on top of one the framework already
// parsed and validated.
//
// It is separate from Load so a test can supply a configuration without an
// environment: the test writes the framework part it cares about and gets the
// ten domains filled from their defaults.
func From(base framework.Config) Config {
	return Config{
		Framework:   base,
		App:         loadApp(base),
		Auth:        loadAuth(),
		Cache:       loadCache(base),
		Database:    loadDatabase(base),
		Filesystems: loadFilesystems(),
		Logging:     loadLogging(base),
		Mail:        loadMail(base),
		Queue:       loadQueue(),
		Services:    loadServices(),
		Session:     loadSession(base),
	}
}

func loadApp(base framework.Config) App {
	return App{
		Name:     base.AppName,
		Env:      base.Env,
		URL:      env("APP_URL", "http://localhost:8080"),
		HTTPAddr: base.HTTPAddr,
		Timezone: env("APP_TIMEZONE", "UTC"),
		Locale:   env("APP_LOCALE", "en"),
	}
}

// The readers below are the only place this package touches the environment.
// They are unexported for a reason: a setting read at the point of use is a
// setting nobody can list, and listing them is what this directory is for.

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// envSeconds reads a duration expressed in seconds, not in Go's duration
// syntax: these values are written by deployment tooling as often as by people,
// and "3600" travels through a Helm chart better than "1h".
func envSeconds(key string, fallback time.Duration) time.Duration {
	if n := envInt(key, -1); n > 0 {
		return time.Duration(n) * time.Second
	}
	return fallback
}
