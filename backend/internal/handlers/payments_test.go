package handlers_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ashgabat-taxi/backend/internal/services"
)

// approvedDriver registers and approves a driver, returns token and driver id.
func (e env) approvedDriver(phone, admin string) (string, any) {
	tok := e.login(phone, "driver")
	id := data(e.call("POST", "/api/v1/driver/register", tok, map[string]any{"car_brand": "Toyota", "car_plate": "AG " + phone[len(phone)-4:], "license_number": "L" + phone, "tariff_codes": "economy"}))["id"]
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", id), admin, map[string]any{"status": "approved"})
	return tok, id
}

func (e env) adminToken() string {
	tok, _ := data(e.call("POST", "/api/v1/auth/admin/login", "", map[string]any{"phone": "+99360000000", "password": "admin12345"}))["token"].(string)
	return tok
}

func (e env) balance(tok string) float64 {
	return data(e.call("GET", "/api/v1/driver/earnings", tok, nil))["balance"].(float64)
}

// postForm submits the demo bank form like a browser (no redirects followed).
func postForm(t *testing.T, u, action string) {
	cli := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cli.PostForm(u, url.Values{"action": {action}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func getBody(t *testing.T, u string) string {
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestCardTopUpDemo(t *testing.T) {
	e := startServer(t)
	admin := e.adminToken()
	drv, _ := e.approvedDriver("+99362100001", admin)

	if r := e.call("POST", "/api/v1/driver/topup", drv, map[string]any{"amount": 1}); r["success"] != false {
		t.Fatalf("amount below minimum accepted: %v", r)
	}
	r := data(e.call("POST", "/api/v1/driver/topup", drv, map[string]any{"amount": 100}))
	form, _ := r["form_url"].(string)
	pid := r["payment"].(map[string]any)["id"]
	if !strings.Contains(form, fmt.Sprintf("/api/v1/payments/demo/%v", pid)) {
		t.Fatalf("form url: %v", r)
	}
	if !strings.Contains(getBody(t, form), "DEMO bank") {
		t.Fatal("demo form page not served")
	}
	postForm(t, form, "pay")
	postForm(t, form, "pay") // a second submit must not credit twice
	st := data(e.call("GET", fmt.Sprintf("/api/v1/driver/topup/%v", pid), drv, nil))
	if st["payment"].(map[string]any)["status"] != "paid" || st["balance"].(float64) != 100 {
		t.Fatalf("after pay: %v", st)
	}
	if !strings.Contains(getBody(t, e.base+fmt.Sprintf("/api/v1/payments/return?pid=%v", pid)), "Töleg kabul edildi") {
		t.Fatal("return page does not confirm")
	}
	tx := data(e.call("GET", "/api/v1/driver/earnings", drv, nil))["transactions"].([]any)
	if tx[0].(map[string]any)["type"] != "card_topup" || len(tx) != 1 {
		t.Fatalf("transactions: %v", tx)
	}

	// cancelled card form: nothing credited
	r = data(e.call("POST", "/api/v1/driver/topup", drv, map[string]any{"amount": 50}))
	postForm(t, r["form_url"].(string), "cancel")
	st = data(e.call("GET", fmt.Sprintf("/api/v1/driver/topup/%v", r["payment"].(map[string]any)["id"]), drv, nil))
	if st["payment"].(map[string]any)["status"] != "failed" || st["balance"].(float64) != 100 {
		t.Fatalf("after cancel: %v", st)
	}

	// admin sees the payments
	ap := data(e.call("GET", "/api/v1/admin/payments", admin, nil))
	if ap["total"].(float64) < 2 || ap["paid_today"].(float64) < 100 {
		t.Fatalf("admin payments: %v", ap)
	}
}

func TestCardTopUpBank(t *testing.T) {
	var bankStatus atomic.Int32 // 0 registered, 2 paid
	bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/register.do":
			_, _ = w.Write([]byte(`{"orderId":"o-1","formUrl":"https://bank.example/pay/o-1"}`))
		case "/getOrderStatusExtended.do":
			_, _ = fmt.Fprintf(w, `{"errorCode":"0","orderStatus":%d}`, bankStatus.Load())
		}
	}))
	defer bank.Close()
	e := startServer(t)
	e.h.Gateway = &services.RBSGateway{BaseURL: bank.URL, User: "m", Password: "p", Currency: "934"}
	drv, _ := e.approvedDriver("+99362100002", e.adminToken())

	r := data(e.call("POST", "/api/v1/driver/topup", drv, map[string]any{"amount": 75.5}))
	if r["form_url"] != "https://bank.example/pay/o-1" {
		t.Fatalf("form: %v", r)
	}
	pid := r["payment"].(map[string]any)["id"]
	// redirect parameters cannot fake a payment: the bank still says "registered"
	if !strings.Contains(getBody(t, e.base+fmt.Sprintf("/api/v1/payments/return?pid=%v&orderId=o-1", pid)), "barlanýar") {
		t.Fatal("unpaid order shown as done")
	}
	if b := e.balance(drv); b != 0 {
		t.Fatalf("credited before the bank confirmed: %v", b)
	}
	bankStatus.Store(2)
	for i := 0; i < 3; i++ { // return page + app polling: credited once
		getBody(t, e.base+fmt.Sprintf("/api/v1/payments/return?pid=%v", pid))
		e.call("GET", fmt.Sprintf("/api/v1/driver/topup/%v", pid), drv, nil)
	}
	if b := e.balance(drv); b != 75.5 {
		t.Fatalf("balance %v, want 75.5", b)
	}
}

