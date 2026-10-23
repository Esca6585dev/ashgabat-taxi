package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// DriverRegister POST /driver/register — create or update the driver profile (goes to moderation).
func (h *Handler) DriverRegister(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return utils.Err(c, 401, err.Error())
	}
	var req struct {
		Name          string `json:"name"`
		CarBrand      string `json:"car_brand"`
		CarModel      string `json:"car_model"`
		CarColor      string `json:"car_color"`
		CarPlate      string `json:"car_plate"`
		CarYear       int    `json:"car_year"`
		LicenseNumber string `json:"license_number"`
		TariffCodes   string `json:"tariff_codes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	if strings.TrimSpace(req.CarBrand) == "" || strings.TrimSpace(req.CarPlate) == "" || strings.TrimSpace(req.LicenseNumber) == "" {
		return utils.Err(c, 400, "awtoulag markasy, nomeri we sürüjilik şahadatnamasy hökmany")
	}
	if n := strings.TrimSpace(req.Name); n != "" {
		h.DB.Model(u).Update("name", n)
	}
	if u.Role != models.RoleDriver {
		h.DB.Model(u).Update("role", models.RoleDriver)
	}
	codes := strings.TrimSpace(req.TariffCodes)
	if codes == "" {
		codes = "economy"
	}
	d, err := h.driverOf(u.ID)
	if err != nil {
		d = &models.Driver{UserID: u.ID, Status: models.DriverPending, CommissionRate: h.Cfg.CommissionRate, PaymentPlan: h.Cfg.DefaultDriverPlan}
	}
	if d.Status == models.DriverBlocked {
		return utils.Err(c, 403, "sürüji hasabyňyz bloklanan")
	}
	if d.CurrentOrderID != nil {
		return utils.Err(c, 400, "işjeň sargyt wagtynda profil üýtgedip bolmaýar")
	}
	plate, license := strings.ToUpper(strings.TrimSpace(req.CarPlate)), strings.TrimSpace(req.LicenseNumber)
	// a new car or licence must be checked again by an admin
	recheck := d.Status == models.DriverApproved && (plate != d.CarPlate || license != d.LicenseNumber)
	d.CarBrand, d.CarModel, d.CarColor = strings.TrimSpace(req.CarBrand), strings.TrimSpace(req.CarModel), strings.TrimSpace(req.CarColor)
	d.CarPlate = plate
	d.CarYear, d.LicenseNumber, d.TariffCodes = req.CarYear, license, codes
	if d.Status == models.DriverRejected || recheck {
		d.Status, d.RejectReason, d.IsOnline = models.DriverPending, "", false
	}
	if err := h.DB.Save(d).Error; err != nil {
		return utils.Err(c, 500, "ýatda saklap bolmady")
	}
	h.Hub.BroadcastRole(string(models.RoleAdmin), "driver.registered", d)
	d.User = nil
	return utils.OK(c, d)
}

// DriverMe GET /driver/me
func (h *Handler) DriverMe(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok, hasaba alyň")
	}
	var active *models.Order
	if d.CurrentOrderID != nil {
		active, _ = h.loadOrder(*d.CurrentOrderID)
	}
	return utils.OK(c, fiber.Map{"driver": d, "active_order": active})
}

// DriverOnline POST /driver/online {online: bool, lat, lng}
func (h *Handler) DriverOnline(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	var req struct {
		Online bool    `json:"online"`
		Lat    float64 `json:"lat"`
		Lng    float64 `json:"lng"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	if req.Online && d.Status != models.DriverApproved {
		return utils.Err(c, 403, "profiliňiz entek tassyklanmady")
	}
	if !req.Online && d.CurrentOrderID != nil {
		return utils.Err(c, 400, "işjeň sargyt wagtynda oflaýn bolup bolmaýar")
	}
	if req.Online {
		if blocked, err := h.billingBlocked(c, d); blocked {
			return err
		}
	}
	upd := map[string]any{"is_online": req.Online, "last_seen_at": time.Now()}
	if req.Lat != 0 || req.Lng != 0 {
		upd["lat"], upd["lng"] = req.Lat, req.Lng
	}
	h.DB.Model(d).Updates(upd)
	d.IsOnline = req.Online
	h.Hub.BroadcastRole(string(models.RoleAdmin), "driver.online", fiber.Map{"driver_id": d.ID, "online": req.Online})
	return utils.OK(c, fiber.Map{"online": d.IsOnline})
}

