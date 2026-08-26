package config

import (
	"fmt"
	"log/slog"

	"github.com/arandu-io/framework/foundation/bootstrap"
	hconfig "github.com/arandu-io/hesape/config"
)

// Logging is what is written to stdout, and how much of it.
//
// There is one channel and it is stdout. A stack of channels exists where a
// process cannot be relied on to have a collector in front of it; a container
// always does, and a log file inside a container is a log nobody
// reads and a disk that fills up. storage/logs does not exist here.
type Logging struct {
	// Level is the lowest severity written. Debug is refused in production by
	// the framework: it records request payloads, which is a data leak into the
	// log aggregator.
	Level slog.Level

	// Format is json or text. Text is for a terminal, json for everything that
	// parses.
	Format string

	// Sampling drops this fraction of info-level records under load, expressed
	// as one in N. Zero keeps all of them.
	Sampling int
}

func loadLogging(base bootstrap.Configuration) (Logging, error) {
	format := env("LOG_FORMAT", "json")
	if base.App.Env.Is(hconfig.EnvDev) {
		format = env("LOG_FORMAT", "text")
	}
	switch format {
	case "json", "text":
	default:
		return Logging{}, fmt.Errorf("LOG_FORMAT has unsupported value %q; expected json or text", format)
	}
	sampling, err := envInt("LOG_SAMPLING", 0)
	if err != nil {
		return Logging{}, err
	}
	return Logging{
		Level:    base.Observability.LogLevel,
		Format:   format,
		Sampling: sampling,
	}, nil
}
