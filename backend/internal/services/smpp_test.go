package services

import (
	"strings"
	"testing"
	"time"

	"github.com/fiorix/go-smpp/smpp/pdu"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutext"
	"github.com/fiorix/go-smpp/smpp/smpptest"

	"github.com/ashgabat-taxi/backend/internal/config"
)

func TestSMPPDestination(t *testing.T) {
	cases := map[string]string{"993": "99365123456", "": "99365123456", "plus": "+99365123456", "local": "65123456", "8": "865123456"}
	for format, want := range cases {
		if got := SMPPDestination("+99365123456", format); got != want {
			t.Errorf("format %q: got %s want %s", format, got, want)
		}
	}
}

func TestSMPPSenderSubmitsOTP(t *testing.T) {
	got := make(chan pdufield.Map, 1)
	srv := smpptest.NewUnstartedServer()
	srv.Handler = func(c smpptest.Conn, p pdu.Body) {
		if p.Header().ID != pdu.SubmitSMID {
			smpptest.EchoHandler(c, p)
			return
		}
		got <- p.Fields()
		r := pdu.NewSubmitSMResp()
		r.Header().Seq = p.Header().Seq
		_ = r.Fields().Set(pdufield.MessageID, "msg-1")
		_ = c.Write(r)
	}
	srv.Start()
	defer srv.Close()

	cfg := &config.Config{
		SMPPAddr: srv.Addr(), SMPPUser: smpptest.DefaultUser, SMPPPassword: smpptest.DefaultPasswd,
		SMPPFrom: "0796", SMPPDstFormat: "993", SMPPDstTON: 1, SMPPDstNPI: 1, OTPTTLMin: 5,
	}
	s := NewSMPPSender(cfg)
	defer s.Close()

	text := "Ashgabat Taxi: tassyklama kody 4321. Kody hiç kime aýtmaň."
	if err := s.Send("+99365123456", text); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case f := <-got:
		if f[pdufield.DestinationAddr].String() != "99365123456" {
			t.Errorf("dst = %s", f[pdufield.DestinationAddr])
		}
		if f[pdufield.SourceAddr].String() != "0796" {
			t.Errorf("src = %s", f[pdufield.SourceAddr])
		}
		if dc := f[pdufield.DataCoding].Raw().(uint8); dc != uint8(pdutext.UCS2Type) {
			t.Errorf("Turkmen text must go as UCS-2, data_coding = %d", dc)
		}
		body := string(pdutext.UCS2(f[pdufield.ShortMessage].Bytes()).Decode())
		if !strings.Contains(body, "4321") || !strings.Contains(body, "aýtmaň") {
			t.Errorf("message = %q", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SMSC did not receive submit_sm")
	}
}

func TestSMPPSenderWrongPassword(t *testing.T) {
	srv := smpptest.NewServer()
	defer srv.Close()
	s := NewSMPPSender(&config.Config{SMPPAddr: srv.Addr(), SMPPUser: "client", SMPPPassword: "wrong", OTPTTLMin: 5})
	defer s.Close()
	if err := s.Send("+99365123456", "test"); err == nil {
		t.Fatal("expected an error when the bind is refused")
	}
}
