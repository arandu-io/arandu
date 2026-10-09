package config

import (
	"fmt"
	"strings"

	"github.com/arandu-io/hesape/geo"
)

// loadGeo builds the native Search and Generative Engine Optimization module.
// The routes exist by default but publication is fail-closed until indexing is
// explicitly enabled for a deployment.
func loadGeo(app App) (geo.Config, error) {
	enabled, err := envBool("GEO_ENABLED", true)
	if err != nil {
		return geo.Config{}, err
	}
	indexing, err := envBool("GEO_INDEXING_ENABLED", false)
	if err != nil {
		return geo.Config{}, err
	}
	if indexing && !enabled {
		return geo.Config{}, fmt.Errorf("GEO_INDEXING_ENABLED requires GEO_ENABLED=true")
	}

	surfaces, err := geoSurfaces(env("GEO_SURFACES", "robots,sitemap,llms,llms-full"))
	if err != nil {
		return geo.Config{}, err
	}
	return geo.Config{
		Enabled: enabled, Indexing: indexing, Origin: app.URL, Surfaces: surfaces,
		Language: app.Locale, IndexTitle: app.Name,
		CorpusTitle: app.Name + " public content",
		Robots: geo.RobotsPolicy{
			Allow:    []string{"/", "/_arandu/assets/"},
			Disallow: []string{"/_arandu/", "/auth/", "/dashboard"},
		},
	}, nil
}

func geoSurfaces(value string) (geo.Surfaces, error) {
	var out geo.Surfaces
	for _, raw := range strings.Split(value, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		var surface geo.Surface
		switch name {
		case "robots":
			surface = geo.Robots
		case "sitemap":
			surface = geo.Sitemap
		case "llms":
			surface = geo.LLMs
		case "llms-full":
			surface = geo.LLMsFull
		default:
			return 0, fmt.Errorf("GEO_SURFACES contains unsupported surface %q", name)
		}
		out |= surface.AsSet()
	}
	if enabled := strings.TrimSpace(value) != ""; enabled && out == 0 {
		return 0, fmt.Errorf("GEO_SURFACES must name at least one surface")
	}
	return out, nil
}
