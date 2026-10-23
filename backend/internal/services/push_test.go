package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/ashgabat-taxi/backend/internal/models"
)

func TestFCMSenderRequestAndInvalidToken(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		if strings.Contains(string(b), "old-token") {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"projects/p/messages/1"}`))
	}))
	defer srv.Close()
	f := &FCMSender{ProjectID: "taxi-tm", Endpoint: srv.URL, tokens: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "acc"}), cli: srv.Client()}

	err := f.Send(context.Background(), "dev-token", PushMessage{Title: "Täze sargyt", Body: "A → B", Data: map[string]string{"type": "order.new"}, Urgent: true})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer acc" || path != "/v1/projects/taxi-tm/messages:send" {
		t.Fatalf("auth=%q path=%q", auth, path)
	}
	m := got["message"].(map[string]any)
	android := m["android"].(map[string]any)
	if m["token"] != "dev-token" || android["priority"] != "HIGH" || android["notification"].(map[string]any)["channel_id"] != "orders" {
		t.Fatalf("message = %v", m)
	}
	if m["notification"].(map[string]any)["title"] != "Täze sargyt" || m["data"].(map[string]any)["type"] != "order.new" {
		t.Fatalf("notification/data = %v", m)
	}
	if err := f.Send(context.Background(), "old-token", PushMessage{Title: "x"}); !errors.Is(err, ErrPushTokenInvalid) {
		t.Fatalf("want ErrPushTokenInvalid, got %v", err)
	}
}

func TestComposeTexts(t *testing.T) {
	p := &Pusher{currency: "TMT"}
	drv := &models.Driver{CarColor: "Ak", CarBrand: "Toyota", CarModel: "Camry", CarPlate: "AG 4545"}
	order := models.Order{ID: 7, PickupAddress: "Bitarap", DropoffAddress: "Aeroport", EstimatedPrice: 21.5, Driver: drv}
	ev := func(mut func(*eventData)) *eventData {
		e := &eventData{Order: order}
		if mut != nil {
			mut(e)
		}
		return e
	}
	cases := []struct {
		role       models.Role
		typ        string
		e          *eventData
		want       string // substring of title+body, "" = no push
		wantUrgent bool
	}{
		{models.RoleDriver, "order.new", &eventData{Inner: &order}, "Täze sargyt · 21.5 TMT|Bitarap → Aeroport", true},
		{models.RoleClient, "order.accepted", ev(nil), "Sürüji tapyldy|Ak Toyota Camry, AG 4545 size gelýär", true},
		{models.RoleClient, "order.status", ev(func(e *eventData) { e.Order.Status = models.OrderArrived }), "Sürüji garaşýar", true},
		{models.RoleClient, "order.status", ev(func(e *eventData) { e.Order.Status = models.OrderInProgress }), "", false},
		{models.RoleClient, "order.completed", ev(func(e *eventData) { e.Order.FinalPrice = 23 }), "Syýahat tamamlandy · 23 TMT", false},
		{models.RoleClient, "order.cancelled", ev(func(e *eventData) { e.Order.CancelledBy = "client" }), "", false},
		{models.RoleClient, "order.cancelled", ev(func(e *eventData) { e.Order.CancelledBy = "system" }), "Sürüji tapylmady", true},
		{models.RoleDriver, "order.cancelled", ev(func(e *eventData) { e.Order.CancelledBy = "client" }), "Sargyt ýatyryldy", true},
		{models.RoleDriver, "driver.status", ev(func(e *eventData) { e.Order.Status = "approved" }), "Profiliňiz tassyklandy", false},
		{models.RoleClient, "ticket.reply", ev(func(e *eventData) { e.Reply = "Barlarys" }), "Goldaw gullugy jogap berdi|Barlarys", false},
		{models.RoleClient, "driver.location", ev(nil), "", false},
	}
	for _, c := range cases {
		m, ok := p.Compose(c.role, c.typ, c.e)
		if c.want == "" {
			if ok {
				t.Errorf("%s/%s: unexpected push %+v", c.role, c.typ, m)
			}
			continue
		}
		if !ok {
			t.Errorf("%s/%s: no push", c.role, c.typ)
			continue
		}
		text := m.Title + "|" + m.Body
		for _, part := range strings.Split(c.want, "|") {
			if !strings.Contains(text, part) {
				t.Errorf("%s/%s: %q does not contain %q", c.role, c.typ, text, part)
			}
		}
		if m.Urgent != c.wantUrgent {
			t.Errorf("%s/%s: urgent=%v", c.role, c.typ, m.Urgent)
		}
	}
}
