// Package handlers implements the HTTP / WebSocket API.
package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/ws"
)

// Handler bundles every dependency the endpoints need.
type Handler struct {
	DB       *gorm.DB
	Cfg      *config.Config
	Hub      *ws.Hub
	Dispatch *services.Dispatcher
	OTP      *services.OTPService
	Router   *services.Router
	Geo      *services.Geocoder
	Pricing  services.Pricing

	// optional: payment plans and card top-up (nil = not enforced / not offered)
	Billing *services.Billing
	Gateway services.PaymentGateway
}

// New builds the handler set.
func New(db *gorm.DB, cfg *config.Config, hub *ws.Hub, disp *services.Dispatcher, otp *services.OTPService, router *services.Router, geo *services.Geocoder) *Handler {
	return &Handler{DB: db, Cfg: cfg, Hub: hub, Dispatch: disp, OTP: otp, Router: router, Geo: geo}
}

var errNotFound = errors.New("tapylmady")

func idParam(c *fiber.Ctx, name string) (uint, error) {
	v, err := strconv.ParseUint(c.Params(name), 10, 64)
	if err != nil || v == 0 {
		return 0, errors.New("nädogry id")
	}
	return uint(v), nil
}

func pageParams(c *fiber.Ctx) (limit, offset int) {
	limit = c.QueryInt("limit", 30)
	if limit < 1 || limit > 200 {
		limit = 30
	}
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	return limit, (page - 1) * limit
}

func (h *Handler) currentUser(c *fiber.Ctx) (*models.User, error) {
	var u models.User
	if err := h.DB.First(&u, middleware.UID(c)).Error; err != nil {
		return nil, errNotFound
	}
	if u.IsBlocked {
		return nil, errors.New("hasabyňyz bloklanan")
	}
	return &u, nil
}

func (h *Handler) driverOf(userID uint) (*models.Driver, error) {
	var d models.Driver
	if err := h.DB.Preload("User").Where("user_id = ?", userID).First(&d).Error; err != nil {
		return nil, errNotFound
	}
	return &d, nil
}

// loadOrder fetches an order with client, driver (+user) and tariff.
func (h *Handler) loadOrder(id uint) (*models.Order, error) {
	var o models.Order
	err := h.DB.Preload("Client").Preload("Driver").Preload("Driver.User").Preload("Tariff").First(&o, id).Error
	if err != nil {
		return nil, errNotFound
	}
	return &o, nil
}

// pushOrder sends an order event to its client and its driver.
func (h *Handler) pushOrder(o *models.Order, typ string) {
	h.Hub.SendTo(o.ClientID, typ, o)
	if o.Driver != nil {
		h.Hub.SendTo(o.Driver.UserID, typ, o)
	}
	h.Hub.BroadcastRole(string(models.RoleAdmin), typ, o)
}

// applyRating updates the running average rating of a user.
func (h *Handler) applyRating(userID uint, stars int) {
	var u models.User
	if err := h.DB.First(&u, userID).Error; err != nil {
		return
	}
	total := u.Rating*float64(u.RatingCount) + float64(stars)
	u.RatingCount++
	u.Rating = total / float64(u.RatingCount)
	h.DB.Model(&u).Updates(map[string]any{"rating": u.Rating, "rating_count": u.RatingCount})
}

func (h *Handler) activeOrderOfClient(clientID uint) *models.Order {
	var o models.Order
	if err := h.DB.Where("client_id = ? AND status IN ?", clientID, models.ActiveStatuses).Order("id desc").First(&o).Error; err != nil {
		return nil
	}
	full, _ := h.loadOrder(o.ID)
	return full
}
