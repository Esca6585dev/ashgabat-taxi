// Ashgabat Taxi API server.
//
//	go run ./cmd/server
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/database"
	"github.com/ashgabat-taxi/backend/internal/handlers"
	"github.com/ashgabat-taxi/backend/internal/routes"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/ws"
)

func main() {
	cfg := config.Load()
	db := database.Connect(cfg)
	database.Migrate(db)
	database.Seed(db, cfg)

	hub := ws.NewHub()
	if pusher := services.NewPusher(db, cfg); pusher != nil {
		hub.OnSend = pusher.Event
	}
	dispatcher := services.NewDispatcher(db, hub, cfg)
	otp := services.NewOTPService(db, cfg, services.NewSMSSender(cfg))
	router := services.NewRouter(cfg)
	geo := services.NewGeocoder(cfg)
	h := handlers.New(db, cfg, hub, dispatcher, otp, router, geo)
	h.Billing = services.NewBilling(db, cfg, hub)
	dispatcher.Billing = h.Billing
	h.Gateway = services.NewPaymentGateway(cfg)
	if cfg.PaymentProvider == "rbs" && (cfg.RBSUser == "" || cfg.RBSPassword == "") {
		log.Fatal("PAYMENT_PROVIDER=rbs: PAYMENT_RBS_USERNAME we PAYMENT_RBS_PASSWORD gerek")
	}
	if cfg.PaymentProvider == "demo" && !cfg.IsDev() {
		log.Println("WARNING: PAYMENT_PROVIDER=demo in production — balances can be topped up without real money")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go dispatcher.Run(ctx)
	go h.ReconcilePayments(ctx)

	app := fiber.New(fiber.Config{
		AppName:               "Ashgabat Taxi API",
		ReadTimeout:           30 * time.Second,
		DisableStartupMessage: !cfg.IsDev(),
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"success": false, "error": err.Error()})
		},
	})
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*", AllowHeaders: "Origin, Content-Type, Accept, Authorization"}))
	if cfg.IsDev() {
		app.Use(logger.New())
	}
	app.Use("/api/v1/auth/send-otp", limiter.New(limiter.Config{Max: 20, Expiration: time.Minute}))
	app.Use("/api/v1/auth/login", limiter.New(limiter.Config{Max: 20, Expiration: time.Minute}))

	routes.Register(app, h)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
		<-quit
		log.Println("shutting down...")
		cancel()
		_ = app.ShutdownWithTimeout(5 * time.Second)
	}()

	log.Printf("Ashgabat Taxi API listening on :%s (%s)", cfg.Port, cfg.Env)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
