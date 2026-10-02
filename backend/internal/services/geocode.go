package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ashgabat-taxi/backend/internal/config"
)

// Place is a geocoding result.
type Place struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

// Geocoder wraps Nominatim (OpenStreetMap) with a small in-memory cache.
type Geocoder struct {
	cfg   *config.Config
	cli   *http.Client
	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	val any
	exp time.Time
}

// NewGeocoder builds the geocoder.
func NewGeocoder(cfg *config.Config) *Geocoder {
	return &Geocoder{cfg: cfg, cli: &http.Client{Timeout: 6 * time.Second}, cache: make(map[string]cacheEntry)}
}

func (g *Geocoder) get(key string) (any, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.cache[key]
	if !ok || time.Now().After(e.exp) {
		return nil, false
	}
	return e.val, true
}

func (g *Geocoder) put(key string, v any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.cache) > 5000 {
		g.cache = make(map[string]cacheEntry)
	}
	g.cache[key] = cacheEntry{val: v, exp: time.Now().Add(24 * time.Hour)}
}

func (g *Geocoder) do(u string, out any) error {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AshgabatTaxi/1.0 (support@ashgabat-taxi.tm)")
	req.Header.Set("Accept-Language", "tk,ru,en")
	resp, err := g.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("nominatim status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type nominatimResult struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Name        string `json:"name"`
	Address     struct {
		Road         string `json:"road"`
		HouseNumber  string `json:"house_number"`
		Suburb       string `json:"suburb"`
		Neighbourhood string `json:"neighbourhood"`
		City         string `json:"city"`
	} `json:"address"`
}

func (r nominatimResult) short() string {
	parts := []string{}
	if r.Address.Road != "" {
		s := r.Address.Road
		if r.Address.HouseNumber != "" {
			s += " " + r.Address.HouseNumber
		}
		parts = append(parts, s)
	}
	if r.Address.Suburb != "" {
		parts = append(parts, r.Address.Suburb)
	} else if r.Address.Neighbourhood != "" {
		parts = append(parts, r.Address.Neighbourhood)
	}
	if len(parts) == 0 {
		if r.Name != "" {
			return r.Name
		}
		if i := strings.Index(r.DisplayName, ","); i > 0 && i < 60 {
			return r.DisplayName[:i]
		}
		return r.DisplayName
	}
	return strings.Join(parts, ", ")
}

// Reverse returns a human address for coordinates.
func (g *Geocoder) Reverse(lat, lng float64) (Place, error) {
	key := fmt.Sprintf("r:%.4f:%.4f", lat, lng)
	if v, ok := g.get(key); ok {
		return v.(Place), nil
	}
	u := fmt.Sprintf("%s/reverse?format=jsonv2&lat=%f&lon=%f&zoom=18&addressdetails=1", g.cfg.NominatimURL, lat, lng)
	var r nominatimResult
	if err := g.do(u, &r); err != nil {
		coord := fmt.Sprintf("%.5f, %.5f", lat, lng)
		return Place{Lat: lat, Lng: lng, Name: coord, Address: coord}, err
	}
	p := Place{Name: r.short(), Address: r.DisplayName, Lat: lat, Lng: lng}
	if p.Name == "" {
		p.Name = fmt.Sprintf("%.5f, %.5f", lat, lng)
	}
	g.put(key, p)
	return p, nil
}

// Search finds places by free text, biased to the city.
func (g *Geocoder) Search(q string) ([]Place, error) {
	q = strings.TrimSpace(q)
	key := "s:" + strings.ToLower(q)
	if v, ok := g.get(key); ok {
		return v.([]Place), nil
	}
	// viewbox around Ashgabat: left,top,right,bottom
	vb := fmt.Sprintf("%f,%f,%f,%f", g.cfg.CityLng-0.35, g.cfg.CityLat+0.2, g.cfg.CityLng+0.35, g.cfg.CityLat-0.2)
	u := fmt.Sprintf("%s/search?format=jsonv2&q=%s&limit=8&addressdetails=1&countrycodes=tm&viewbox=%s&bounded=1",
		g.cfg.NominatimURL, url.QueryEscape(q), vb)
	var rs []nominatimResult
	if err := g.do(u, &rs); err != nil {
		return nil, err
	}
	out := make([]Place, 0, len(rs))
	for _, r := range rs {
		var lat, lng float64
		fmt.Sscanf(r.Lat, "%f", &lat)
		fmt.Sscanf(r.Lon, "%f", &lng)
		out = append(out, Place{Name: r.short(), Address: r.DisplayName, Lat: lat, Lng: lng})
	}
	g.put(key, out)
	return out, nil
}
