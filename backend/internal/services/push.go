package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
)

// PushMessage is one notification for one device.
type PushMessage struct {
	Title  string
	Body   string
	Data   map[string]string // delivered to the app (type, order_id) for routing on tap
	Urgent bool              // high priority: wakes the phone (new order for a driver)
}

// ErrPushTokenInvalid means the device token is gone (app removed, token rotated): forget it.
var ErrPushTokenInvalid = errors.New("push: device token is no longer valid")

// PushSender delivers a message to one device token.
type PushSender interface {
	Send(ctx context.Context, token string, m PushMessage) error
}

// FCMSender talks to the Firebase Cloud Messaging HTTP v1 API with a service account.
type FCMSender struct {
	ProjectID string
	Endpoint  string // https://fcm.googleapis.com (overridable in tests)
	tokens    oauth2.TokenSource
	cli       *http.Client
}

// NewFCMSender builds a sender from a Firebase service-account JSON key.
func NewFCMSender(credentialsJSON []byte) (*FCMSender, error) {
	creds, err := google.CredentialsFromJSON(context.Background(), credentialsJSON, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, fmt.Errorf("fcm credentials: %w", err)
	}
	if creds.ProjectID == "" {
		return nil, errors.New("fcm credentials: project_id missing")
	}
	return &FCMSender{ProjectID: creds.ProjectID, Endpoint: "https://fcm.googleapis.com", tokens: creds.TokenSource, cli: &http.Client{Timeout: 10 * time.Second}}, nil
}

