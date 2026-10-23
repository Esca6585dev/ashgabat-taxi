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

	SMSProvider  string // log | http | smpp (default: smpp when SMPP_IP is set, else log)
	SMSHTTPURL   string
	SMSHTTPToken string

	// SMPP 3.4 operator connection (SMS_PROVIDER=smpp)
	SMPPAddr       string // SMPP_IP:SMPP_PORT
	SMPPUser       string
	SMPPPassword   string
	SMPPFrom       string // sender: short number or alpha name
	SMPPSystemType string
	SMPPDstFormat  string // 993 | plus | local | 8 — how the SMSC wants the recipient number
	SMPPSrcTON     int
	SMPPSrcNPI     int
	SMPPDstTON     int
	SMPPDstNPI     int

	// Firebase Cloud Messaging service-account key (file path or the JSON itself); empty => push disabled
	FCMCredentialsFile string
	FCMCredentialsJSON string

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

	// Driver payment plans and card top-up
	DefaultDriverPlan   string  // commission | monthly — plan of newly registered drivers
	MonthlyFee          float64 // default fee of the monthly plan (per 30 days)
	DriverMinBalance    float64 // commission plan: below this balance the driver cannot take orders
	PaymentProvider     string  // "" (off) | demo | rbs
	PaymentCurrencyCode string  // ISO 4217 numeric, TMT = 934
	RBSURL              string
	RBSUser             string
	RBSPassword         string
	RBSLanguage         string
	TopUpMin            float64
	TopUpMax            float64
	PublicURL           string // https://api.example.tm — for bank return links; empty = from the request
	Currency            string
	CityLat             float64
	CityLng             float64
	SupportPhone        string

	AdminDir string
	SiteDir  string

	// Flutter web builds served at /app/ (client) and /driver/ (driver); skipped when missing.
	ClientWebDir string
	DriverWebDir string
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
	smppIP := env("SMPP_IP", "")
	smsProvider := env("SMS_PROVIDER", "")
	if smsProvider == "" {
		smsProvider = "log"
		if smppIP != "" {
			smsProvider = "smpp"
		}
	}
	appEnv := env("APP_ENV", "development")
	// card top-up: the demo bank only outside production unless chosen explicitly
	paymentProvider := env("PAYMENT_PROVIDER", "")
	if paymentProvider == "" && appEnv != "production" {
		paymentProvider = "demo"
	}
	if paymentProvider == "none" {
		paymentProvider = ""
	}
	dstFormat := env("SMPP_DST_FORMAT", "993")
	dstTON, dstNPI := 1, 1 // international / ISDN
	if dstFormat == "local" || dstFormat == "8" {
		dstTON = 0
	}
	return &Config{
		Port:           env("PORT", "8080"),
		Env:            appEnv,
		JWTSecret:      env("JWT_SECRET", "change-me-in-production"),
		JWTExpiryHours: envInt("JWT_EXPIRY_HOURS", 24*30),

		DBDriver: env("DB_DRIVER", "sqlite"),
		DBDSN:    env("DB_DSN", "taxi.db"),

		AdminPhone:    env("ADMIN_PHONE", "+99360000000"),
		AdminPassword: env("ADMIN_PASSWORD", "admin12345"),

		SMSProvider:  smsProvider,
		SMSHTTPURL:   env("SMS_HTTP_URL", ""),
		SMSHTTPToken: env("SMS_HTTP_TOKEN", ""),

		SMPPAddr:       smppIP + ":" + env("SMPP_PORT", "2775"),
		SMPPUser:       env("SMPP_USERNAME", ""),
		SMPPPassword:   env("SMPP_PASSWORD", ""),
		SMPPFrom:       env("SMPP_FROM", ""),
		SMPPSystemType: env("SMPP_SYSTEM_TYPE", ""),
		SMPPDstFormat:  dstFormat,
		SMPPSrcTON:     envInt("SMPP_SRC_TON", 0),
		SMPPSrcNPI:     envInt("SMPP_SRC_NPI", 0),
		SMPPDstTON:     envInt("SMPP_DST_TON", dstTON),
		SMPPDstNPI:     envInt("SMPP_DST_NPI", dstNPI),

		FCMCredentialsFile: env("FCM_CREDENTIALS_FILE", ""),
		FCMCredentialsJSON: env("FCM_CREDENTIALS_JSON", ""),

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

		DefaultDriverPlan:   env("DEFAULT_DRIVER_PLAN", "commission"),
		MonthlyFee:          envFloat("MONTHLY_FEE", 300),
		DriverMinBalance:    envFloat("DRIVER_MIN_BALANCE", -50),
		PaymentProvider:     paymentProvider,
		PaymentCurrencyCode: env("PAYMENT_CURRENCY_CODE", "934"),
		RBSURL:              env("PAYMENT_RBS_URL", "https://mpi.gov.tm/payment/rest"),
		RBSUser:             env("PAYMENT_RBS_USERNAME", ""),
		RBSPassword:         env("PAYMENT_RBS_PASSWORD", ""),
		RBSLanguage:         env("PAYMENT_RBS_LANGUAGE", "ru"),
		TopUpMin:            envFloat("TOPUP_MIN", 5),
		TopUpMax:            envFloat("TOPUP_MAX", 5000),
		PublicURL:           env("PUBLIC_URL", ""),
		Currency:            env("CURRENCY", "TMT"),
		CityLat:             envFloat("CITY_LAT", 37.9601),
		CityLng:             envFloat("CITY_LNG", 58.3261),
		SupportPhone:        env("SUPPORT_PHONE", "+99312000000"),

		AdminDir: env("ADMIN_DIR", "./web/admin"),
		SiteDir:  env("SITE_DIR", "../website"),

		ClientWebDir: env("CLIENT_WEB_DIR", "../apps/client_app/build/web"),
		DriverWebDir: env("DRIVER_WEB_DIR", "../apps/driver_app/build/web"),
	}
}

// IsDev reports whether the server runs in development mode.
func (c *Config) IsDev() bool { return c.Env != "production" }
