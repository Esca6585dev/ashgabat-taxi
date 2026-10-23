package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

type pointReq struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Address string  `json:"address"`
}

func (p pointReq) valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180 && (p.Lat != 0 || p.Lng != 0)
}

// Estimate POST /orders/estimate {pickup, dropoff} -> price for every active tariff
func (h *Handler) Estimate(c *fiber.Ctx) error {
	var req struct {
		Pickup  pointReq `json:"pickup"`
		Dropoff pointReq `json:"dropoff"`
	}
	if err := c.BodyParser(&req); err != nil || !req.Pickup.valid() || !req.Dropoff.valid() {
		return utils.Err(c, 400, "pickup we dropoff koordinatalary gerek")
	}
	route := h.Router.Route(services.LatLng{Lat: req.Pickup.Lat, Lng: req.Pickup.Lng}, services.LatLng{Lat: req.Dropoff.Lat, Lng: req.Dropoff.Lng})
	var tariffs []models.Tariff
	h.DB.Where("is_active = ?", true).Order("sort_order").Find(&tariffs)
	now := time.Now()
	type opt struct {
		Tariff models.Tariff `json:"tariff"`
		Price  float64       `json:"price"`
	}
	opts := make([]opt, 0, len(tariffs))
	for _, t := range tariffs {
		opts = append(opts, opt{Tariff: t, Price: h.Pricing.Price(&t, route.DistanceKm, route.DurationMin, now)})
	}
	nearby := h.Dispatch.NearbyDrivers(req.Pickup.Lat, req.Pickup.Lng, h.Cfg.DispatchMaxRadiusKm, "")
	eta := 0
	if len(nearby) > 0 {
		eta = int(nearby[0].EtaMin + 1)
	}
	return utils.OK(c, fiber.Map{
		"distance_km":    utils.RoundMoney(route.DistanceKm*20) / 20,
		"duration_min":   int(route.DurationMin + 0.5),
		"polyline":       route.Polyline,
		"route_source":   route.Source,
		"is_night":       services.IsNight(now),
		"options":        opts,
		"drivers_nearby": len(nearby),
		"pickup_eta_min": eta,
		"currency":       h.Cfg.Currency,
	})
}

