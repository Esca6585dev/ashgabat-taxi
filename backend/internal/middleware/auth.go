// Package middleware contains Fiber middlewares (JWT auth, roles).
package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// SignToken creates a JWT for the user.
func SignToken(cfg *config.Config, u *models.User) (string, error) {
	claims := jwt.MapClaims{
		"uid":  u.ID,
		"role": string(u.Role),
		"exp":  time.Now().Add(time.Duration(cfg.JWTExpiryHours) * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
}

// ParseToken validates a JWT and returns (userID, role).
func ParseToken(cfg *config.Config, tokenStr string) (uint, string, bool) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || !tok.Valid {
		return 0, "", false
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", false
	}
	uid, _ := claims["uid"].(float64)
	role, _ := claims["role"].(string)
	if uid == 0 {
		return 0, "", false
	}
	return uint(uid), role, true
}

// Protect requires a valid Bearer token (or ?token= for websockets).
func Protect(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenStr := ""
		if h := c.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tokenStr = strings.TrimPrefix(h, "Bearer ")
		} else if q := c.Query("token"); q != "" {
			tokenStr = q
		}
		if tokenStr == "" {
			return utils.Err(c, fiber.StatusUnauthorized, "token gerek")
		}
		uid, role, ok := ParseToken(cfg, tokenStr)
		if !ok {
			return utils.Err(c, fiber.StatusUnauthorized, "token nädogry ýa-da möhleti geçen")
		}
		c.Locals("uid", uid)
		c.Locals("role", role)
		return c.Next()
	}
}

// RequireRole allows only the given roles.
func RequireRole(roles ...models.Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		for _, r := range roles {
			if string(r) == role {
				return c.Next()
			}
		}
		return utils.Err(c, fiber.StatusForbidden, "rugsat ýok")
	}
}

// UID returns the authenticated user id from context.
func UID(c *fiber.Ctx) uint {
	v, _ := c.Locals("uid").(uint)
	return v
}
