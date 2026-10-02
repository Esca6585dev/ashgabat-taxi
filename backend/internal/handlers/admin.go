package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// AdminStats GET /admin/stats
func (h *Handler) AdminStats(c *fiber.Ctx) error {
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var (
		users, drivers, driversOnline, driversPending, ordersTotal, ordersToday, activeOrders, completedToday, openTickets int64
		revenueToday, commissionToday, revenueTotal                                                                          float64
	)
	h.DB.Model(&models.User{}).Where("role = ?", models.RoleClient).Count(&users)
	h.DB.Model(&models.Driver{}).Count(&drivers)
	h.DB.Model(&models.Driver{}).Where("is_online = ?", true).Count(&driversOnline)
	h.DB.Model(&models.Driver{}).Where("status = ?", models.DriverPending).Count(&driversPending)
	h.DB.Model(&models.Order{}).Count(&ordersTotal)
	h.DB.Model(&models.Order{}).Where("created_at >= ?", day).Count(&ordersToday)
	h.DB.Model(&models.Order{}).Where("status IN ?", models.ActiveStatuses).Count(&activeOrders)
	h.DB.Model(&models.Order{}).Where("status = ? AND completed_at >= ?", models.OrderCompleted, day).Count(&completedToday)
	h.DB.Model(&models.SupportTicket{}).Where("status = ?", "open").Count(&openTickets)
	h.DB.Model(&models.Order{}).Where("status = ? AND completed_at >= ?", models.OrderCompleted, day).Select("COALESCE(SUM(final_price),0)").Scan(&revenueToday)
	h.DB.Model(&models.Order{}).Where("status = ? AND completed_at >= ?", models.OrderCompleted, day).Select("COALESCE(SUM(commission),0)").Scan(&commissionToday)
	h.DB.Model(&models.Order{}).Where("status = ?", models.OrderCompleted).Select("COALESCE(SUM(final_price),0)").Scan(&revenueTotal)

	type dayStat struct {
		Date    string  `json:"date"`
		Orders  int64   `json:"orders"`
		Revenue float64 `json:"revenue"`
	}
	days := make([]dayStat, 0, 14)
	for i := 13; i >= 0; i-- {
		from := day.AddDate(0, 0, -i)
		to := from.AddDate(0, 0, 1)
		var ds dayStat
		ds.Date = from.Format("2006-01-02")
		h.DB.Model(&models.Order{}).Where("created_at >= ? AND created_at < ?", from, to).Count(&ds.Orders)
		h.DB.Model(&models.Order{}).Where("status = ? AND completed_at >= ? AND completed_at < ?", models.OrderCompleted, from, to).Select("COALESCE(SUM(final_price),0)").Scan(&ds.Revenue)
		days = append(days, ds)
	}
	type st struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var byStatus []st
	h.DB.Model(&models.Order{}).Select("status, COUNT(*) as count").Group("status").Scan(&byStatus)

	return utils.OK(c, fiber.Map{
		"clients": users, "drivers": drivers, "drivers_online": driversOnline, "drivers_pending": driversPending,
		"orders_total": ordersTotal, "orders_today": ordersToday, "active_orders": activeOrders, "completed_today": completedToday,
		"revenue_today": revenueToday, "commission_today": commissionToday, "revenue_total": revenueTotal, "open_tickets": openTickets,
		"days": days, "by_status": byStatus, "ws_connections": h.Hub.ConnectedCount(), "currency": h.Cfg.Currency,
	})
}

// AdminUsers GET /admin/users?q=&role=&page=
func (h *Handler) AdminUsers(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	q := h.DB.Model(&models.User{})
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = q.Where("phone LIKE ? OR name LIKE ?", "%"+s+"%", "%"+s+"%")
	}
	if r := c.Query("role"); r != "" {
		q = q.Where("role = ?", r)
	}
	var total int64
	q.Count(&total)
	var users []models.User
	q.Order("id desc").Limit(limit).Offset(offset).Find(&users)
	return utils.OK(c, fiber.Map{"items": users, "total": total})
}

