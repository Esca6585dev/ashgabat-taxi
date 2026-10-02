package services

import (
	"time"

	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// Pricing turns a route and a tariff into a manat amount.
type Pricing struct{}

// IsNight reports whether the night multiplier applies (22:00 – 06:00 local).
func IsNight(t time.Time) bool {
	h := t.Hour()
	return h >= 22 || h < 6
}

// Price computes the fixed fare for a tariff.
func (Pricing) Price(t *models.Tariff, distKm, durMin float64, at time.Time) float64 {
	p := t.BaseFare + t.PerKm*distKm + t.PerMin*durMin
	if p < t.MinFare {
		p = t.MinFare
	}
	if t.Multiplier > 0 {
		p *= t.Multiplier
	}
	if IsNight(at) && t.NightMultiplier > 0 {
		p *= t.NightMultiplier
	}
	return utils.RoundMoney(p)
}

// ApplyPromo returns the discount amount for a price.
func ApplyPromo(p *models.PromoCode, price float64) float64 {
	if p == nil {
		return 0
	}
	d := price * p.DiscountPercent / 100
	d += p.DiscountFixed
	if d > price {
		d = price
	}
	return utils.RoundMoney(d)
}

// PromoValid checks that a promo code may still be used.
func PromoValid(p *models.PromoCode) bool {
	if p == nil || !p.IsActive {
		return false
	}
	if p.MaxUses > 0 && p.Uses >= p.MaxUses {
		return false
	}
	if p.ExpiresAt != nil && p.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}