// Send implements PushSender.
func (f *FCMSender) Send(ctx context.Context, token string, m PushMessage) error {
	priority, channel := "NORMAL", "ride"
	if m.Urgent {
		priority, channel = "HIGH", "orders"
	}
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token":        token,
		"notification": map[string]string{"title": m.Title, "body": m.Body},
		"data":         m.Data,
		"android": map[string]any{
			"priority":     priority,
			"ttl":          "300s",
			"notification": map[string]string{"channel_id": channel, "sound": "default"},
		},
		"apns": map[string]any{"payload": map[string]any{"aps": map[string]string{"sound": "default"}}},
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/projects/%s/messages:send", f.Endpoint, f.ProjectID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	tok, err := f.tokens.Token()
	if err != nil {
		return fmt.Errorf("fcm auth: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode == http.StatusNotFound || bytes.Contains(msg, []byte("UNREGISTERED")) ||
		(resp.StatusCode == http.StatusBadRequest && bytes.Contains(msg, []byte("registration token"))) {
		return ErrPushTokenInvalid
	}
	return fmt.Errorf("fcm status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}

// Pusher turns realtime events into phone notifications for users who have an FCM token.
type Pusher struct {
	db       *gorm.DB
	sender   PushSender
	currency string
}

// NewPusher returns nil (push disabled) when no Firebase key is configured.
func NewPusher(db *gorm.DB, cfg *config.Config) *Pusher {
	creds := []byte(cfg.FCMCredentialsJSON)
	if len(creds) == 0 && cfg.FCMCredentialsFile != "" {
		b, err := os.ReadFile(cfg.FCMCredentialsFile)
		if err != nil {
			log.Printf("push: cannot read %s: %v (push disabled)", cfg.FCMCredentialsFile, err)
			return nil
		}
		creds = b
	}
	if len(creds) == 0 {
		log.Printf("push: FCM_CREDENTIALS_FILE not set, push notifications disabled")
		return nil
	}
	s, err := NewFCMSender(creds)
	if err != nil {
		log.Printf("push: %v (push disabled)", err)
		return nil
	}
	log.Printf("push: FCM enabled for project %s", s.ProjectID)
	return NewPusherWith(db, s, cfg.Currency)
}

// NewPusherWith wires a custom sender (tests).
func NewPusherWith(db *gorm.DB, s PushSender, currency string) *Pusher {
	return &Pusher{db: db, sender: s, currency: currency}
}

// pushTypes are the realtime events worth a notification; everything else (locations, pings) is ignored cheaply.
var pushTypes = map[string]bool{
	"order.new": true, "order.accepted": true, "order.status": true, "order.completed": true,
	"order.cancelled": true, "order.requeued": true, "driver.status": true, "ticket.reply": true,
}

// eventData is the union of the payloads the hub sends for pushTypes.
type eventData struct {
	models.Order
	Inner     *models.Order `json:"order"`         // order.new, order.requeued
	Reason    string        `json:"reason"`        // order.requeued
	RejectWhy string        `json:"reject_reason"` // driver.status
	Reply     string        `json:"reply"`         // ticket.reply
}

// Event is the hub hook: called for every SendTo. Returns immediately; delivery runs in the background.
func (p *Pusher) Event(userID uint, typ string, data any) {
	if p == nil || !pushTypes[typ] {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	go p.deliver(userID, typ, raw)
}

func (p *Pusher) deliver(userID uint, typ string, raw []byte) {
	var u models.User
	if err := p.db.Select("id", "role", "fcm_token").First(&u, userID).Error; err != nil || u.FCMToken == "" {
		return
	}
	var ev eventData
	_ = json.Unmarshal(raw, &ev)
	m, ok := p.Compose(u.Role, typ, &ev)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := p.sender.Send(ctx, u.FCMToken, m)
	if errors.Is(err, ErrPushTokenInvalid) {
		p.db.Model(&models.User{}).Where("id = ? AND fcm_token = ?", u.ID, u.FCMToken).Update("fcm_token", "")
	} else if err != nil {
		log.Printf("push to user %d (%s): %v", u.ID, typ, err)
	}
}

func (p *Pusher) money(v float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", v), "0"), ".") + " " + p.currency
}

func orderPrice(o *models.Order) float64 {
	if o.FinalPrice > 0 {
		return o.FinalPrice
	}
	return o.EstimatedPrice - o.Discount
}

// Compose builds the Turkmen notification text for a recipient role and event (false = no notification).
func (p *Pusher) Compose(role models.Role, typ string, ev *eventData) (PushMessage, bool) {
	o := &ev.Order
	if ev.Inner != nil {
		o = ev.Inner
	}
	data := map[string]string{"type": typ}
	if o.ID != 0 {
		data["order_id"] = fmt.Sprint(o.ID)
	}
	route := strings.Trim(o.PickupAddress+" → "+o.DropoffAddress, " →")
	car := ""
	if o.Driver != nil {
		car = strings.TrimSpace(strings.Join([]string{o.Driver.CarColor, o.Driver.CarBrand, o.Driver.CarModel}, " ") + ", " + o.Driver.CarPlate)
	}
	msg := func(title, body string, urgent bool) (PushMessage, bool) {
		return PushMessage{Title: title, Body: body, Data: data, Urgent: urgent}, true
	}

	switch role {
	case models.RoleDriver:
		switch typ {
		case "order.new":
			return msg("Täze sargyt · "+p.money(orderPrice(o)), route, true)
		case "order.cancelled":
			if o.CancelledBy == "client" || o.CancelledBy == "admin" {
				return msg("Sargyt ýatyryldy", fmt.Sprintf("#%d sargyt ýatyryldy. %s", o.ID, o.CancelReason), true)
			}
		case "driver.status":
			// the driver profile's "status" lands in the embedded Order.Status
			switch string(ev.Order.Status) {
			case string(models.DriverApproved):
				return msg("Profiliňiz tassyklandy", "Indi «Işe başla» basyp sargyt alyp bilersiňiz.", false)
			case string(models.DriverRejected):
				return msg("Profil ret edildi", strings.TrimSpace("Maglumatlary düzediň. "+ev.RejectWhy), false)
			case string(models.DriverBlocked):
				return msg("Hasabyňyz bloklandy", strings.TrimSpace(ev.RejectWhy), false)
			}
		case "ticket.reply":
			return msg("Goldaw gullugy jogap berdi", ev.Reply, false)
		}
	case models.RoleClient:
		switch typ {
		case "order.accepted":
			return msg("Sürüji tapyldy", strings.Trim(car+" size gelýär", " ,"), true)
		case "order.status":
			if o.Status == models.OrderArrived {
				return msg("Sürüji garaşýar", strings.Trim(car+" geldi", " ,"), true)
			}
		case "order.completed":
			return msg("Syýahat tamamlandy · "+p.money(orderPrice(o)), "Sürüjä baha beriň.", false)
		case "order.cancelled":
			switch o.CancelledBy {
			case "system":
				return msg("Sürüji tapylmady", "Golaýda boş sürüji ýok. Biraz soňra täzeden synanyşyň.", true)
			case "admin":
				return msg("Sargyt ýatyryldy", "Sargydyňyzy operator ýatyrdy. "+o.CancelReason, true)
			}
		case "order.requeued":
			return msg("Sürüji sargydy ýatyrdy", "Size täze sürüji gözlenýär.", true)
		case "ticket.reply":
			return msg("Goldaw gullugy jogap berdi", ev.Reply, false)
		}
	}
	return PushMessage{}, false
}
