package handlers

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// paymentTTL: a card form not finished within this time is closed as failed.
const paymentTTL = time.Hour

func (h *Handler) publicURL(c *fiber.Ctx) string {
	if h.Cfg.PublicURL != "" {
		return strings.TrimRight(h.Cfg.PublicURL, "/")
	}
	return c.BaseURL()
}

// DriverTopUp POST /driver/topup {amount} — starts a card payment, returns the bank form URL.
func (h *Handler) DriverTopUp(c *fiber.Ctx) error {
	if h.Gateway == nil {
		return utils.Err(c, 503, services.ErrPaymentsOff.Error())
	}
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	if d.Status == models.DriverBlocked {
		return utils.Err(c, 403, "sürüji hasabyňyz bloklanan")
	}
	var req struct {
		Amount float64 `json:"amount"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	amount := math.Round(req.Amount*100) / 100
	if amount < h.Cfg.TopUpMin || amount > h.Cfg.TopUpMax {
		return utils.Err(c, 400, fmt.Sprintf("mukdar %.0f–%.0f %s aralygynda bolmaly", h.Cfg.TopUpMin, h.Cfg.TopUpMax, h.Cfg.Currency))
	}
	p := models.Payment{DriverID: d.ID, Amount: amount, Status: models.PaymentPending, Provider: h.Gateway.Name()}
	if err := h.DB.Create(&p).Error; err != nil {
		return utils.Err(c, 500, "töleg döredip bolmady")
	}
	base := h.publicURL(c)
	returnURL := fmt.Sprintf("%s/api/v1/payments/return?pid=%d", base, p.ID)
	ctx, cancel := context.WithTimeout(c.Context(), 25*time.Second)
	defer cancel()
	orderID, formURL, err := h.Gateway.Create(ctx, &p, returnURL, fmt.Sprintf("Aşgabat Taxi balans: sürüji #%d", d.ID))
	if err != nil {
		log.Printf("payment %d: %v", p.ID, err)
		h.DB.Model(&p).Updates(map[string]any{"status": models.PaymentFailed, "error": truncate(err.Error(), 250)})
		return utils.Err(c, 502, "bank tölegi kabul etmedi, birazdan gaýtadan synanyşyň")
	}
	if formURL == "" { // demo bank: the card form is our own page
		formURL = fmt.Sprintf("%s/api/v1/payments/demo/%d", base, p.ID)
	}
	h.DB.Model(&p).Updates(map[string]any{"provider_order_id": orderID, "form_url": formURL})
	p.ProviderOrderID, p.FormURL = orderID, formURL
	return c.Status(201).JSON(fiber.Map{"success": true, "data": fiber.Map{"payment": p, "form_url": formURL}})
}

// DriverTopUpStatus GET /driver/topup/:id — the app polls this after returning from the bank page.
func (h *Handler) DriverTopUpStatus(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	var p models.Payment
	if err := h.DB.Where("id = ? AND driver_id = ?", id, d.ID).First(&p).Error; err != nil {
		return utils.Err(c, 404, "töleg tapylmady")
	}
	h.refreshPayment(c.Context(), &p)
	var fresh models.Driver
	h.DB.First(&fresh, d.ID)
	return utils.OK(c, fiber.Map{"payment": p, "balance": fresh.Balance, "subscription_until": fresh.SubscriptionUntil})
}

// DriverPayments GET /driver/payments — the driver's card top-ups.
func (h *Handler) DriverPayments(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	var ps []models.Payment
	h.DB.Where("driver_id = ?", d.ID).Order("id desc").Limit(30).Find(&ps)
	return utils.OK(c, ps)
}

// refreshPayment asks the bank about a pending payment and settles it. Safe to call repeatedly.
func (h *Handler) refreshPayment(ctx context.Context, p *models.Payment) {
	if p.Status != models.PaymentPending || h.Gateway == nil || p.ProviderOrderID == "" || p.Provider != h.Gateway.Name() {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	state, reason, err := h.Gateway.Status(cctx, p.ProviderOrderID)
	switch {
	case err != nil:
		log.Printf("payment %d status: %v", p.ID, err)
	case state == services.PayPaid:
		h.settlePayment(p, true, "")
		return
	case state == services.PayFailed:
		h.settlePayment(p, false, reason)
		return
	}
	if time.Since(p.CreatedAt) > paymentTTL {
		h.settlePayment(p, false, "möhleti geçdi")
	}
}

// settlePayment moves a pending payment to paid (crediting the balance exactly once) or failed.
func (h *Handler) settlePayment(p *models.Payment, paid bool, reason string) {
	now := time.Now()
	if !paid {
		res := h.DB.Model(&models.Payment{}).Where("id = ? AND status = ?", p.ID, models.PaymentPending).
			Updates(map[string]any{"status": models.PaymentFailed, "error": truncate(reason, 250)})
		if res.RowsAffected == 1 {
			p.Status, p.Error = models.PaymentFailed, reason
		}
		return
	}
	credited := false
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Payment{}).Where("id = ? AND status = ?", p.ID, models.PaymentPending).
			Updates(map[string]any{"status": models.PaymentPaid, "paid_at": now})
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error // already settled by another request
		}
		if err := tx.Model(&models.Driver{}).Where("id = ?", p.DriverID).Update("balance", gorm.Expr("balance + ?", p.Amount)).Error; err != nil {
			return err
		}
		credited = true
		return tx.Create(&models.Transaction{DriverID: p.DriverID, Amount: p.Amount, Type: "card_topup", Note: fmt.Sprintf("Kart bilen doldurmak #%d", p.ID)}).Error
	})
	if err != nil {
		log.Printf("payment %d settle: %v", p.ID, err)
		return
	}
	h.DB.First(p, p.ID)
	if !credited {
		return
	}
	var d models.Driver
	if h.DB.First(&d, p.DriverID).Error != nil {
		return
	}
	h.Hub.SendTo(d.UserID, "balance.changed", fiber.Map{"balance": d.Balance, "amount": p.Amount, "payment_id": p.ID})
	// a monthly driver whose plan ran out gets it renewed right away
	if h.Billing != nil && d.PaymentPlan == models.PlanMonthly && !services.SubscriptionActive(&d, now) {
		_ = h.Billing.Renew(&d)
	}
}

// ReconcilePayments settles card payments whose bank redirect never reached us (app closed, network).
func (h *Handler) ReconcilePayments(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var ps []models.Payment
			h.DB.Where("status = ? AND created_at < ?", models.PaymentPending, time.Now().Add(-2*time.Minute)).Limit(50).Find(&ps)
			for i := range ps {
				h.refreshPayment(ctx, &ps[i])
			}
		}
	}
}

// PaymentReturn GET /payments/return?pid= — the bank sends the driver's browser here after the card form.
func (h *Handler) PaymentReturn(c *fiber.Ctx) error {
	var p models.Payment
	if err := h.DB.First(&p, c.QueryInt("pid")).Error; err != nil {
		return paymentPage(c, 404, "Töleg tapylmady", "")
	}
	h.refreshPayment(c.Context(), &p)
	switch p.Status {
	case models.PaymentPaid:
		return paymentPage(c, 200, "Töleg kabul edildi ✅", fmt.Sprintf("Balansyňyza %.2f %s goşuldy. Programma dolanyň.", p.Amount, h.Cfg.Currency))
	case models.PaymentFailed:
		return paymentPage(c, 200, "Töleg geçmedi", strings.TrimSpace("Kartdan pul alynmady. "+p.Error))
	}
	return paymentPage(c, 200, "Töleg barlanýar…", "Bank tassyklan badyna balansyňyza goşular. Programma dolanyň.")
}

func paymentPage(c *fiber.Ctx, status int, title, text string) error {
	c.Status(status).Type("html", "utf-8")
	return c.SendString(`<!doctype html><html lang="tk"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>Aşgabat Taxi</title><style>body{font-family:system-ui,sans-serif;background:#f7f7f8;margin:0;display:grid;place-items:center;min-height:100vh}` +
		`.c{background:#fff;border-radius:18px;padding:28px;max-width:360px;margin:16px;text-align:center;box-shadow:0 4px 18px #0001}` +
		`h1{font-size:22px}p{color:#555}button{background:#FFC72C;border:0;border-radius:12px;padding:14px 22px;font-size:16px;font-weight:700}</style></head>` +
		`<body><div class="c"><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(text) + `</p>` + `</div></body></html>`)
}

// DemoPayPage GET /payments/demo/:id — fake card form of the demo bank (development only).
func (h *Handler) DemoPayPage(c *fiber.Ctx) error {
	p, err := h.demoPayment(c)
	if err != nil {
		return paymentPage(c, 404, "Töleg tapylmady", "")
	}
	if p.Status != models.PaymentPending {
		return c.Redirect(fmt.Sprintf("/api/v1/payments/return?pid=%d", p.ID))
	}
	c.Type("html", "utf-8")
	return c.SendString(fmt.Sprintf(`<!doctype html><html lang="tk"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Demo bank</title>`+
		`<style>body{font-family:system-ui,sans-serif;background:#eef1f6;margin:0;display:grid;place-items:center;min-height:100vh}.c{background:#fff;border-radius:18px;padding:24px;max-width:360px;margin:16px;box-shadow:0 4px 18px #0001}`+
		`input{width:100%%;box-sizing:border-box;padding:12px;margin:6px 0;border:1px solid #ccd;border-radius:10px;font-size:16px}button{width:100%%;padding:14px;border:0;border-radius:12px;font-size:16px;font-weight:700;margin-top:8px}`+
		`.p{background:#1E9E5A;color:#fff}.x{background:#eee}.w{background:#fff3cd;padding:8px;border-radius:8px;font-size:13px}</style></head><body><form class="c" method="post">`+
		`<p class="w">DEMO bank — hakyky pul alynmaýar. Önümçilikde PAYMENT_PROVIDER=rbs.</p><h2>%.2f %s</h2>`+
		`<input placeholder="Kart belgisi" value="9934 0000 0000 0000"><input placeholder="AA/ÝÝ" value="12/30">`+
		`<button class="p" name="action" value="pay">Töle</button><button class="x" name="action" value="cancel">Ýatyr</button></form></body></html>`,
		p.Amount, html.EscapeString(h.Cfg.Currency)))
}

// DemoPaySubmit POST /payments/demo/:id {action: pay|cancel}
func (h *Handler) DemoPaySubmit(c *fiber.Ctx) error {
	p, err := h.demoPayment(c)
	if err != nil {
		return paymentPage(c, 404, "Töleg tapylmady", "")
	}
	if p.Status == models.PaymentPending {
		h.settlePayment(p, c.FormValue("action") == "pay", "ulanyjy ýatyrdy")
	}
	return c.Redirect(fmt.Sprintf("/api/v1/payments/return?pid=%d", p.ID), fiber.StatusSeeOther)
}

func (h *Handler) demoPayment(c *fiber.Ctx) (*models.Payment, error) {
	if h.Gateway == nil || h.Gateway.Name() != "demo" {
		return nil, errNotFound
	}
	id, err := idParam(c, "id")
	if err != nil {
		return nil, err
	}
	var p models.Payment
	if err := h.DB.Where("id = ? AND provider = ?", id, "demo").First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// DriverRenew POST /driver/subscription — monthly plan: pay the next 30 days from the balance now.
func (h *Handler) DriverRenew(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	if h.Billing == nil || d.PaymentPlan != models.PlanMonthly {
		return utils.Err(c, 400, "siziň tölegiňiz göterim boýunça, aýlyk töleg gerek däl")
	}
	if err := h.Billing.Renew(d); err != nil {
		if errors.Is(err, services.ErrNeedTopUp) {
			return utils.Err(c, 402, err.Error())
		}
		return utils.Err(c, 500, "töleg geçmedi")
	}
	d.User = nil
	return utils.OK(c, d)
}

// billingBlocked is used before a driver goes online or takes an order. When the driver may not
// work it writes the 402 response and returns true; the caller then returns the second value.
func (h *Handler) billingBlocked(c *fiber.Ctx, d *models.Driver) (bool, error) {
	if h.Billing == nil {
		return false, nil
	}
	err := h.Billing.CanWork(d)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, services.ErrNeedTopUp) {
		return true, c.Status(402).JSON(fiber.Map{"success": false, "error": err.Error(), "data": fiber.Map{"need_topup": true, "plan": d.PaymentPlan}})
	}
	return true, utils.Err(c, 500, "töleg barlanmady")
}

// AdminPayments GET /admin/payments?status=&page= — card top-ups.
func (h *Handler) AdminPayments(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	q := h.DB.Model(&models.Payment{})
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	var total int64
	q.Count(&total)
	var ps []models.Payment
	q.Preload("Driver").Preload("Driver.User").Order("id desc").Limit(limit).Offset(offset).Find(&ps)
	var paidToday float64
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	h.DB.Model(&models.Payment{}).Where("status = ? AND paid_at >= ?", models.PaymentPaid, day).Select("COALESCE(SUM(amount),0)").Scan(&paidToday)
	return utils.OK(c, fiber.Map{"items": ps, "total": total, "paid_today": paidToday, "provider": h.providerName()})
}

func (h *Handler) providerName() string {
	if h.Gateway == nil {
		return ""
	}
	return h.Gateway.Name()
}

// truncate cuts to n runes (Turkmen letters are multi-byte).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
