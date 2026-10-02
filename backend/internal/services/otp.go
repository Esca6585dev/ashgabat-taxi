// Package services holds business logic: OTP, pricing, routing, geocoding, dispatch.
package services

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
)

// OTPService issues and verifies SMS login codes with simple rate limiting.
type OTPService struct {
	db     *gorm.DB
	cfg    *config.Config
	sender SMSSender

	mu       sync.Mutex
	attempts map[string][]time.Time // phone -> send timestamps (last 10 min)
}

// NewOTPService wires the service.
func NewOTPService(db *gorm.DB, cfg *config.Config, sender SMSSender) *OTPService {
	return &OTPService{db: db, cfg: cfg, sender: sender, attempts: make(map[string][]time.Time)}
}

// ErrTooMany is returned when a phone requests too many codes.
var ErrTooMany = errors.New("gaty köp synanyşyk, 10 minutdan soň gaýtadan synanyşyň")

func (s *OTPService) allowed(phone string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	kept := s.attempts[phone][:0]
	for _, t := range s.attempts[phone] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 5 {
		s.attempts[phone] = kept
		return false
	}
	s.attempts[phone] = append(kept, time.Now())
	return true
}

func randomCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%04d", n.Int64())
}

// Send generates a code, stores it and sends the SMS. The code is returned so
// development builds can surface it in the API response.
func (s *OTPService) Send(phone string) (string, error) {
	if !s.allowed(phone) {
		return "", ErrTooMany
	}
	code := s.cfg.OTPFixedCode
	if code == "" {
		code = randomCode()
	}
	s.db.Model(&models.OTPCode{}).Where("phone = ? AND used = ?", phone, false).Update("used", true)
	rec := models.OTPCode{Phone: phone, Code: code, ExpiresAt: time.Now().Add(time.Duration(s.cfg.OTPTTLMin) * time.Minute)}
	if err := s.db.Create(&rec).Error; err != nil {
		return "", err
	}
	text := fmt.Sprintf("Ashgabat Taxi: tassyklama kody %s. Kody hiç kime aýtmaň.", code)
	if err := s.sender.Send(phone, text); err != nil {
		return "", err
	}
	return code, nil
}

// Verify checks the code and marks it used.
func (s *OTPService) Verify(phone, code string) bool {
	var rec models.OTPCode
	err := s.db.Where("phone = ? AND code = ? AND used = ? AND expires_at > ?", phone, code, false, time.Now()).
		Order("id desc").First(&rec).Error
	if err != nil {
		return false
	}
	s.db.Model(&rec).Update("used", true)
	return true
}
