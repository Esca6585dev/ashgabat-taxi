package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
)

// PayState is the gateway's view of a card payment.
type PayState int

const (
	PayPending PayState = iota // card form not finished yet
	PayPaid                    // money captured: credit the balance
	PayFailed                  // declined, cancelled, reversed or expired
)

// PaymentGateway registers a card payment and reports its state. The server never trusts
// redirect parameters: the balance is credited only after Status returns PayPaid.
type PaymentGateway interface {
	Name() string
	// Create registers the payment and returns the gateway order id and the card form URL.
	Create(ctx context.Context, p *models.Payment, returnURL, description string) (orderID, formURL string, err error)
	Status(ctx context.Context, orderID string) (PayState, string, error)
}

// NewPaymentGateway picks the gateway from config; nil means card top-up is not configured.
func NewPaymentGateway(cfg *config.Config) PaymentGateway {
	switch cfg.PaymentProvider {
	case "rbs":
		return &RBSGateway{BaseURL: strings.TrimRight(cfg.RBSURL, "/"), User: cfg.RBSUser, Password: cfg.RBSPassword,
			Currency: cfg.PaymentCurrencyCode, Language: cfg.RBSLanguage, cli: &http.Client{Timeout: 20 * time.Second}}
	case "demo":
		return DemoGateway{}
	}
	return nil
}

// RBSGateway speaks the RBS/BPC REST API used by Turkmen banks' e-commerce
// (Altyn Asyr processing, e.g. https://mpi.gov.tm/payment/rest): register.do + getOrderStatusExtended.do.
type RBSGateway struct {
	BaseURL  string
	User     string
	Password string
	Currency string // ISO 4217 numeric, TMT = 934
	Language string
	cli      *http.Client
}

// Name implements PaymentGateway.
func (g *RBSGateway) Name() string { return "rbs" }

func (g *RBSGateway) call(ctx context.Context, method string, form url.Values, out any) error {
	form.Set("userName", g.User)
	form.Set("password", g.Password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/"+method, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cli := g.cli
	if cli == nil {
		cli = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("bank: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bank %s: HTTP %d", method, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("bank %s: bad response", method)
	}
	return nil
}

// rbsError is "errorCode" as string or number depending on the bank build.
type rbsError struct {
	Code    json.RawMessage `json:"errorCode"`
	Message string          `json:"errorMessage"`
}

func (e rbsError) failed() bool {
	c := strings.Trim(string(e.Code), `" `)
	return c != "" && c != "0" && c != "null"
}

// Create implements PaymentGateway (register.do, one-stage payment).
func (g *RBSGateway) Create(ctx context.Context, p *models.Payment, returnURL, description string) (string, string, error) {
	form := url.Values{}
	form.Set("orderNumber", fmt.Sprintf("AT%d-%d", p.ID, p.CreatedAt.Unix()))
	form.Set("amount", fmt.Sprint(int64(math.Round(p.Amount*100)))) // in tenge (1/100 manat)
	form.Set("currency", g.Currency)
	form.Set("returnUrl", returnURL)
	form.Set("failUrl", returnURL)
	form.Set("description", description)
	if g.Language != "" {
		form.Set("language", g.Language)
	}
	var r struct {
		rbsError
		OrderID string `json:"orderId"`
		FormURL string `json:"formUrl"`
	}
	if err := g.call(ctx, "register.do", form, &r); err != nil {
		return "", "", err
	}
	if r.failed() || r.OrderID == "" || r.FormURL == "" {
		return "", "", fmt.Errorf("bank register: %s", strings.TrimSpace(r.Message+" "+string(r.Code)))
	}
	return r.OrderID, r.FormURL, nil
}

// Status implements PaymentGateway (getOrderStatusExtended.do).
// orderStatus: 0 registered, 1 pre-authorized, 2 deposited (paid), 3 reversed, 4 refunded, 5 3-D Secure, 6 declined.
func (g *RBSGateway) Status(ctx context.Context, orderID string) (PayState, string, error) {
	form := url.Values{}
	form.Set("orderId", orderID)
	var r struct {
		rbsError
		OrderStatus  *int   `json:"orderStatus"`
		ActionDesc   string `json:"actionCodeDescription"`
		ErrorMessage string `json:"ErrorMessage"`
	}
	if err := g.call(ctx, "getOrderStatusExtended.do", form, &r); err != nil {
		return PayPending, "", err
	}
	if r.OrderStatus == nil {
		if r.failed() {
			return PayPending, "", fmt.Errorf("bank status: %s", r.Message)
		}
		return PayPending, "", nil
	}
	switch *r.OrderStatus {
	case 2:
		return PayPaid, "", nil
	case 3, 4, 6:
		return PayFailed, strings.TrimSpace(r.ActionDesc + " " + r.ErrorMessage), nil
	}
	return PayPending, "", nil
}

// DemoGateway is a fake bank for development: its "card form" is a page on this server
// (/api/v1/payments/demo/:id) with Pay / Cancel buttons. Never enable it in production.
type DemoGateway struct{}

// Name implements PaymentGateway.
func (DemoGateway) Name() string { return "demo" }

// Create implements PaymentGateway; the form URL is filled in by the handler (it knows the public URL).
func (DemoGateway) Create(_ context.Context, p *models.Payment, _, _ string) (string, string, error) {
	return fmt.Sprintf("demo-%d", p.ID), "", nil
}

// Status implements PaymentGateway; demo payments are settled directly by the demo page.
func (DemoGateway) Status(context.Context, string) (PayState, string, error) {
	return PayPending, "", nil
}

// ErrPaymentsOff is returned when no card gateway is configured.
var ErrPaymentsOff = errors.New("kart bilen töleg entek sazlanmadyk, operator bilen habarlaşyň")
