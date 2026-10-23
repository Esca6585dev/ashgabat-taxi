package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	fws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/database"
	"github.com/ashgabat-taxi/backend/internal/handlers"
	"github.com/ashgabat-taxi/backend/internal/routes"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/ws"
)

type env struct {
	base string
	t    *testing.T
	h    *handlers.Handler
	hub  *ws.Hub
}

func (e env) call(method, path, token string, body any) map[string]any {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func data(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)
	return d
}

func startServer(t *testing.T, setup ...func(*gorm.DB, *ws.Hub)) env {
	t.Helper()
	os.Setenv("DB_DSN", "file::memory:?cache=shared")
	os.Setenv("OTP_FIXED_CODE", "1234")
	os.Setenv("ADMIN_DIR", "/nonexistent")
	os.Setenv("SITE_DIR", "/nonexistent")
	os.Setenv("NOMINATIM_URL", "http://127.0.0.1:1") // unreachable -> coordinate fallback
	cfg := config.Load()
	db := database.Connect(cfg)
	database.Migrate(db)
	database.Seed(db, cfg)
	hub := ws.NewHub()
	for _, f := range setup {
		f(db, hub)
	}
	disp := services.NewDispatcher(db, hub, cfg)
	h := handlers.New(db, cfg, hub, disp, services.NewOTPService(db, cfg, services.LogSender{}), services.NewRouter(cfg), services.NewGeocoder(cfg))
	h.Billing = services.NewBilling(db, cfg, hub)
	disp.Billing = h.Billing
	h.Gateway = services.DemoGateway{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	routes.Register(app, h)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	return env{base: "http://" + ln.Addr().String(), t: t, h: h, hub: hub}
}

func (e env) login(phone, role string) string {
	e.call("POST", "/api/v1/auth/send-otp", "", map[string]any{"phone": phone})
	r := e.call("POST", "/api/v1/auth/verify-otp", "", map[string]any{"phone": phone, "code": "1234", "role": role, "name": "Test"})
	tok, _ := data(r)["token"].(string)
	if tok == "" {
		e.t.Fatalf("login %s failed: %v", phone, r)
	}
	return tok
}

func TestFullRideFlowWithWebSocket(t *testing.T) {
	e := startServer(t)
	client := e.login("+99365000001", "client")
	driver := e.login("+99362000001", "driver")
	admin, _ := data(e.call("POST", "/api/v1/auth/admin/login", "", map[string]any{"phone": "+99360000000", "password": "admin12345"}))["token"].(string)
	if admin == "" {
		t.Fatal("admin login failed")
	}

	r := e.call("POST", "/api/v1/driver/register", driver, map[string]any{"car_brand": "Toyota", "car_plate": "AG1", "license_number": "L1", "tariff_codes": "economy"})
	driverID := data(r)["id"]
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", driverID), admin, map[string]any{"status": "approved"})

	// websocket connections
	wsURL := "ws" + e.base[4:] + "/ws?token="
	dconn, _, err := fws.DefaultDialer.Dial(wsURL+driver, nil)
	if err != nil {
		t.Fatalf("driver ws: %v", err)
	}
	defer dconn.Close()
	cconn, _, err := fws.DefaultDialer.Dial(wsURL+client, nil)
	if err != nil {
		t.Fatalf("client ws: %v", err)
	}
	defer cconn.Close()
	expect := func(c *fws.Conn, typ string) map[string]any {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			_ = c.SetReadDeadline(deadline)
			_, msg, err := c.ReadMessage()
			if err != nil {
				t.Fatalf("waiting for %s: %v", typ, err)
			}
			var ev map[string]any
			_ = json.Unmarshal(msg, &ev)
			if ev["type"] == typ {
				return ev
			}
		}
		t.Fatalf("event %s not received", typ)
		return nil
	}
	expect(dconn, "connected")
	expect(cconn, "connected")

	e.call("POST", "/api/v1/driver/online", driver, map[string]any{"online": true, "lat": 37.95, "lng": 58.33})
	_ = dconn.WriteJSON(map[string]any{"type": "location", "lat": 37.951, "lng": 58.331, "heading": 45})

	r = e.call("POST", "/api/v1/orders/", client, map[string]any{"tariff_id": 1, "pickup": map[string]any{"lat": 37.96, "lng": 58.326}, "dropoff": map[string]any{"lat": 37.93, "lng": 58.39}})
	order := data(r)
	if order["status"] != "searching" {
		t.Fatalf("order not created: %v", r)
	}
	oid := order["id"]

	ev := expect(dconn, "order.new") // dispatcher pushed the offer
	if d, _ := ev["data"].(map[string]any); d["order"].(map[string]any)["id"] != oid {
		t.Fatalf("wrong order offered: %v", ev)
	}

	r = e.call("POST", fmt.Sprintf("/api/v1/driver/orders/%v/accept", oid), driver, nil)
	if data(r)["status"] != "accepted" {
		t.Fatalf("accept failed: %v", r)
	}
	expect(cconn, "order.accepted")

	_ = dconn.WriteJSON(map[string]any{"type": "location", "lat": 37.955, "lng": 58.328, "heading": 90})
	loc := expect(cconn, "driver.location")
	if d, _ := loc["data"].(map[string]any); d["lat"] != 37.955 {
		t.Fatalf("client did not get driver location: %v", loc)
	}

	for _, step := range []string{"arrived", "start", "complete"} {
		r = e.call("POST", fmt.Sprintf("/api/v1/driver/orders/%v/%s", oid, step), driver, nil)
		if r["success"] != true {
			t.Fatalf("%s failed: %v", step, r)
		}
	}
	done := expect(cconn, "order.completed")
	if d, _ := done["data"].(map[string]any); d["final_price"].(float64) <= 0 {
		t.Fatalf("final price missing: %v", done)
	}

	stats := data(e.call("GET", "/api/v1/admin/stats", admin, nil))
	if stats["completed_today"].(float64) != 1 {
		t.Fatalf("stats wrong: %v", stats)
	}
}

