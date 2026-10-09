package unit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/framework/geo"
	hgeo "github.com/arandu-io/hesape/geo"
)

func TestGeoDefaultsToAllSurfacesWithIndexingClosed(t *testing.T) {
	cfg, err := loadConfigurationWith(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Geo.Enabled || cfg.Geo.Indexing || cfg.Geo.Surfaces != hgeo.AllSurfaces {
		t.Fatalf("GEO defaults = enabled %t indexing %t surfaces %d", cfg.Geo.Enabled, cfg.Geo.Indexing, cfg.Geo.Surfaces)
	}
	module := geo.NewModule(cfg.Geo, nil)
	if err := module.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGeoSurfacesAreIndependentlySelectable(t *testing.T) {
	cfg, err := loadConfigurationWith(t, map[string]string{"GEO_SURFACES": "robots,llms"})
	if err != nil {
		t.Fatal(err)
	}
	want := hgeo.Robots.AsSet() | hgeo.LLMs.AsSet()
	if cfg.Geo.Surfaces != want {
		t.Fatalf("GEO surfaces = %d, want %d", cfg.Geo.Surfaces, want)
	}
}

func TestGeoRejectsUnknownSurfacesAndContradictoryFlags(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"unknown surface":                {"GEO_SURFACES": "robots,answers"},
		"indexing while module disabled": {"GEO_ENABLED": "false", "GEO_INDEXING_ENABLED": "true"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadConfigurationWith(t, values)
			if err == nil || (!strings.Contains(err.Error(), "GEO_SURFACES") && !strings.Contains(err.Error(), "GEO_INDEXING_ENABLED")) {
				t.Fatalf("configuration error = %v", err)
			}
		})
	}
}

func TestGeoIndexingRequiresASafePublicOriginAtBoot(t *testing.T) {
	cfg, err := loadConfigurationWith(t, map[string]string{
		"APP_ENV": "prod", "APP_URL": "https://app.example.test", "GEO_INDEXING_ENABLED": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Geo.Indexing {
		t.Fatal("explicit production indexing was not preserved")
	}
	if err := geo.NewModule(cfg.Geo, nil).Boot(context.Background()); err != nil {
		t.Fatalf("safe production GEO config: %v", err)
	}

	unsafe, err := loadConfigurationWith(t, map[string]string{
		"APP_ENV": "prod", "APP_URL": "http://app.example.test", "GEO_INDEXING_ENABLED": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := geo.NewModule(unsafe.Geo, nil).Boot(context.Background()); err == nil {
		t.Fatal("plain HTTP origin enabled public GEO indexing")
	}
}
