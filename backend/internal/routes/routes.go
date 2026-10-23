// Package routes wires URL paths to handlers.
package routes

import (
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"

	"github.com/ashgabat-taxi/backend/internal/handlers"
	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
)

// Register mounts the whole API under /api/v1 plus static admin panel and website.
func Register(app *fiber.App, h *handlers.Handler) {
	auth := middleware.Protect(h.Cfg)
	anyUser := middleware.RequireRole(models.RoleClient, models.RoleDriver, models.RoleAdmin)
	driverOnly := middleware.RequireRole(models.RoleDriver)
	adminOnly := middleware.RequireRole(models.RoleAdmin)

	api := app.Group("/api/v1")
	api.Get("/health", h.Health)
	api.Get("/config", h.PublicConfig)
	api.Get("/tariffs", h.Tariffs)
	api.Get("/geo/reverse", h.GeoReverse)
	api.Get("/geo/search", h.GeoSearch)
	api.Get("/drivers/nearby", h.NearbyDrivers)

	// auth
	a := api.Group("/auth")
	a.Post("/send-otp", h.SendOTP)
	a.Post("/verify-otp", h.VerifyOTP)
	a.Post("/admin/login", h.AdminLogin)
	a.Post("/login", h.PasswordLogin) // drivers: phone + password
	a.Post("/password", auth, h.SetPassword)
	a.Get("/me", auth, h.Me)
	a.Put("/profile", auth, h.UpdateProfile)

	// client (any logged-in user may order)
	o := api.Group("/orders", auth, anyUser)
	o.Post("/estimate", h.Estimate)
	o.Post("/", h.CreateOrder)
	o.Get("/", h.MyOrders)
	o.Get("/active", h.ActiveOrder)
	o.Get("/:id", h.GetOrder)
	o.Post("/:id/cancel", h.CancelOrder)
	o.Post("/:id/rate", h.RateDriver)

	api.Get("/addresses", auth, h.Addresses)
	api.Post("/addresses", auth, h.AddAddress)
	api.Delete("/addresses/:id", auth, h.DeleteAddress)
	api.Post("/promo/check", auth, h.CheckPromo)
	api.Post("/support", auth, h.CreateTicket)
	api.Get("/support", auth, h.MyTickets)

	// driver
	d := api.Group("/driver", auth)
	d.Post("/register", h.DriverRegister) // client may become a driver here
	d.Get("/me", driverOnly, h.DriverMe)
	d.Post("/online", driverOnly, h.DriverOnline)
	d.Post("/location", driverOnly, h.DriverLocation)
	d.Get("/orders", driverOnly, h.DriverOrders)
	d.Get("/orders/available", driverOnly, h.AvailableOrders)
	d.Post("/orders/:id/accept", driverOnly, h.AcceptOrder)
	d.Post("/orders/:id/decline", driverOnly, h.DeclineOrder)
	d.Post("/orders/:id/arrived", driverOnly, h.ArrivedOrder)
	d.Post("/orders/:id/start", driverOnly, h.StartOrder)
	d.Post("/orders/:id/complete", driverOnly, h.CompleteOrder)
	d.Post("/orders/:id/cancel", driverOnly, h.DriverCancelOrder)
	d.Post("/orders/:id/rate", driverOnly, h.RateClient)
	d.Get("/earnings", driverOnly, h.DriverEarnings)

	// admin
	ad := api.Group("/admin", auth, adminOnly)
	ad.Get("/stats", h.AdminStats)
	ad.Post("/password", h.ChangeAdminPassword)
	ad.Get("/users", h.AdminUsers)
	ad.Put("/users/:id/block", h.AdminBlockUser)
	ad.Put("/users/:id/password", h.AdminSetPassword)
	ad.Post("/drivers/account", h.AdminCreateDriver)
	ad.Get("/drivers", h.AdminDrivers)
	ad.Get("/drivers/online", h.AdminOnlineDrivers)
	ad.Put("/drivers/:id/status", h.AdminDriverStatus)
	ad.Post("/drivers/:id/balance", h.AdminDriverBalance)
	ad.Get("/orders", h.AdminOrders)
	ad.Get("/orders/:id", h.GetOrder)
	ad.Post("/orders/:id/assign", h.AdminAssignOrder)
	ad.Post("/orders/:id/cancel", h.AdminCancelOrder)
	ad.Get("/tariffs", h.AllTariffs)
	ad.Post("/tariffs", h.AdminTariffSave)
	ad.Put("/tariffs/:id", h.AdminTariffSave)
	ad.Delete("/tariffs/:id", h.AdminTariffDelete)
	ad.Get("/promos", h.AdminPromos)
	ad.Post("/promos", h.AdminPromoSave)
	ad.Put("/promos/:id", h.AdminPromoSave)
	ad.Delete("/promos/:id", h.AdminPromoDelete)
	ad.Get("/tickets", h.AdminTickets)
	ad.Put("/tickets/:id", h.AdminTicketReply)

	// realtime
	app.Get("/ws", h.WSUpgrade, h.WSHandler())

	// static: admin panel and marketing website
	if st, err := os.Stat(h.Cfg.AdminDir); err == nil && st.IsDir() {
		app.Static("/admin", h.Cfg.AdminDir)
		app.Get("/admin/*", func(c *fiber.Ctx) error { return c.SendFile(filepath.Join(h.Cfg.AdminDir, "index.html")) })
	}
	// web versions of the apps (Flutter, built with --base-href /app/ and /driver/)
	for prefix, dir := range map[string]string{"/app": h.Cfg.ClientWebDir, "/driver": h.Cfg.DriverWebDir} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			index := filepath.Join(dir, "index.html")
			// assets resolve through <base href="/app/">, so /app and /app/ both work
			app.Static(prefix, dir, fiber.Static{Index: "index.html"})
			app.Get(prefix+"/*", func(c *fiber.Ctx) error { return c.SendFile(index) })
		}
	}
	if st, err := os.Stat(h.Cfg.SiteDir); err == nil && st.IsDir() {
		app.Static("/", h.Cfg.SiteDir)
	}
}