// AdminBlockUser PUT /admin/users/:id/block {blocked: bool}
func (h *Handler) AdminBlockUser(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		Blocked bool `json:"blocked"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	res := h.DB.Model(&models.User{}).Where("id = ? AND role <> ?", id, models.RoleAdmin).Update("is_blocked", req.Blocked)
	if res.RowsAffected == 0 {
		return utils.Err(c, 404, "ulanyjy tapylmady")
	}
	if req.Blocked {
		h.DB.Model(&models.Driver{}).Where("user_id = ?", id).Update("is_online", false)
		h.Hub.SendTo(id, "account.blocked", nil)
	}
	return utils.OK(c, true)
}

// AdminDrivers GET /admin/drivers?status=&online=1&q=
func (h *Handler) AdminDrivers(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	q := h.DB.Model(&models.Driver{}).Preload("User")
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if c.Query("online") == "1" {
		q = q.Where("is_online = ?", true)
	}
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = q.Joins("JOIN users ON users.id = drivers.user_id").Where("users.phone LIKE ? OR users.name LIKE ? OR drivers.car_plate LIKE ?", "%"+s+"%", "%"+s+"%", "%"+s+"%")
	}
	var total int64
	q.Count(&total)
	var drivers []models.Driver
	q.Order("drivers.id desc").Limit(limit).Offset(offset).Find(&drivers)
	return utils.OK(c, fiber.Map{"items": drivers, "total": total})
}

// AdminOnlineDrivers GET /admin/drivers/online — for the live map.
func (h *Handler) AdminOnlineDrivers(c *fiber.Ctx) error {
	var drivers []models.Driver
	h.DB.Preload("User").Where("is_online = ?", true).Find(&drivers)
	return utils.OK(c, drivers)
}

// AdminDriverStatus PUT /admin/drivers/:id/status {status, reason, commission_rate, tariff_codes}
func (h *Handler) AdminDriverStatus(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		Status         string   `json:"status"`
		Reason         string   `json:"reason"`
		CommissionRate *float64 `json:"commission_rate"`
		TariffCodes    *string  `json:"tariff_codes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	var d models.Driver
	if err := h.DB.First(&d, id).Error; err != nil {
		return utils.Err(c, 404, "sürüji tapylmady")
	}
	upd := map[string]any{}
	switch models.DriverStatus(req.Status) {
	case models.DriverApproved, models.DriverRejected, models.DriverBlocked, models.DriverPending:
		upd["status"] = req.Status
		upd["reject_reason"] = req.Reason
		if req.Status != string(models.DriverApproved) {
			upd["is_online"] = false
		}
	case "":
	default:
		return utils.Err(c, 400, "nädogry status")
	}
	if req.CommissionRate != nil {
		upd["commission_rate"] = *req.CommissionRate
	}
	if req.TariffCodes != nil {
		upd["tariff_codes"] = *req.TariffCodes
	}
	h.DB.Model(&d).Updates(upd)
	h.DB.Preload("User").First(&d, id)
	h.Hub.SendTo(d.UserID, "driver.status", d)
	return utils.OK(c, d)
}

// AdminDriverBalance POST /admin/drivers/:id/balance {amount, note}
func (h *Handler) AdminDriverBalance(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := c.BodyParser(&req); err != nil || req.Amount == 0 {
		return utils.Err(c, 400, "mukdar gerek")
	}
	typ := "topup"
	if req.Amount < 0 {
		typ = "payout"
	}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Driver{}).Where("id = ?", id).Update("balance", gorm.Expr("balance + ?", req.Amount))
		if res.RowsAffected == 0 {
			return errNotFound
		}
		return tx.Create(&models.Transaction{DriverID: id, Amount: req.Amount, Type: typ, Note: req.Note}).Error
	})
	if err != nil {
		return utils.Err(c, 404, "sürüji tapylmady")
	}
	var d models.Driver
	h.DB.Preload("User").First(&d, id)
	h.Hub.SendTo(d.UserID, "balance.changed", fiber.Map{"balance": d.Balance, "amount": req.Amount})
	return utils.OK(c, d)
}

// AdminOrders GET /admin/orders?status=&q=&page=
func (h *Handler) AdminOrders(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	q := h.DB.Model(&models.Order{}).Preload("Client").Preload("Driver").Preload("Driver.User").Preload("Tariff")
	if s := c.Query("status"); s == "active" {
		q = q.Where("status IN ?", models.ActiveStatuses)
	} else if s != "" {
		q = q.Where("status = ?", s)
	}
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = q.Where("pickup_address LIKE ? OR dropoff_address LIKE ? OR id = ?", "%"+s+"%", "%"+s+"%", s)
	}
	var total int64
	q.Count(&total)
	var orders []models.Order
	q.Order("id desc").Limit(limit).Offset(offset).Find(&orders)
	return utils.OK(c, fiber.Map{"items": orders, "total": total})
}

// AdminAssignOrder POST /admin/orders/:id/assign {driver_id}
func (h *Handler) AdminAssignOrder(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		DriverID uint `json:"driver_id"`
	}
	if err := c.BodyParser(&req); err != nil || req.DriverID == 0 {
		return utils.Err(c, 400, "driver_id gerek")
	}
	var d models.Driver
	if err := h.DB.First(&d, req.DriverID).Error; err != nil || d.Status != models.DriverApproved || d.CurrentOrderID != nil {
		return utils.Err(c, 400, "sürüji elýeterli däl")
	}
	now := time.Now()
	res := h.DB.Model(&models.Order{}).Where("id = ? AND status = ?", id, models.OrderSearching).
		Updates(map[string]any{"driver_id": d.ID, "status": models.OrderAccepted, "accepted_at": now})
	if res.RowsAffected == 0 {
		return utils.Err(c, 400, "sargyt gözleg ýagdaýynda däl")
	}
	h.DB.Model(&d).Updates(map[string]any{"current_order_id": id, "is_online": true})
	h.Dispatch.Clear(id)
	o, _ := h.loadOrder(id)
	h.pushOrder(o, "order.accepted")
	return utils.OK(c, o)
}

