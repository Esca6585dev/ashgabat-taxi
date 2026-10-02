package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// LatLng is a WGS84 coordinate.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// RouteInfo is the result of a routing query.
type RouteInfo struct {
	DistanceKm  float64  `json:"distance_km"`
	DurationMin float64  `json:"duration_min"`
	Polyline    []LatLng `json:"polyline,omitempty"`
	Source      string   `json:"source"` // osrm | estimate
}

// Router computes driving routes. Uses OSRM when configured, otherwise a
// straight-line estimate multiplied by a city detour factor.
type Router struct {
	cfg *config.Config
	cli *http.Client
}

// NewRouter builds the router.
func NewRouter(cfg *config.Config) *Router {
	return &Router{cfg: cfg, cli: &http.Client{Timeout: 6 * time.Second}}
}

const (
	detourFactor   = 1.3  // real road distance ≈ 1.3 × straight line inside Ashgabat
	avgCitySpeedKm = 32.0 // km/h average incl. traffic lights
)

// Estimate returns the straight-line based estimate.
func (r *Router) Estimate(a, b LatLng) RouteInfo {
	d := utils.Haversine(a.Lat, a.Lng, b.Lat, b.Lng) * detourFactor
	return RouteInfo{DistanceKm: d, DurationMin: d / avgCitySpeedKm * 60, Polyline: []LatLng{a, b}, Source: "estimate"}
}

// Route queries OSRM (if configured) and falls back to Estimate.
func (r *Router) Route(a, b LatLng) RouteInfo {
	if r.cfg.OSRMURL == "" {
		return r.Estimate(a, b)
	}
	url := fmt.Sprintf("%s/route/v1/driving/%f,%f;%f,%f?overview=full&geometries=geojson", r.cfg.OSRMURL, a.Lng, a.Lat, b.Lng, b.Lat)
	resp, err := r.cli.Get(url)
	if err != nil {
		return r.Estimate(a, b)
	}
	defer resp.Body.Close()
	var out struct {
		Code   string `json:"code"`
		Routes []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Code != "Ok" || len(out.Routes) == 0 {
		return r.Estimate(a, b)
	}
	rt := out.Routes[0]
	info := RouteInfo{DistanceKm: rt.Distance / 1000, DurationMin: rt.Duration / 60, Source: "osrm"}
	for _, c := range rt.Geometry.Coordinates {
		if len(c) == 2 {
			info.Polyline = append(info.Polyline, LatLng{Lat: c[1], Lng: c[0]})
		}
	}
	return info
}