// DriverLocation POST /driver/location {lat, lng, heading} (also accepted over WebSocket)
func (h *Handler) DriverLocation(c *fiber.Ctx) error {
	var req struct {
		Lat     float64 `json:"lat"`
		Lng     float64 `json:"lng"`
		Heading float64 `json:"heading"`
	}
	if err := c.BodyParser(&req); err != nil || (req.Lat == 0 && req.Lng == 0) {
		return utils.Err(c, 400, "koordinata gerek")
	}
	h.updateDriverLocation(middleware.UID(c), req.Lat, req.Lng, req.Heading)
	return utils.OK(c, true)
}

// updateDriverLocation persists the position and relays it to the active client.
func (h *Handler) updateDriverLocation(userID uint, lat, lng, heading float64) {
	var d models.Driver
	if err := h.DB.Where("user_id = ?", userID).First(&d).Error; err != nil {
		return
	}
	h.DB.Model(&d).Updates(map[string]any{"lat": lat, "lng": lng, "heading": heading, "last_seen_at": time.Now()})
	if d.CurrentOrderID != nil {
		var o models.Order
		if err := h.DB.Select("id", "client_id", "status").First(&o, *d.CurrentOrderID).Error; err == nil && o.IsActive() {
			h.Hub.SendTo(o.ClientID, "driver.location", fiber.Map{"order_id": o.ID, "lat": lat, "lng": lng, "heading": heading})
		}
	}
	h.Hub.BroadcastRole(string(models.RoleAdmin), "driver.location", fiber.Map{"driver_id": d.ID, "lat": lat, "lng": lng, "heading": heading, "online": d.IsOnline})
}

// AvailableOrders GET /driver/orders/available — searching orders near the driver (poll fallback for WS).
func (h *Handler) AvailableOrders(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	if d.Status != models.DriverApproved || !d.IsOnline {
		return utils.OK(c, []any{})
	}
	var orders []models.Order
	h.DB.Preload("Client").Preload("Tariff").Where("status = ?", models.OrderSearching).Order("id desc").Limit(50).Find(&orders)
	type item struct {
		Order      models.Order `json:"order"`
		DistanceKm float64      `json:"distance_km"`
	}
	out := []item{}
	now := time.Now()
	for _, o := range orders {
		if !services.DispatchDue(&o, now) {
			continue // booked for later: not offered yet
		}
		dist := utils.Haversine(d.Lat, d.Lng, o.PickupLat, o.PickupLng)
		if dist <= h.Cfg.DispatchMaxRadiusKm && (o.Tariff == nil || strings.Contains(d.TariffCodes, o.Tariff.Code)) {
			out = append(out, item{Order: o, DistanceKm: utils.RoundMoney(dist*20) / 20})
		}
	}
	return utils.OK(c, out)
}

// AcceptOrder POST /driver/orders/:id/accept
func (h *Handler) AcceptOrder(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	if d.Status != models.DriverApproved {
		return utils.Err(c, 403, "profiliňiz tassyklanmady")
	}
	if d.CurrentOrderID != nil {
		return utils.Err(c, 400, "sizde eýýäm işjeň sargyt bar")
	}
	if blocked, err := h.billingBlocked(c, d); blocked {
		return err
	}
	now := time.Now()
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Order{}).Where("id = ? AND status = ? AND driver_id IS NULL", id, models.OrderSearching).
			Updates(map[string]any{"driver_id": d.ID, "status": models.OrderAccepted, "accepted_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNotFound
		}
		return tx.Model(&models.Driver{}).Where("id = ?", d.ID).Updates(map[string]any{"current_order_id": id, "is_online": true, "last_seen_at": now}).Error
	})
	if err != nil {
		return utils.Err(c, 409, "sargyt eýýäm alnan ýa-da ýatyrylan")
	}
	notified := h.Dispatch.NotifiedDrivers(id)
	h.Dispatch.Clear(id)
	o, _ := h.loadOrder(id)
	h.pushOrder(o, "order.accepted")
	for _, uid := range notified {
		if uid != d.UserID {
			h.Hub.SendTo(uid, "order.taken", fiber.Map{"order_id": id})
		}
	}
	return utils.OK(c, o)
}