func TestPaymentPlans(t *testing.T) {
	e := startServer(t)
	admin := e.adminToken()

	// commission plan: below the minimum balance the driver cannot go online until topping up
	drv, id := e.approvedDriver("+99362100003", admin)
	e.call("POST", fmt.Sprintf("/api/v1/admin/drivers/%v/balance", id), admin, map[string]any{"amount": -60, "note": "bergi"})
	r := e.call("POST", "/api/v1/driver/online", drv, map[string]any{"online": true, "lat": 37.95, "lng": 58.33})
	if r["success"] != false || data(r)["need_topup"] != true {
		t.Fatalf("went online with -60: %v", r)
	}
	f := data(e.call("POST", "/api/v1/driver/topup", drv, map[string]any{"amount": 20}))["form_url"].(string)
	postForm(t, f, "pay")
	if r := e.call("POST", "/api/v1/driver/online", drv, map[string]any{"online": true, "lat": 37.95, "lng": 58.33}); r["success"] != true {
		t.Fatalf("still blocked after top-up: %v", r)
	}

	// monthly plan: the fee is taken from the balance, then rides are commission-free
	mdrv, mid := e.approvedDriver("+99362100004", admin)
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", mid), admin, map[string]any{"payment_plan": "monthly", "monthly_fee": 200})
	if r := e.call("POST", "/api/v1/driver/online", mdrv, map[string]any{"online": true, "lat": 37.95, "lng": 58.33}); r["success"] != false {
		t.Fatalf("monthly driver online without paying: %v", r)
	}
	f = data(e.call("POST", "/api/v1/driver/topup", mdrv, map[string]any{"amount": 250}))["form_url"].(string)
	postForm(t, f, "pay") // credited 250, the 200 fee is taken at once
	earn := data(e.call("GET", "/api/v1/driver/earnings", mdrv, nil))
	if earn["balance"].(float64) != 50 || earn["subscription_until"] == nil || earn["payment_plan"] != "monthly" || earn["monthly_fee"].(float64) != 200 {
		t.Fatalf("monthly after top-up: %v", earn)
	}
	if r := e.call("POST", "/api/v1/driver/online", mdrv, map[string]any{"online": true, "lat": 37.9601, "lng": 58.3261}); r["success"] != true {
		t.Fatalf("monthly driver blocked after paying: %v", r)
	}
	e.call("POST", "/api/v1/driver/online", drv, map[string]any{"online": false})
	client := e.login("+99365100004", "client")
	o := data(e.call("POST", "/api/v1/orders/", client, map[string]any{"tariff_id": 1, "pickup": map[string]any{"lat": 37.96, "lng": 58.326}, "dropoff": map[string]any{"lat": 37.93, "lng": 58.39}}))
	for _, step := range []string{"accept", "arrived", "start", "complete"} {
		if r := e.call("POST", fmt.Sprintf("/api/v1/driver/orders/%v/%s", o["id"], step), mdrv, nil); r["success"] != true {
			t.Fatalf("%s: %v", step, r)
		}
	}
	done := data(e.call("GET", fmt.Sprintf("/api/v1/orders/%v", o["id"]), client, nil))
	if done["commission"].(float64) != 0 {
		t.Fatalf("monthly driver paid commission: %v", done["commission"])
	}
	if b := e.balance(mdrv); b != 50 {
		t.Fatalf("monthly balance changed by a cash ride: %v", b)
	}
	// paying the next month early extends from the current end
	if r := e.call("POST", "/api/v1/driver/subscription", mdrv, nil); r["success"] != false {
		t.Fatalf("renewed with 50 < 200: %v", r)
	}
}

