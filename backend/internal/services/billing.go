package services

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/ws"
)

// SubscriptionDays is the length of one monthly payment.
const SubscriptionDays = 30

// Billing enforces the driver payment plans:
//   - commission: a share of every ride; the driver may work while balance >= DriverMinBalance.
//   - monthly: a fixed fee taken from the balance every 30 days; no commission per ride.
type Billing struct {
	db  *gorm.DB
	cfg *config.Config
	hub *ws.Hub
}

// NewBilling wires the billing rules.
func NewBilling(db *gorm.DB, cfg *config.Config, hub *ws.Hub) *Billing {
	return &Billing{db: db, cfg: cfg, hub: hub}
}

// ErrNeedTopUp means the driver must top up the balance before taking orders.
var ErrNeedTopUp = errors.New("balansy doldurmak gerek")

// Fee returns the monthly fee of a driver (own override or the default).
func (b *Billing) Fee(d *models.Driver) float64 {
	if d.MonthlyFee > 0 {
		return d.MonthlyFee
	}
	return b.cfg.MonthlyFee
}

// SubscriptionActive reports whether a monthly driver has paid up to now.
func SubscriptionActive(d *models.Driver, now time.Time) bool {
	return d.SubscriptionUntil != nil && d.SubscriptionUntil.After(now)
}

// CanWork checks (and for monthly plans renews from the balance) whether the driver may take orders.
// The returned error is a user-facing Turkmen message wrapping ErrNeedTopUp.
func (b *Billing) CanWork(d *models.Driver) error {
	now := time.Now()
	if d.PaymentPlan != models.PlanMonthly {
		if d.Balance < b.cfg.DriverMinBalance {
			return fmt.Errorf("%w: balansyňyz %.1f %s, iň azy %.1f %s bolmaly", ErrNeedTopUp, d.Balance, b.cfg.Currency, b.cfg.DriverMinBalance, b.cfg.Currency)
		}
		return nil
	}
	if SubscriptionActive(d, now) {
		return nil
	}
	if err := b.Renew(d); err != nil {
		return err
	}
	return nil
}

// Renew takes one monthly fee from the balance and extends the subscription by 30 days
// (from its current end if still active, otherwise from now).
func (b *Billing) Renew(d *models.Driver) error {
	fee := b.Fee(d)
	now := time.Now()
	from := now
	if SubscriptionActive(d, now) {
		from = *d.SubscriptionUntil
	}
	until := from.AddDate(0, 0, SubscriptionDays)
	err := b.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Driver{}).Where("id = ? AND balance >= ?", d.ID, fee).
			Updates(map[string]any{"balance": gorm.Expr("balance - ?", fee), "subscription_until": until})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("%w: aýlyk töleg %.1f %s, balansyňyz %.1f %s", ErrNeedTopUp, fee, b.cfg.Currency, d.Balance, b.cfg.Currency)
		}
		return tx.Create(&models.Transaction{DriverID: d.ID, Amount: -fee, Type: "subscription",
			Note: "Aýlyk töleg " + until.Format("02.01.2006") + " çenli"}).Error
	})
	if err != nil {
		return err
	}
	d.Balance -= fee
	d.SubscriptionUntil = &until
	b.hub.SendTo(d.UserID, "balance.changed", map[string]any{"balance": d.Balance, "amount": -fee, "subscription_until": until})
	return nil
}

// Sweep runs from the dispatcher loop: renews expired monthly plans of online drivers
// and takes offline the drivers who can no longer work, telling them why.
func (b *Billing) Sweep() {
	var ds []models.Driver
	now := time.Now()
	b.db.Where("is_online = ? AND current_order_id IS NULL AND status = ?", true, models.DriverApproved).
		Where("((payment_plan = ? AND (subscription_until IS NULL OR subscription_until <= ?)) OR (payment_plan <> ? AND balance < ?))",
			models.PlanMonthly, now, models.PlanMonthly, b.cfg.DriverMinBalance).
		Find(&ds)
	for i := range ds {
		d := &ds[i]
		err := b.CanWork(d)
		if err == nil {
			continue
		}
		b.db.Model(&models.Driver{}).Where("id = ?", d.ID).Update("is_online", false)
		b.hub.SendTo(d.UserID, "billing.required", map[string]any{"message": err.Error(), "balance": d.Balance, "plan": d.PaymentPlan})
		log.Printf("billing: driver %d offline: %v", d.ID, err)
	}
}

// WorkFilter limits dispatch candidates to drivers allowed to work right now.
func (b *Billing) WorkFilter(q *gorm.DB) *gorm.DB {
	return q.Where("((payment_plan = ? AND subscription_until > ?) OR (payment_plan <> ? AND balance >= ?))",
		models.PlanMonthly, time.Now(), models.PlanMonthly, b.cfg.DriverMinBalance)
}
