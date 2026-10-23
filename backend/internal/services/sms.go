package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ashgabat-taxi/backend/internal/config"
)

// SMSSender delivers a text message to a phone number.
type SMSSender interface {
	Send(phone, text string) error
}

// LogSender prints the message to the console (development).
type LogSender struct{}

// Send implements SMSSender.
func (LogSender) Send(phone, text string) error {
	log.Printf("SMS -> %s: %s", phone, text)
	return nil
}

// HTTPSender posts {"phone","message"} as JSON to a gateway URL with a Bearer token.
// Adapt this to the SMS provider you contract in Turkmenistan (e.g. TM Cell / Altyn Asyr gateway).
type HTTPSender struct {
	URL   string
	Token string
	cli   *http.Client
}

// Send implements SMSSender.
func (s *HTTPSender) Send(phone, text string) error {
	body, _ := json.Marshal(map[string]string{"phone": phone, "message": text})
	req, err := http.NewRequest(http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	if s.cli == nil {
		s.cli = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := s.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sms gateway status %d", resp.StatusCode)
	}
	return nil
}

// NewSMSSender picks the provider from config.
func NewSMSSender(cfg *config.Config) SMSSender {
	if cfg.SMSProvider == "smpp" {
		if cfg.SMPPUser == "" || strings.HasPrefix(cfg.SMPPAddr, ":") {
			log.Fatal("SMS_PROVIDER=smpp: SMPP_IP, SMPP_PORT, SMPP_USERNAME we SMPP_PASSWORD gerek")
		}
		return NewSMPPSender(cfg)
	}
	if cfg.SMSProvider == "http" && cfg.SMSHTTPURL != "" {
		return &HTTPSender{URL: cfg.SMSHTTPURL, Token: cfg.SMSHTTPToken}
	}
	return LogSender{}
}