func TestPhoneNormalizationAndRejects(t *testing.T) {
	e := startServer(t)
	r := e.call("POST", "/api/v1/auth/send-otp", "", map[string]any{"phone": "12345"})
	if r["success"] != false {
		t.Fatalf("short phone accepted: %v", r)
	}
	r = e.call("POST", "/api/v1/auth/send-otp", "", map[string]any{"phone": "8 65 12 34 56"})
	if data(r)["phone"] != "+99365123456" {
		t.Fatalf("normalization failed: %v", r)
	}
	r = e.call("GET", "/api/v1/orders/active", "", nil)
	if r["success"] != false {
		t.Fatalf("unauthenticated access allowed: %v", r)
	}
}

func TestDriverCarChangeNeedsNewApproval(t *testing.T) {
	e := startServer(t)
	driver := e.login("+99362000077", "driver")
	admin, _ := data(e.call("POST", "/api/v1/auth/admin/login", "", map[string]any{"phone": "+99360000000", "password": "admin12345"}))["token"].(string)
	car := map[string]any{"car_brand": "Toyota", "car_model": "Camry", "car_plate": "ag 7777", "license_number": "L77", "tariff_codes": "economy"}
	id := data(e.call("POST", "/api/v1/driver/register", driver, car))["id"]
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", id), admin, map[string]any{"status": "approved"})

	// colour / tariffs only: stays approved
	car["car_color"], car["tariff_codes"] = "Ak", "economy,comfort"
	if st := data(e.call("POST", "/api/v1/driver/register", driver, car))["status"]; st != "approved" {
		t.Fatalf("cosmetic change reset approval: %v", st)
	}
	// new plate: back to moderation and offline
	car["car_plate"] = "AG 8888"
	d := data(e.call("POST", "/api/v1/driver/register", driver, car))
	if d["status"] != "pending" || d["is_online"] != false {
		t.Fatalf("plate change kept approval: %v", d)
	}
}

func TestDriverPasswordLogin(t *testing.T) {
	e := startServer(t)
	admin, _ := data(e.call("POST", "/api/v1/auth/admin/login", "", map[string]any{"phone": "+99360000000", "password": "admin12345"}))["token"].(string)
	login := func(phone, pass string) map[string]any {
		return e.call("POST", "/api/v1/auth/login", "", map[string]any{"phone": phone, "password": pass})
	}

	// admin creates a driver account; the driver logs in without SMS
	r := e.call("POST", "/api/v1/admin/drivers/account", admin, map[string]any{"phone": "65 33 44 55", "name": "Parol Sürüji", "password": "gizlin12"})
	if data(r)["role"] != "driver" || data(r)["has_password"] != true {
		t.Fatalf("create driver account: %v", r)
	}
	r = login("+99365334455", "gizlin12")
	tok, _ := data(r)["token"].(string)
	if tok == "" {
		t.Fatalf("password login failed: %v", r)
	}
	if me := data(e.call("GET", "/api/v1/auth/me", tok, nil)); me["role"] != "driver" || me["has_password"] != true {
		t.Fatalf("me: %v", me)
	}

	// driver changes own password: old one required
	if r := e.call("POST", "/api/v1/auth/password", tok, map[string]any{"old_password": "bad", "new_password": "taze1234"}); r["success"] != false {
		t.Fatalf("changed without old password: %v", r)
	}
	if r := e.call("POST", "/api/v1/auth/password", tok, map[string]any{"old_password": "gizlin12", "new_password": "taze1234"}); r["success"] != true {
		t.Fatalf("change password: %v", r)
	}
	if login("+99365334455", "gizlin12")["success"] != false || login("+99365334455", "taze1234")["success"] != true {
		t.Fatal("old password still works or new one does not")
	}

	// a driver who joined with SMS sets a first password without an old one
	sms := e.login("+99362334455", "driver")
	if r := e.call("POST", "/api/v1/auth/password", sms, map[string]any{"new_password": "birinji1"}); r["success"] != true {
		t.Fatalf("first password: %v", r)
	}
	if login("+99362334455", "birinji1")["success"] != true {
		t.Fatal("first password login failed")
	}

	// clients cannot use passwords
	cl := e.login("+99365334466", "client")
	if r := e.call("POST", "/api/v1/auth/password", cl, map[string]any{"new_password": "musderi1"}); r["success"] != false {
		t.Fatalf("client set a password: %v", r)
	}
	if login("+99365334466", "musderi1")["success"] != false {
		t.Fatal("client logged in with password")
	}

	// lockout after repeated wrong passwords, admin reset clears it
	for i := 0; i < 5; i++ {
		login("+99362334455", "yalnys")
	}
	if r := login("+99362334455", "birinji1"); r["success"] != false {
		t.Fatalf("not locked after 5 wrong passwords: %v", r)
	}
	users := data(e.call("GET", "/api/v1/admin/users?q=62334455", admin, nil))["items"].([]any)
	uid := users[0].(map[string]any)["id"]
	if r := e.call("PUT", fmt.Sprintf("/api/v1/admin/users/%v/password", uid), admin, map[string]any{"password": "admin-berdi"}); r["success"] != true {
		t.Fatalf("admin reset: %v", r)
	}
	if login("+99362334455", "admin-berdi")["success"] != true {
		t.Fatal("login after admin reset failed")
	}
}

