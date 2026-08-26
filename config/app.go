// Package config is this application's configuration, one file per domain: ten
// files, named after what they configure.
//
// The difference is the one that matters: there is no config("app.name") lookup.
// Every setting is a field of a typed struct, so a wrong key is a compile error
// instead of a nil that surfaces on the first request that happens to need it.
//
// The framework parses what the kernel itself needs -- APP_KEY, APP_ENV, the
// connection, one struct per component -- and this package parses the rest.
// Config carries both, so there is one value to pass around and one place to
// look.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
	hconfig "github.com/arandu-io/hesape/config"
)

// Config is the whole configuration of this application: one field per file in
// this directory, plus what the framework parsed for the kernel.
type Config struct {
	// Framework is what kernel.New takes: one struct per component, filled from
	// the environment once. It is not a copy of the fields below -- the kernel
	// validates APP_KEY, APP_ENV and the connection while it is filled, and this
	// package never re-reads them.
	Framework bootstrap.Configuration

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
	Env hconfig.Env
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
func (a App) IsDev() bool { return a.Env.Is(hconfig.EnvDev) }

// Load reads the environment and returns the whole configuration, or the first
// error. It fails at boot, never on a request.
func Load() (Config, error) {
	base, err := bootstrap.LoadConfiguration()
	if err != nil {
		return Config{}, err
	}
	return From(base)
}

// From builds the application configuration on top of one the framework already
// parsed. It validates that base and returns the first invalid application
// setting instead of constructing a partial configuration.
//
// It is separate from Load so a test can supply a valid framework configuration
// directly and get the ten application domains filled from their defaults.
func From(base bootstrap.Configuration) (Config, error) {
	if err := base.App.Validate(); err != nil {
		return Config{}, fmt.Errorf("framework application configuration: %w", err)
	}

	auth, err := loadAuth()
	if err != nil {
		return Config{}, err
	}
	cache, err := loadCache(base)
	if err != nil {
		return Config{}, err
	}
	database, err := loadDatabase(base)
	if err != nil {
		return Config{}, err
	}
	filesystems, err := loadFilesystems()
	if err != nil {
		return Config{}, err
	}
	logging, err := loadLogging(base)
	if err != nil {
		return Config{}, err
	}
	mail, err := loadMail(base)
	if err != nil {
		return Config{}, err
	}
	queue, err := loadQueue()
	if err != nil {
		return Config{}, err
	}
	session, err := loadSession(base)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Framework:   base,
		App:         loadApp(base),
		Auth:        auth,
		Cache:       cache,
		Database:    database,
		Filesystems: filesystems,
		Logging:     logging,
		Mail:        mail,
		Queue:       queue,
		Services:    loadServices(),
		Session:     session,
	}

	return cfg, nil
}

func loadApp(base bootstrap.Configuration) App {
	// The address and the zone are rendered from what was already parsed, not
	// read from APP_URL and APP_TIMEZONE a second time: two readers of one
	// variable are two answers the day one of them grows a default the other
	// does not have.
	//
	// A nil address is what a configuration built by hand in a test leaves
	// behind. On a real boot it never happens -- App.Validate refuses it.
	address := ""
	if base.App.URL != nil {
		address = base.App.URL.String()
	}

	return App{
		Name:     base.App.Name,
		Env:      base.App.Env,
		URL:      address,
		HTTPAddr: base.App.HTTPAddr,
		// A nil location renders as UTC, which is the default this read had.
		Timezone: base.App.Timezone.String(),
		Locale:   base.App.Locale,
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

func envBool(key string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s has invalid boolean value %q", key, v)
	}
}

func envInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid integer value %q: %w", key, v, err)
	}
	return n, nil
}

// envSeconds reads a duration expressed in seconds, not in Go's duration
// syntax: these values are written by deployment tooling as often as by people,
// and "3600" travels through a Helm chart better than "1h".
func envSeconds(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := envInt(key, 0)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be a positive number of seconds, got %q", key, v)
	}
	return time.Duration(n) * time.Second, nil
}