func TestBillingSweep(t *testing.T) {
	e := startServer(t)
	admin := e.adminToken()
	drv, id := e.approvedDriver("+99362100005", admin)
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", id), admin, map[string]any{"payment_plan": "monthly", "monthly_fee": 100})
	e.call("POST", fmt.Sprintf("/api/v1/admin/drivers/%v/balance", id), admin, map[string]any{"amount": 150})
	if r := e.call("POST", "/api/v1/driver/online", drv, map[string]any{"online": true, "lat": 37.95, "lng": 58.33}); r["success"] != true {
		t.Fatalf("online: %v", r) // first month taken: balance 50
	}
	expire := func() {
		e.h.DB.Exec("UPDATE drivers SET subscription_until = ? WHERE id = ?", time.Now().Add(-time.Minute), id)
	}
	state := func() (bool, float64) {
		var row struct {
			IsOnline bool
			Balance  float64
		}
		e.h.DB.Table("drivers").Select("is_online, balance").Where("id = ?", id).Scan(&row)
		return row.IsOnline, row.Balance
	}

	// month over, 50 < 100: the sweep takes the driver offline
	expire()
	e.h.Billing.Sweep()
	if on, b := state(); on || b != 50 {
		t.Fatalf("after sweep without money: online=%v balance=%v", on, b)
	}
	// topped up and back online: the sweep renews from the balance and keeps the driver working
	e.call("POST", fmt.Sprintf("/api/v1/admin/drivers/%v/balance", id), admin, map[string]any{"amount": 100})
	if r := e.call("POST", "/api/v1/driver/online", drv, map[string]any{"online": true, "lat": 37.95, "lng": 58.33}); r["success"] != true {
		t.Fatalf("online after top-up: %v", r) // renewed here: 150 - 100 = 50
	}
	expire()
	e.call("POST", fmt.Sprintf("/api/v1/admin/drivers/%v/balance", id), admin, map[string]any{"amount": 100})
	e.h.Billing.Sweep()
	if on, b := state(); !on || b != 50 {
		t.Fatalf("sweep renewal: online=%v balance=%v", on, b)
	}
}

func TestScheduledRide(t *testing.T) {
	e := startServer(t)
	admin := e.adminToken()
	drv, _ := e.approvedDriver("+99362100006", admin)
	e.call("POST", "/api/v1/driver/online", drv, map[string]any{"online": true, "lat": 37.96, "lng": 58.326})
	client := e.login("+99365100006", "client")
	trip := func(at time.Time) map[string]any {
		return e.call("POST", "/api/v1/orders/", client, map[string]any{"tariff_id": 1, "scheduled_at": at.UTC().Format(time.RFC3339),
			"pickup": map[string]any{"lat": 37.96, "lng": 58.326}, "dropoff": map[string]any{"lat": 37.93, "lng": 58.39}})
	}
	if r := trip(time.Now().AddDate(0, 0, 8)); r["success"] != false {
		t.Fatalf("8 days ahead accepted: %v", r)
	}
	o := data(trip(time.Now().Add(2 * time.Hour)))
	if o["scheduled_at"] == nil || o["status"] != "searching" {
		t.Fatalf("booking: %v", o)
	}
	if list := e.call("GET", "/api/v1/driver/orders/available", drv, nil)["data"].([]any); len(list) != 0 {
		t.Fatalf("ride booked for later offered now: %v", list)
	}
	if r := e.call("POST", fmt.Sprintf("/api/v1/orders/%v/cancel", o["id"]), client, map[string]any{"reason": "test"}); r["success"] != true {
		t.Fatalf("cancel booking: %v", r)
	}
	// card is refused while ride card payment is off
	r := e.call("POST", "/api/v1/orders/", client, map[string]any{"tariff_id": 1, "payment_method": "card",
		"pickup": map[string]any{"lat": 37.96, "lng": 58.326}, "dropoff": map[string]any{"lat": 37.93, "lng": 58.39}})
	if r["success"] != false {
		t.Fatalf("card ride accepted: %v", r)
	}
}