// fakePush records notifications instead of calling Firebase.
type fakePush struct {
	mu   sync.Mutex
	sent []string // "token|title"
}

func (f *fakePush) Send(_ context.Context, token string, m services.PushMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if token == "dead-token" {
		return services.ErrPushTokenInvalid
	}
	f.sent = append(f.sent, token+"|"+m.Title)
	return nil
}

func (f *fakePush) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, s := range f.sent {
			if strings.Contains(s, want) {
				f.mu.Unlock()
				return
			}
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t.Fatalf("push %q not sent; got %v", want, f.sent)
}

func TestPushNotifications(t *testing.T) {
	fp := &fakePush{}
	var gdb *gorm.DB
	e := startServer(t, func(db *gorm.DB, hub *ws.Hub) {
		gdb = db
		hub.OnSend = services.NewPusherWith(db, fp, "TMT").Event
	})
	client := e.login("+99365000555", "client")
	driver := e.login("+99362000555", "driver")
	admin, _ := data(e.call("POST", "/api/v1/auth/admin/login", "", map[string]any{"phone": "+99360000000", "password": "admin12345"}))["token"].(string)
	e.call("PUT", "/api/v1/auth/profile", client, map[string]any{"fcm_token": "client-phone"})
	e.call("PUT", "/api/v1/auth/profile", driver, map[string]any{"fcm_token": "driver-phone"})

	id := data(e.call("POST", "/api/v1/driver/register", driver, map[string]any{"car_brand": "Toyota", "car_plate": "AG 5555", "license_number": "L5", "tariff_codes": "economy"}))["id"]
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", id), admin, map[string]any{"status": "approved"})
	fp.waitFor(t, "driver-phone|Profiliňiz tassyklandy")

	e.call("POST", "/api/v1/driver/online", driver, map[string]any{"online": true, "lat": 37.95, "lng": 58.33})
	o := data(e.call("POST", "/api/v1/orders/", client, map[string]any{"tariff_id": 1, "pickup": map[string]any{"lat": 37.96, "lng": 58.326}, "dropoff": map[string]any{"lat": 37.93, "lng": 58.39}}))
	fp.waitFor(t, "driver-phone|Täze sargyt") // offer reaches the driver's phone even without WebSocket

	e.call("POST", fmt.Sprintf("/api/v1/driver/orders/%v/accept", o["id"]), driver, nil)
	fp.waitFor(t, "client-phone|Sürüji tapyldy")
	e.call("POST", fmt.Sprintf("/api/v1/driver/orders/%v/arrived", o["id"]), driver, nil)
	fp.waitFor(t, "client-phone|Sürüji garaşýar")

	// same phone logs into another account: the token moves, the old account stops getting pushes
	other := e.login("+99365000556", "client")
	e.call("PUT", "/api/v1/auth/profile", other, map[string]any{"fcm_token": "client-phone"})
	var n int64
	gdb.Table("users").Where("fcm_token = ?", "client-phone").Count(&n)
	if n != 1 {
		t.Fatalf("token kept by %d accounts", n)
	}

	// a token Firebase rejects is forgotten
	e.call("PUT", "/api/v1/auth/profile", driver, map[string]any{"fcm_token": "dead-token"})
	e.call("POST", "/api/v1/driver/orders/"+fmt.Sprint(o["id"])+"/start", driver, nil)
	e.call("POST", "/api/v1/driver/orders/"+fmt.Sprint(o["id"])+"/complete", driver, nil)
	e.call("PUT", fmt.Sprintf("/api/v1/admin/drivers/%v/status", id), admin, map[string]any{"status": "approved"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		var tok string
		gdb.Table("users").Select("fcm_token").Where("phone = ?", "+99362000555").Scan(&tok)
		if tok == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dead token not cleared: %q", tok)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
