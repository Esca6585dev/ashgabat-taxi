package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ashgabat-taxi/backend/internal/models"
)

// fakeRBS imitates the bank's register.do / getOrderStatusExtended.do.
func fakeRBS(t *testing.T, status *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("userName") != "merchant" || r.Form.Get("password") != "secret" {
			_, _ = w.Write([]byte(`{"errorCode":"5","errorMessage":"Access denied"}`))
			return
		}
		switch r.URL.Path {
		case "/register.do":
			if r.Form.Get("amount") != "12550" || r.Form.Get("currency") != "934" || r.Form.Get("returnUrl") == "" {
				t.Errorf("register params: %v", r.Form)
			}
			_, _ = w.Write([]byte(`{"orderId":"bank-77","formUrl":"https://bank.example/form?id=bank-77"}`))
		case "/getOrderStatusExtended.do":
			if r.Form.Get("orderId") != "bank-77" {
				t.Errorf("status orderId: %v", r.Form)
			}
			_, _ = w.Write([]byte(`{"errorCode":"0","orderStatus":` + itoa(*status) + `,"actionCodeDescription":"Declined"}`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func itoa(i int) string { return string(rune('0' + i)) }

func TestRBSGateway(t *testing.T) {
	status := 0
	srv := fakeRBS(t, &status)
	defer srv.Close()
	g := &RBSGateway{BaseURL: srv.URL, User: "merchant", Password: "secret", Currency: "934", cli: srv.Client()}
	p := &models.Payment{ID: 5, Amount: 125.5, CreatedAt: time.Now()}

	id, form, err := g.Create(context.Background(), p, "https://api.taxi.tm/api/v1/payments/return?pid=5", "test")
	if err != nil || id != "bank-77" || form == "" {
		t.Fatalf("create: %q %q %v", id, form, err)
	}
	for _, c := range []struct {
		status int
		want   PayState
	}{{0, PayPending}, {5, PayPending}, {2, PayPaid}, {6, PayFailed}, {3, PayFailed}} {
		status = c.status
		got, _, err := g.Status(context.Background(), "bank-77")
		if err != nil || got != c.want {
			t.Errorf("orderStatus %d: got %v err %v", c.status, got, err)
		}
	}

	bad := &RBSGateway{BaseURL: srv.URL, User: "merchant", Password: "wrong", Currency: "934", cli: srv.Client()}
	if _, _, err := bad.Create(context.Background(), p, "x", "y"); err == nil {
		t.Fatal("bank error not reported")
	}
}
