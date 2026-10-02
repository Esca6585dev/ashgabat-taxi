// Package database opens the DB connection, migrates and seeds defaults.
package database

import (
	"log"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// Connect opens sqlite or postgres depending on config.
func Connect(cfg *config.Config) *gorm.DB {
	gcfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	var (
		db  *gorm.DB
		err error
	)
	switch cfg.DBDriver {
	case "postgres":
		db, err = gorm.Open(postgres.Open(cfg.DBDSN), gcfg)
	default:
		db, err = gorm.Open(sqlite.Open(cfg.DBDSN), gcfg)
		if err == nil {
			db.Exec("PRAGMA journal_mode=WAL")
			db.Exec("PRAGMA busy_timeout=5000")
		}
	}
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	log.Printf("database: connected (%s)", cfg.DBDriver)
	return db
}

// Migrate creates / updates all tables.
func Migrate(db *gorm.DB) {
	if err := db.AutoMigrate(models.All()...); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}

// Seed inserts default tariffs and the admin account when missing.
func Seed(db *gorm.DB, cfg *config.Config) {
	var n int64
	db.Model(&models.Tariff{}).Count(&n)
	if n == 0 {
		db.Create(&[]models.Tariff{
			{Code: "economy", Name: "Ekonom", NameRu: "Эконом", Description: "Amatly bahaly gündelik gatnaw", BaseFare: 5, PerKm: 1.5, PerMin: 0.2, MinFare: 7, FreeWaitMin: 3, PerWaitMin: 0.5, Multiplier: 1, NightMultiplier: 1.2, Icon: "economy", Seats: 4, IsActive: true, SortOrder: 1},
			{Code: "comfort", Name: "Komfort", NameRu: "Комфорт", Description: "Täze we rahat awtoulaglar", BaseFare: 7, PerKm: 2, PerMin: 0.3, MinFare: 10, FreeWaitMin: 3, PerWaitMin: 0.7, Multiplier: 1, NightMultiplier: 1.2, Icon: "comfort", Seats: 4, IsActive: true, SortOrder: 2},
			{Code: "business", Name: "Biznes", NameRu: "Бизнес", Description: "Premium klas awtoulaglar", BaseFare: 12, PerKm: 3, PerMin: 0.5, MinFare: 20, FreeWaitMin: 5, PerWaitMin: 1, Multiplier: 1, NightMultiplier: 1.2, Icon: "business", Seats: 4, IsActive: true, SortOrder: 3},
			{Code: "minivan", Name: "Miniwen", NameRu: "Минивэн", Description: "6-7 adam üçin giň awtoulag", BaseFare: 10, PerKm: 2.5, PerMin: 0.4, MinFare: 15, FreeWaitMin: 3, PerWaitMin: 0.8, Multiplier: 1, NightMultiplier: 1.2, Icon: "minivan", Seats: 7, IsActive: true, SortOrder: 4},
			{Code: "delivery", Name: "Eltip bermek", NameRu: "Доставка", Description: "Posylka we nahar eltip bermek", BaseFare: 5, PerKm: 1.2, PerMin: 0.15, MinFare: 6, FreeWaitMin: 5, PerWaitMin: 0.5, Multiplier: 1, NightMultiplier: 1.1, Icon: "delivery", Seats: 0, IsActive: true, SortOrder: 5},
		})
		log.Println("seed: default tariffs created")
	}

	var admin models.User
	if err := db.Where("role = ?", models.RoleAdmin).First(&admin).Error; err != nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
		db.Create(&models.User{Phone: utils.NormalizePhone(cfg.AdminPhone), Name: "Administrator", Role: models.RoleAdmin, PasswordHash: string(hash), Rating: 5})
		log.Printf("seed: admin %s created", cfg.AdminPhone)
	}
}
