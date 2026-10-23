package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// Health GET /health
func (h *Handler) Health(c *fiber.Ctx) error {
	return utils.OK(c, fiber.Map{"status": "ok", "ws_users": h.Hub.ConnectedCount()})
}

// PublicConfig GET /config — constants the apps need before login.
func (h *Handler) PublicConfig(c *fiber.Ctx) error {
	return utils.OK(c, fiber.Map{
		"city":            "Aşgabat",
		"city_lat":        h.Cfg.CityLat,
		"city_lng":        h.Cfg.CityLng,
		"currency":        h.Cfg.Currency,
		"support_phone":   h.Cfg.SupportPhone,
		"otp_ttl_min":     h.Cfg.OTPTTLMin,
		"min_app_version": 1,
		"payment_methods": h.paymentMethods(),
		"card_topup":      h.Gateway != nil,
	})
}

// Tariffs GET /tariffs
func (h *Handler) Tariffs(c *fiber.Ctx) error {
	var ts []models.Tariff
	h.DB.Where("is_active = ?", true).Order("sort_order").Find(&ts)
	return utils.OK(c, ts)
}

// AllTariffs GET /admin/tariffs (incl. inactive)
func (h *Handler) AllTariffs(c *fiber.Ctx) error {
	var ts []models.Tariff
	h.DB.Order("sort_order").Find(&ts)
	return utils.OK(c, ts)
}

// Addresses GET /addresses
func (h *Handler) Addresses(c *fiber.Ctx) error {
	var as []models.SavedAddress
	h.DB.Where("user_id = ?", middleware.UID(c)).Order("id").Find(&as)
	return utils.OK(c, as)
}

// AddAddress POST /addresses {title, address, lat, lng}
func (h *Handler) AddAddress(c *fiber.Ctx) error {
	var a models.SavedAddress
	if err := c.BodyParser(&a); err != nil || strings.TrimSpace(a.Title) == "" || (a.Lat == 0 && a.Lng == 0) {
		return utils.Err(c, 400, "ady we koordinatasy gerek")
	}
	a.ID, a.UserID = 0, middleware.UID(c)
	var n int64
	h.DB.Model(&models.SavedAddress{}).Where("user_id = ?", a.UserID).Count(&n)
	if n >= 20 {
		return utils.Err(c, 400, "iň köp 20 salgy saklap bolýar")
	}
	h.DB.Create(&a)
	return c.Status(201).JSON(fiber.Map{"success": true, "data": a})
}

// DeleteAddress DELETE /addresses/:id
func (h *Handler) DeleteAddress(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	h.DB.Where("id = ? AND user_id = ?", id, middleware.UID(c)).Delete(&models.SavedAddress{})
	return utils.OK(c, true)
}

// CheckPromo POST /promo/check {code, price}
func (h *Handler) CheckPromo(c *fiber.Ctx) error {
	var req struct {
		Code  string  `json:"code"`
		Price float64 `json:"price"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	var p models.PromoCode
	if err := h.DB.Where("code = ?", strings.ToUpper(strings.TrimSpace(req.Code))).First(&p).Error; err != nil || !services.PromoValid(&p) {
		return utils.Err(c, 400, "promokod nädogry ýa-da möhleti geçen")
	}
	return utils.OK(c, fiber.Map{"code": p.Code, "discount": services.ApplyPromo(&p, req.Price), "discount_percent": p.DiscountPercent, "discount_fixed": p.DiscountFixed})
}

// GeoReverse GET /geo/reverse?lat=&lng=
func (h *Handler) GeoReverse(c *fiber.Ctx) error {
	lat, lng := c.QueryFloat("lat"), c.QueryFloat("lng")
	if lat == 0 && lng == 0 {
		return utils.Err(c, 400, "lat we lng gerek")
	}
	p, _ := h.Geo.Reverse(lat, lng)
	return utils.OK(c, p)
}

// GeoSearch GET /geo/search?q=
func (h *Handler) GeoSearch(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		return utils.OK(c, []services.Place{})
	}
	ps, err := h.Geo.Search(q)
	if err != nil {
		return utils.OK(c, []services.Place{})
	}
	return utils.OK(c, ps)
}

// NearbyDrivers GET /drivers/nearby?lat=&lng= — small list for the client map (positions only).
func (h *Handler) NearbyDrivers(c *fiber.Ctx) error {
	lat, lng := c.QueryFloat("lat"), c.QueryFloat("lng")
	if lat == 0 && lng == 0 {
		lat, lng = h.Cfg.CityLat, h.Cfg.CityLng
	}
	cands := h.Dispatch.NearbyDrivers(lat, lng, h.Cfg.DispatchMaxRadiusKm, "")
	type pos struct {
		Lat     float64 `json:"lat"`
		Lng     float64 `json:"lng"`
		Heading float64 `json:"heading"`
		Tariffs string  `json:"tariffs"`
	}
	out := make([]pos, 0, len(cands))
	for i, cd := range cands {
		if i >= 15 {
			break
		}
		out = append(out, pos{cd.Driver.Lat, cd.Driver.Lng, cd.Driver.Heading, cd.Driver.TariffCodes})
	}
	return utils.OK(c, out)
}

// paymentMethods are the ride payment options offered to clients.
// Card is hidden until rides are actually charged online (RIDE_CARD_PAYMENT=true).
func (h *Handler) paymentMethods() []string {
	if h.Cfg.RideCardPayment {
		return []string{"cash", "card"}
	}
	return []string{"cash"}
}
