package handlers

import (
	"encoding/json"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// WSUpgrade authenticates ?token= and allows the websocket upgrade.
func (h *Handler) WSUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	uid, role, ok := middleware.ParseToken(h.Cfg, c.Query("token"))
	if !ok {
		return utils.Err(c, 401, "token nädogry")
	}
	c.Locals("uid", uid)
	c.Locals("role", role)
	return c.Next()
}

type wsIn struct {
	Type    string  `json:"type"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Heading float64 `json:"heading"`
}

// WSHandler is the realtime endpoint GET /ws?token=JWT
//
// server -> app events: connected, order.new, order.taken, order.accepted, order.status,
// order.completed, order.cancelled, order.requeued, driver.location, driver.status,
// balance.changed, ticket.reply, account.blocked, pong
// app -> server messages: {"type":"location","lat":..,"lng":..,"heading":..} (driver), {"type":"ping"}
func (h *Handler) WSHandler() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		uid, _ := conn.Locals("uid").(uint)
		role, _ := conn.Locals("role").(string)
		cl := h.Hub.Register(uid, role, conn)
		defer h.Hub.Unregister(cl)

		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		})
		h.Hub.SendTo(uid, "connected", fiber.Map{"user_id": uid, "role": role})
		if role == string(models.RoleDriver) {
			h.DB.Model(&models.Driver{}).Where("user_id = ?", uid).Update("last_seen_at", time.Now())
		}

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			var in wsIn
			if json.Unmarshal(msg, &in) != nil {
				continue
			}
			switch in.Type {
			case "ping":
				h.Hub.SendTo(uid, "pong", nil)
			case "location":
				if role == string(models.RoleDriver) && (in.Lat != 0 || in.Lng != 0) {
					h.updateDriverLocation(uid, in.Lat, in.Lng, in.Heading)
				}
			}
		}
	})
}