// DeclineOrder POST /driver/orders/:id/decline
func (h *Handler) DeclineOrder(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	h.Dispatch.Decline(id, middleware.UID(c))
	return utils.OK(c, true)
}

// driverOrderTransition loads the driver's current order and checks the expected status.
func (h *Handler) driverOrderTransition(c *fiber.Ctx, from ...models.OrderStatus) (*models.Driver, *models.Order, error) {
	id, err := idParam(c, "id")
	if err != nil {
		return nil, nil, err
	}
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return nil, nil, err
	}
	o, err := h.loadOrder(id)
	if err != nil || o.DriverID == nil || *o.DriverID != d.ID {
		return nil, nil, errNotFound
	}
	ok := false
	for _, s := range from {
		if o.Status == s {
			ok = true
		}
	}
	if !ok {
		return nil, nil, fiber.NewError(400, "sargydyň ýagdaýy muňa rugsat bermeýär: "+string(o.Status))
	}
	return d, o, nil
}

// ArrivedOrder POST /driver/orders/:id/arrived
func (h *Handler) ArrivedOrder(c *fiber.Ctx) error {
	_, o, err := h.driverOrderTransition(c, models.OrderAccepted)
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	now := time.Now()
	h.DB.Model(o).Updates(map[string]any{"status": models.OrderArrived, "arrived_at": now})
	o.Status, o.ArrivedAt = models.OrderArrived, &now
	h.pushOrder(o, "order.status")
	return utils.OK(c, o)
}

// StartOrder POST /driver/orders/:id/start
func (h *Handler) StartOrder(c *fiber.Ctx) error {
	_, o, err := h.driverOrderTransition(c, models.OrderArrived, models.OrderAccepted)
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	now := time.Now()
	h.DB.Model(o).Updates(map[string]any{"status": models.OrderInProgress, "started_at": now})
	o.Status, o.StartedAt = models.OrderInProgress, &now
	h.pushOrder(o, "order.status")
	return utils.OK(c, o)
}

// CompleteOrder POST /driver/orders/:id/complete — fixed fare + waiting fee, commission, balance.
func (h *Handler) CompleteOrder(c *fiber.Ctx) error {
	d, o, err := h.driverOrderTransition(c, models.OrderInProgress)
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	now := time.Now()
	final := o.EstimatedPrice - o.Discount
	if o.Tariff != nil && o.ArrivedAt != nil && o.StartedAt != nil {
		wait := o.StartedAt.Sub(*o.ArrivedAt).Minutes() - float64(o.Tariff.FreeWaitMin)
		if wait > 0 {
			final += utils.RoundMoney(wait * o.Tariff.PerWaitMin)
		}
	}
	if final < 0 {
		final = 0
	}
	rate := d.CommissionRate
	if rate <= 0 {
		rate = h.Cfg.CommissionRate
	}
	if d.PaymentPlan == models.PlanMonthly {
		rate = 0 // monthly plan: the fee is paid per month, rides are commission-free
	}
	commission := utils.RoundMoney(final * rate)
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Order{}).Where("id = ?", o.ID).Updates(map[string]any{
			"status": models.OrderCompleted, "completed_at": now, "final_price": final, "commission": commission, "is_paid": true,
		}).Error; err != nil {
			return err
		}
		delta := -commission // cash: driver keeps the fare, owes commission
		txs := []models.Transaction{{DriverID: d.ID, OrderID: &o.ID, Amount: -commission, Type: "commission", Note: "Sargyt komissiýasy"}}
		if o.PaymentMethod == "card" {
			delta = final - commission
			txs = append(txs, models.Transaction{DriverID: d.ID, OrderID: &o.ID, Amount: final, Type: "ride_income", Note: "Kart bilen töleg"})
		}
		if err := tx.Create(&txs).Error; err != nil {
			return err
		}
		return tx.Model(&models.Driver{}).Where("id = ?", d.ID).Updates(map[string]any{
			"current_order_id": nil, "balance": gorm.Expr("balance + ?", delta), "total_rides": gorm.Expr("total_rides + 1"), "last_seen_at": now,
		}).Error
	})
	if err != nil {
		return utils.Err(c, 500, "tamamlap bolmady")
	}
	o.Status, o.CompletedAt, o.FinalPrice, o.Commission, o.IsPaid = models.OrderCompleted, &now, final, commission, true
	h.pushOrder(o, "order.completed")
	return utils.OK(c, o)
}