// CreateOrder POST /orders
func (h *Handler) CreateOrder(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return utils.Err(c, 401, err.Error())
	}
	var req struct {
		TariffID       uint       `json:"tariff_id"`
		Pickup         pointReq   `json:"pickup"`
		Dropoff        pointReq   `json:"dropoff"`
		PaymentMethod  string     `json:"payment_method"`
		PromoCode      string     `json:"promo_code"`
		Comment        string     `json:"comment"`
		OrderType      string     `json:"order_type"`
		RecipientPhone string     `json:"recipient_phone"`
		ScheduledAt    *time.Time `json:"scheduled_at"`
	}
	if err := c.BodyParser(&req); err != nil || !req.Pickup.valid() || !req.Dropoff.valid() {
		return utils.Err(c, 400, "pickup we dropoff koordinatalary gerek")
	}
	if active := h.activeOrderOfClient(u.ID); active != nil {
		return c.Status(409).JSON(fiber.Map{"success": false, "error": "sizde eýýäm işjeň sargyt bar", "data": active})
	}
	var tariff models.Tariff
	if err := h.DB.Where("id = ? AND is_active = ?", req.TariffID, true).First(&tariff).Error; err != nil {
		return utils.Err(c, 400, "tarif tapylmady")
	}
	route := h.Router.Route(services.LatLng{Lat: req.Pickup.Lat, Lng: req.Pickup.Lng}, services.LatLng{Lat: req.Dropoff.Lat, Lng: req.Dropoff.Lng})
	if route.DistanceKm < 0.2 {
		return utils.Err(c, 400, "barmaly ýer gaty ýakyn")
	}
	at := time.Now()
	if req.ScheduledAt != nil {
		switch {
		case req.ScheduledAt.After(at.AddDate(0, 0, 7)):
			return utils.Err(c, 400, "iň köp 7 gün öňünden sargyt edip bolýar")
		case req.ScheduledAt.Before(at.Add(services.ScheduledLead)):
			req.ScheduledAt = nil // sooner than the dispatch lead: just a normal ride now
		default:
			at = *req.ScheduledAt
		}
	}
	price := h.Pricing.Price(&tariff, route.DistanceKm, route.DurationMin, at)

	var promo *models.PromoCode
	discount := 0.0
	if code := strings.ToUpper(strings.TrimSpace(req.PromoCode)); code != "" {
		var p models.PromoCode
		if err := h.DB.Where("code = ?", code).First(&p).Error; err != nil || !services.PromoValid(&p) {
			return utils.Err(c, 400, "promokod nädogry ýa-da möhleti geçen")
		}
		promo = &p
		discount = services.ApplyPromo(&p, price)
	}
	pm := "cash"
	if req.PaymentMethod == "card" {
		if !h.Cfg.RideCardPayment {
			return utils.Err(c, 400, "kart bilen töleg häzirlikçe elýeterli däl, nagt saýlaň")
		}
		pm = "card"
	}
	ot := "ride"
	if req.OrderType == "delivery" || tariff.Code == "delivery" {
		ot = "delivery"
	}
	o := models.Order{
		ClientID: u.ID, TariffID: tariff.ID, Status: models.OrderSearching, OrderType: ot,
		PickupLat: req.Pickup.Lat, PickupLng: req.Pickup.Lng, PickupAddress: strings.TrimSpace(req.Pickup.Address),
		DropoffLat: req.Dropoff.Lat, DropoffLng: req.Dropoff.Lng, DropoffAddress: strings.TrimSpace(req.Dropoff.Address),
		DistanceKm: route.DistanceKm, DurationMin: route.DurationMin,
		EstimatedPrice: price, Discount: discount, FinalPrice: price - discount,
		PaymentMethod: pm, Comment: strings.TrimSpace(req.Comment),
		RecipientPhone: utils.NormalizePhone(req.RecipientPhone), ScheduledAt: req.ScheduledAt,
	}
	if promo != nil {
		o.PromoCodeID = &promo.ID
	}
	if o.PickupAddress == "" {
		if p, err := h.Geo.Reverse(o.PickupLat, o.PickupLng); err == nil {
			o.PickupAddress = p.Name
		}
	}
	if o.DropoffAddress == "" {
		if p, err := h.Geo.Reverse(o.DropoffLat, o.DropoffLng); err == nil {
			o.DropoffAddress = p.Name
		}
	}
	if err := h.DB.Create(&o).Error; err != nil {
		return utils.Err(c, 500, "sargyt döredip bolmady")
	}
	if promo != nil {
		h.DB.Model(promo).Update("uses", promo.Uses+1)
	}
	full, _ := h.loadOrder(o.ID)
	if services.DispatchDue(full, time.Now()) {
		go h.Dispatch.OfferOrder(full)
	}
	h.Hub.BroadcastRole(string(models.RoleAdmin), "order.created", full)
	return c.Status(201).JSON(fiber.Map{"success": true, "data": full})
}

// ActiveOrder GET /orders/active
func (h *Handler) ActiveOrder(c *fiber.Ctx) error {
	o := h.activeOrderOfClient(middleware.UID(c))
	if o == nil {
		return utils.OK(c, nil)
	}
	return utils.OK(c, o)
}

// MyOrders GET /orders?page=&limit=
func (h *Handler) MyOrders(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	var orders []models.Order
	h.DB.Preload("Driver").Preload("Driver.User").Preload("Tariff").
		Where("client_id = ?", middleware.UID(c)).Order("id desc").Limit(limit).Offset(offset).Find(&orders)
	return utils.OK(c, orders)
}