// AdminCancelOrder POST /admin/orders/:id/cancel {reason}
func (h *Handler) AdminCancelOrder(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	o, err := h.loadOrder(id)
	if err != nil {
		return utils.Err(c, 404, "sargyt tapylmady")
	}
	if !o.IsActive() {
		return utils.Err(c, 400, "sargyt eýýäm ýapyk")
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&req)
	h.cancel(o, "admin", req.Reason)
	return utils.OK(c, o)
}

// AdminTariffSave POST /admin/tariffs (create) / PUT /admin/tariffs/:id
func (h *Handler) AdminTariffSave(c *fiber.Ctx) error {
	var t models.Tariff
	if err := c.BodyParser(&t); err != nil || strings.TrimSpace(t.Code) == "" {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	if c.Params("id") != "" {
		id, err := idParam(c, "id")
		if err != nil {
			return utils.Err(c, 400, err.Error())
		}
		t.ID = id
		if err := h.DB.Model(&models.Tariff{}).Where("id = ?", id).Select("*").Omit("id", "created_at").Updates(&t).Error; err != nil {
			return utils.Err(c, 500, "ýatda saklap bolmady")
		}
	} else {
		t.ID = 0
		if err := h.DB.Create(&t).Error; err != nil {
			return utils.Err(c, 400, "tarif kody gaýtalanýar")
		}
	}
	h.DB.First(&t, t.ID)
	return utils.OK(c, t)
}

// AdminTariffDelete DELETE /admin/tariffs/:id
func (h *Handler) AdminTariffDelete(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	h.DB.Model(&models.Tariff{}).Where("id = ?", id).Update("is_active", false)
	return utils.OK(c, true)
}

// AdminPromos GET /admin/promos
func (h *Handler) AdminPromos(c *fiber.Ctx) error {
	var ps []models.PromoCode
	h.DB.Order("id desc").Find(&ps)
	return utils.OK(c, ps)
}

// AdminPromoSave POST /admin/promos / PUT /admin/promos/:id
func (h *Handler) AdminPromoSave(c *fiber.Ctx) error {
	var p models.PromoCode
	if err := c.BodyParser(&p); err != nil || strings.TrimSpace(p.Code) == "" {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	p.Code = strings.ToUpper(strings.TrimSpace(p.Code))
	if c.Params("id") != "" {
		id, err := idParam(c, "id")
		if err != nil {
			return utils.Err(c, 400, err.Error())
		}
		p.ID = id
		if err := h.DB.Model(&models.PromoCode{}).Where("id = ?", id).Select("code", "discount_percent", "discount_fixed", "max_uses", "expires_at", "is_active").Updates(&p).Error; err != nil {
			return utils.Err(c, 500, "ýatda saklap bolmady")
		}
	} else {
		p.ID, p.Uses = 0, 0
		if err := h.DB.Create(&p).Error; err != nil {
			return utils.Err(c, 400, "promokod gaýtalanýar")
		}
	}
	h.DB.First(&p, p.ID)
	return utils.OK(c, p)
}

// AdminPromoDelete DELETE /admin/promos/:id
func (h *Handler) AdminPromoDelete(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	h.DB.Delete(&models.PromoCode{}, id)
	return utils.OK(c, true)
}

// AdminTickets GET /admin/tickets?status=
func (h *Handler) AdminTickets(c *fiber.Ctx) error {
	limit, offset := pageParams(c)
	q := h.DB.Model(&models.SupportTicket{}).Preload("User")
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	var total int64
	q.Count(&total)
	var ts []models.SupportTicket
	q.Order("id desc").Limit(limit).Offset(offset).Find(&ts)
	return utils.OK(c, fiber.Map{"items": ts, "total": total})
}

// AdminTicketReply PUT /admin/tickets/:id {reply, status}
func (h *Handler) AdminTicketReply(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		Reply  string `json:"reply"`
		Status string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	if req.Status == "" {
		req.Status = "resolved"
	}
	var t models.SupportTicket
	if err := h.DB.First(&t, id).Error; err != nil {
		return utils.Err(c, 404, "ýüztutma tapylmady")
	}
	h.DB.Model(&t).Updates(map[string]any{"reply": req.Reply, "status": req.Status})
	h.Hub.SendTo(t.UserID, "ticket.reply", t)
	return utils.OK(c, t)
}