// DriverCancelOrder POST /driver/orders/:id/cancel {reason} — order returns to the search pool.
func (h *Handler) DriverCancelOrder(c *fiber.Ctx) error {
	d, o, err := h.driverOrderTransition(c, models.OrderAccepted, models.OrderArrived)
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&req)
	h.DB.Model(&models.Driver{}).Where("id = ?", d.ID).Update("current_order_id", nil)
	// requeue: back to searching with no driver, fresh created_at so the timeout restarts
	h.DB.Model(&models.Order{}).Where("id = ?", o.ID).Updates(map[string]any{
		"status": models.OrderSearching, "driver_id": nil, "accepted_at": nil, "arrived_at": nil, "created_at": time.Now(),
	})
	h.Dispatch.Decline(o.ID, d.UserID)
	o.Status, o.DriverID, o.Driver, o.AcceptedAt, o.ArrivedAt = models.OrderSearching, nil, nil, nil, nil
	h.Hub.SendTo(o.ClientID, "order.requeued", fiber.Map{"order": o, "reason": req.Reason})
	h.Hub.BroadcastRole(string(models.RoleAdmin), "order.status", o)
	go h.Dispatch.OfferOrder(o)
	return utils.OK(c, o)
}

// DriverOrders GET /driver/orders?page= — completed / cancelled history.
func (h *Handler) DriverOrders(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	limit, offset := pageParams(c)
	var orders []models.Order
	h.DB.Preload("Client").Preload("Tariff").Where("driver_id = ? AND status IN ?", d.ID, []models.OrderStatus{models.OrderCompleted, models.OrderCancelled}).
		Order("id desc").Limit(limit).Offset(offset).Find(&orders)
	return utils.OK(c, orders)
}

// DriverEarnings GET /driver/earnings
func (h *Handler) DriverEarnings(c *fiber.Ctx) error {
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	week := day.AddDate(0, 0, -int(now.Weekday()+6)%7)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	type agg struct {
		Rides  int     `json:"rides"`
		Income float64 `json:"income"`
	}
	sum := func(since time.Time) agg {
		var a agg
		h.DB.Model(&models.Order{}).Select("COUNT(*) as rides, COALESCE(SUM(final_price),0) as income").
			Where("driver_id = ? AND status = ? AND completed_at >= ?", d.ID, models.OrderCompleted, since).Scan(&a)
		return a
	}
	var txs []models.Transaction
	h.DB.Where("driver_id = ?", d.ID).Order("id desc").Limit(30).Find(&txs)
	return utils.OK(c, fiber.Map{
		"today": sum(day), "week": sum(week), "month": sum(month),
		"balance": d.Balance, "total_rides": d.TotalRides, "rating": d.User.Rating,
		"commission_rate": d.CommissionRate, "transactions": txs, "currency": h.Cfg.Currency,
		"payment_plan": d.PaymentPlan, "monthly_fee": h.monthlyFee(d), "subscription_until": d.SubscriptionUntil,
		"min_balance": h.Cfg.DriverMinBalance, "card_topup": h.Gateway != nil, "topup_min": h.Cfg.TopUpMin, "topup_max": h.Cfg.TopUpMax,
	})
}

// RateClient POST /driver/orders/:id/rate {rating}
func (h *Handler) RateClient(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	d, err := h.driverOf(middleware.UID(c))
	if err != nil {
		return utils.Err(c, 404, "sürüji profili ýok")
	}
	o, err := h.loadOrder(id)
	if err != nil || o.DriverID == nil || *o.DriverID != d.ID || o.Status != models.OrderCompleted {
		return utils.Err(c, 404, "sargyt tapylmady")
	}
	if o.DriverRating > 0 {
		return utils.Err(c, 400, "eýýäm baha berdiňiz")
	}
	var req struct {
		Rating int `json:"rating"`
	}
	if err := c.BodyParser(&req); err != nil || req.Rating < 1 || req.Rating > 5 {
		return utils.Err(c, 400, "baha 1-5 aralygynda bolmaly")
	}
	h.DB.Model(o).Update("driver_rating", req.Rating)
	h.applyRating(o.ClientID, req.Rating)
	return utils.OK(c, true)
}

func (h *Handler) monthlyFee(d *models.Driver) float64 {
	if h.Billing != nil {
		return h.Billing.Fee(d)
	}
	return h.Cfg.MonthlyFee
}
