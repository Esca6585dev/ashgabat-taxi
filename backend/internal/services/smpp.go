package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutext"

	"github.com/ashgabat-taxi/backend/internal/config"
)

// SMPPSender sends SMS through an operator SMSC over SMPP 3.4.
// One transmitter bind is kept open; go-smpp re-binds automatically when the link drops.
type SMPPSender struct {
	cfg *config.Config
	tx  *smpp.Transmitter

	mu        sync.Mutex
	connected bool
	lastErr   error
	settled   chan struct{} // closed on the first bind result, success or failure
	once      sync.Once
}

// ErrSMPPNotConnected is returned when the SMSC bind is not up.
var ErrSMPPNotConnected = errors.New("smpp: SMS merkezine birikip bolmady")

// NewSMPPSender binds to the SMSC in the background.
func NewSMPPSender(cfg *config.Config) *SMPPSender {
	s := &SMPPSender{
		cfg:     cfg,
		settled: make(chan struct{}),
		tx: &smpp.Transmitter{
			Addr:        cfg.SMPPAddr,
			User:        cfg.SMPPUser,
			Passwd:      cfg.SMPPPassword,
			SystemType:  cfg.SMPPSystemType,
			RespTimeout: 10 * time.Second,
			// retry the bind every 10s while the SMSC is unreachable
			BindInterval: 10 * time.Second,
		},
	}
	go s.watch(s.tx.Bind())
	return s
}

func (s *SMPPSender) watch(st <-chan smpp.ConnStatus) {
	for c := range st {
		s.mu.Lock()
		s.connected = c.Status() == smpp.Connected
		s.lastErr = c.Error()
		s.mu.Unlock()
		s.once.Do(func() { close(s.settled) })
		if c.Status() == smpp.Connected {
			log.Printf("smpp: bound to %s as %s", s.cfg.SMPPAddr, s.cfg.SMPPUser)
		} else {
			log.Printf("smpp: %s (%v)", c.Status(), c.Error())
		}
	}
}

// Connected reports whether the bind is currently up.
func (s *SMPPSender) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

// Close unbinds from the SMSC.
func (s *SMPPSender) Close() error { return s.tx.Close() }

// Send implements SMSSender.
func (s *SMPPSender) Send(phone, text string) error {
	// right after start-up the bind may still be in progress
	select {
	case <-s.settled:
	case <-time.After(10 * time.Second):
	}
	if !s.Connected() {
		s.mu.Lock()
		err := s.lastErr
		s.mu.Unlock()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrSMPPNotConnected, err)
		}
		return ErrSMPPNotConnected
	}
	dst := SMPPDestination(phone, s.cfg.SMPPDstFormat)
	sm := &smpp.ShortMessage{
		Src:           s.cfg.SMPPFrom,
		Dst:           dst,
		Text:          smppText(text),
		Validity:      time.Duration(s.cfg.OTPTTLMin) * time.Minute,
		Register:      pdufield.NoDeliveryReceipt,
		SourceAddrTON: uint8(s.cfg.SMPPSrcTON),
		SourceAddrNPI: uint8(s.cfg.SMPPSrcNPI),
		DestAddrTON:   uint8(s.cfg.SMPPDstTON),
		DestAddrNPI:   uint8(s.cfg.SMPPDstNPI),
	}
	if _, err := s.tx.Submit(sm); err != nil {
		return fmt.Errorf("smpp submit to %s: %w", dst, err)
	}
	log.Printf("smpp: sent to %s id=%s", dst, sm.RespID())
	return nil
}

// smppText picks Latin-1 for plain ASCII and UCS-2 when Turkmen letters (ä, ň, ý, ş...) are present.
func smppText(text string) pdutext.Codec {
	for _, r := range text {
		if r > unicode.MaxASCII {
			return pdutext.UCS2(text)
		}
	}
	return pdutext.Raw(text)
}

// SMPPDestination formats a normalized +993XXXXXXXX number for the SMSC:
// "993" -> 99365123456 (default), "plus" -> +99365123456, "local" -> 65123456, "8" -> 865123456.
func SMPPDestination(phone, format string) string {
	digits := strings.TrimPrefix(phone, "+")
	local := strings.TrimPrefix(digits, "993")
	switch format {
	case "plus":
		return "+" + digits
	case "local":
		return local
	case "8":
		return "8" + local
	default:
		return digits
	}
}
