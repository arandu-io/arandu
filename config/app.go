// Package config is this application's configuration: what the framework reads
// for every application -- bootstrap.Settings, one typed section per driver --
// and the sections that are this application's own, beside it.
//
// There is no config("app.name") lookup. Every setting is a field of a typed
// struct, so a wrong key is a compile error instead of a nil that surfaces on
// the first request that happens to need it.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/hesape/geo"
)

// Config is the whole configuration of this application: the framework's
// sections, embedded, so cfg.Cache, cfg.Mail and cfg.App read as fields, and
// this application's own.
type Config struct {
	bootstrap.Settings

	Auth     Auth
	Geo      geo.Config
	Services Services
}

// Load reads the environment and returns the whole configuration, or the first
// error. It fails at boot, never on a request.
func Load() (Config, error) {
	settings, err := bootstrap.LoadSettings()
	if err != nil {
		return Config{}, err
	}
	return From(settings)
}

// From adds this application's sections to settings the framework already
// read. It is separate from Load so a test can supply the settings directly.
func From(settings bootstrap.Settings) (Config, error) {
	auth, err := loadAuth()
	if err != nil {
		return Config{}, err
	}
	geoConfig, err := loadGeo(settings.App)
	if err != nil {
		return Config{}, err
	}
	services, err := loadServices()
	if err != nil {
		return Config{}, err
	}
	return Config{Settings: settings, Auth: auth, Geo: geoConfig, Services: services}, nil
}

// The readers below are the only place this package touches the environment.

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

// envSeconds reads a duration expressed in seconds.
func envSeconds(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive number of seconds, got %q", key, v)
	}
	return time.Duration(n) * time.Second, nil
}
