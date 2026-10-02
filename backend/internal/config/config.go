// Package config loads runtime configuration from environment variables.
package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds every tunable of the Ashgabat Taxi backend.
type Config struct {
	Port           string
	Env            string
	JWTSecret      string
	JWTExpiryHours int

	DBDriver string // sqlite | postgres
	DBDSN    string

	AdminPhone    string
	AdminPassword string

	SMSProvider  string // log | http
	SMSHTTPURL   string
	SMSHTTPToken string
	OTPFixedCode string // non-empty => every OTP equals this (dev/test only)
	OTPTTLMin    int

	OSRMURL      string // e.g. https://router.project-osrm.org (empty => haversine estimate)
	NominatimURL string

	DispatchRadiusKm    float64
	DispatchMaxRadiusKm float64
	DispatchTimeoutSec  int
	DispatchBatch       int
	DriverOfflineSec    int

	CommissionRate float64
	Currency       string
	CityLat        float64
	CityLng        float64
	SupportPhone   string

	AdminDir string
	SiteDir  string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

// Load reads .env (if present) and the process environment.
func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		Port:           env("PORT", "8080"),
		Env:            env("APP_ENV", "development"),
		JWTSecret:      env("JWT_SECRET", "change-me-in-production"),
		JWTExpiryHours: envInt("JWT_EXPIRY_HOURS", 24*30),

		DBDriver: env("DB_DRIVER", "sqlite"),
		DBDSN:    env("DB_DSN", "taxi.db"),

		AdminPhone:    env("ADMIN_PHONE", "+99360000000"),
		AdminPassword: env("ADMIN_PASSWORD", "admin12345"),

		SMSProvider:  env("SMS_PROVIDER", "log"),
		SMSHTTPURL:   env("SMS_HTTP_URL", ""),
		SMSHTTPToken: env("SMS_HTTP_TOKEN", ""),
		OTPFixedCode: env("OTP_FIXED_CODE", ""),
		OTPTTLMin:    envInt("OTP_TTL_MIN", 5),

		OSRMURL:      env("OSRM_URL", ""),
		NominatimURL: env("NOMINATIM_URL", "https://nominatim.openstreetmap.org"),

		DispatchRadiusKm:    envFloat("DISPATCH_RADIUS_KM", 3),
		DispatchMaxRadiusKm: envFloat("DISPATCH_MAX_RADIUS_KM", 12),
		DispatchTimeoutSec:  envInt("DISPATCH_TIMEOUT_SEC", 300),
		DispatchBatch:       envInt("DISPATCH_BATCH", 10),
		DriverOfflineSec:    envInt("DRIVER_OFFLINE_SEC", 120),

		CommissionRate: envFloat("COMMISSION_RATE", 0.10),
		Currency:       env("CURRENCY", "TMT"),
		CityLat:        envFloat("CITY_LAT", 37.9601),
		CityLng:        envFloat("CITY_LNG", 58.3261),
		SupportPhone:   env("SUPPORT_PHONE", "+99312000000"),

		AdminDir: env("ADMIN_DIR", "./web/admin"),
		SiteDir:  env("SITE_DIR", "../website"),
	}
}

// IsDev reports whether the server runs in development mode.
func (c *Config) IsDev() bool { return c.Env != "production" }