// GetOrder GET /orders/:id (client, assigned driver or admin)
func (h *Handler) GetOrder(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	o, err := h.loadOrder(id)
	if err != nil {
		return utils.Err(c, 404, "sargyt tapylmady")
	}
	uid := middleware.UID(c)
	role, _ := c.Locals("role").(string)
	if o.ClientID != uid && role != string(models.RoleAdmin) && (o.Driver == nil || o.Driver.UserID != uid) {
		return utils.Err(c, 403, "rugsat ýok")
	}
	return utils.OK(c, o)
}

// CancelOrder POST /orders/:id/cancel {reason}
func (h *Handler) CancelOrder(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	o, err := h.loadOrder(id)
	if err != nil || o.ClientID != middleware.UID(c) {
		return utils.Err(c, 404, "sargyt tapylmady")
	}
	if o.Status == models.OrderInProgress || !o.IsActive() {
		return utils.Err(c, 400, "bu sargydy indi ýatyryp bolmaýar")
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&req)
	h.cancel(o, "client", req.Reason)
	return utils.OK(c, o)
}

// cancel finalises an order cancellation from any side and frees the driver.
func (h *Handler) cancel(o *models.Order, by, reason string) {
	now := time.Now()
	h.DB.Model(&models.Order{}).Where("id = ?", o.ID).Updates(map[string]any{
		"status": models.OrderCancelled, "cancelled_at": now, "cancelled_by": by, "cancel_reason": reason,
	})
	if o.DriverID != nil {
		h.DB.Model(&models.Driver{}).Where("id = ? AND current_order_id = ?", *o.DriverID, o.ID).Update("current_order_id", nil)
	}
	notified := h.Dispatch.NotifiedDrivers(o.ID)
	h.Dispatch.Clear(o.ID)
	o.Status, o.CancelledAt, o.CancelledBy, o.CancelReason = models.OrderCancelled, &now, by, reason
	h.pushOrder(o, "order.cancelled")
	h.Hub.SendToMany(notified, "order.taken", map[string]any{"order_id": o.ID})
}

// RateDriver POST /orders/:id/rate {rating 1-5, comment}
func (h *Handler) RateDriver(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	o, err := h.loadOrder(id)
	if err != nil || o.ClientID != middleware.UID(c) {
		return utils.Err(c, 404, "sargyt tapylmady")
	}
	if o.Status != models.OrderCompleted || o.Driver == nil {
		return utils.Err(c, 400, "diňe tamamlanan sargyda baha berip bolýar")
	}
	if o.ClientRating > 0 {
		return utils.Err(c, 400, "eýýäm baha berdiňiz")
	}
	var req struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if err := c.BodyParser(&req); err != nil || req.Rating < 1 || req.Rating > 5 {
		return utils.Err(c, 400, "baha 1-5 aralygynda bolmaly")
	}
	h.DB.Model(o).Updates(map[string]any{"client_rating": req.Rating, "client_comment": strings.TrimSpace(req.Comment)})
	h.applyRating(o.Driver.UserID, req.Rating)
	return utils.OK(c, true)
}

// CreateTicket POST /support {order_id?, message}
func (h *Handler) CreateTicket(c *fiber.Ctx) error {
	var req struct {
		OrderID *uint  `json:"order_id"`
		Message string `json:"message"`
	}
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		return utils.Err(c, 400, "habar boş bolmaly däl")
	}
	t := models.SupportTicket{UserID: middleware.UID(c), OrderID: req.OrderID, Message: strings.TrimSpace(req.Message), Status: "open"}
	h.DB.Create(&t)
	h.Hub.BroadcastRole(string(models.RoleAdmin), "ticket.new", t)
	return c.Status(201).JSON(fiber.Map{"success": true, "data": t})
}

// MyTickets GET /support
func (h *Handler) MyTickets(c *fiber.Ctx) error {
	var ts []models.SupportTicket
	h.DB.Where("user_id = ?", middleware.UID(c)).Order("id desc").Limit(50).Find(&ts)
	return utils.OK(c, ts)
}
