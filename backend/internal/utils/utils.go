// Package utils has small helpers shared by handlers and services.
package utils

import (
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// OK writes a success envelope.
func OK(c *fiber.Ctx, data any) error {
	return c.JSON(fiber.Map{"success": true, "data": data})
}

// Err writes an error envelope with the given HTTP status.
func Err(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"success": false, "error": msg})
}

var (
	nonDigit   = regexp.MustCompile(`\D`)
	phoneRegex = regexp.MustCompile(`^\+993\d{8}$`)
)

// NormalizePhone converts user input to the canonical +993XXXXXXXX form.
// Accepts "61234567", "861234567", "99361234567", "+993 61 23-45-67".
func NormalizePhone(raw string) string {
	d := nonDigit.ReplaceAllString(raw, "")
	switch {
	case len(d) == 8:
		d = "993" + d
	case len(d) == 9 && strings.HasPrefix(d, "8"):
		d = "993" + d[1:]
	}
	if d == "" {
		return ""
	}
	return "+" + d
}

// ValidPhone reports whether the normalized phone looks like a Turkmen mobile number.
func ValidPhone(p string) bool {
	return phoneRegex.MatchString(p)
}

// Haversine distance in kilometres between two coordinates.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// RoundMoney rounds to the nearest 0.5 manat.
func RoundMoney(v float64) float64 {
	return math.Round(v*2) / 2
}

// Now returns a pointer to the current time (for nullable timestamp columns).
func Now() *time.Time {
	t := time.Now()
	return &t
}

// Ptr returns a pointer to v.
func Ptr[T any](v T) *T { return &v }
