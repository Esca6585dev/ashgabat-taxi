package services

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/ashgabat-taxi/backend/internal/config"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
	"github.com/ashgabat-taxi/backend/internal/ws"
)

// Dispatcher matches searching orders with nearby online drivers.
//
// Algorithm (Yandex-Go like "broadcast to nearest"):
//  1. every tick (and immediately on creation) each searching order is offered to
//     the N nearest free, approved, online drivers inside a radius;
//  2. the radius grows with the age of the order up to DispatchMaxRadiusKm;
//  3. drivers that declined are skipped;
//  4. after DispatchTimeoutSec the order is cancelled by the system.
type Dispatcher struct {
	db  *gorm.DB
	hub *ws.Hub
	cfg *config.Config

	mu     sync.Mutex
	offers map[uint]*offerState // orderID -> state

	// Billing, when set, keeps drivers without a paid plan / enough balance out of dispatch.
	Billing *Billing
	ticks   int
}

type offerState struct {
	notified map[uint]time.Time // driverID -> when
	declined map[uint]bool
}

// DriverCandidate is a driver with the distance to the pickup.
type DriverCandidate struct {
	Driver     models.Driver `json:"driver"`
	DistanceKm float64       `json:"distance_km"`
	EtaMin     float64       `json:"eta_min"`
}

// NewDispatcher wires the dispatcher.
func NewDispatcher(db *gorm.DB, hub *ws.Hub, cfg *config.Config) *Dispatcher {
	return &Dispatcher{db: db, hub: hub, cfg: cfg, offers: make(map[uint]*offerState)}
}

func (d *Dispatcher) state(orderID uint) *offerState {
	s, ok := d.offers[orderID]
	if !ok {
		s = &offerState{notified: map[uint]time.Time{}, declined: map[uint]bool{}}
		d.offers[orderID] = s
	}
	return s
}

// Decline records that a driver refused the order.
func (d *Dispatcher) Decline(orderID, driverID uint) {
	d.mu.Lock()
	d.state(orderID).declined[driverID] = true
	d.mu.Unlock()
}

// Clear forgets an order (accepted or cancelled).
func (d *Dispatcher) Clear(orderID uint) {
	d.mu.Lock()
	delete(d.offers, orderID)
	d.mu.Unlock()
}

// NotifiedDrivers returns driver user ids that received the offer (to tell them it was taken).
func (d *Dispatcher) NotifiedDrivers(orderID uint) []uint {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.offers[orderID]
	if !ok {
		return nil
	}
	ids := make([]uint, 0, len(s.notified))
	for id := range s.notified {
		ids = append(ids, id)
	}
	return ids
}

// radiusFor grows the search radius with order age.
func (d *Dispatcher) radiusFor(age time.Duration) float64 {
	r := d.cfg.DispatchRadiusKm + float64(int(age.Seconds())/30)*1.5
	if r > d.cfg.DispatchMaxRadiusKm {
		r = d.cfg.DispatchMaxRadiusKm
	}
	return r
}

// NearbyDrivers lists free online approved drivers around a point.
func (d *Dispatcher) NearbyDrivers(lat, lng, radiusKm float64, tariffCode string) []DriverCandidate {
	cut := time.Now().Add(-time.Duration(d.cfg.DriverOfflineSec) * time.Second)
	dLat := radiusKm / 111.0
	dLng := radiusKm / 85.0 // ~ cos(38°)*111
	var drivers []models.Driver
	q := d.db.Preload("User").
		Where("is_online = ? AND status = ? AND current_order_id IS NULL AND last_seen_at > ?", true, models.DriverApproved, cut).
		Where("lat BETWEEN ? AND ? AND lng BETWEEN ? AND ?", lat-dLat, lat+dLat, lng-dLng, lng+dLng)
	if d.Billing != nil {
		q = d.Billing.WorkFilter(q)
	}
	q.Find(&drivers)
	out := make([]DriverCandidate, 0, len(drivers))
	for _, dr := range drivers {
		if tariffCode != "" && !driverServes(dr, tariffCode) {
			continue
		}
		if dr.User != nil && dr.User.IsBlocked {
			continue
		}
		dist := utils.Haversine(lat, lng, dr.Lat, dr.Lng)
		if dist <= radiusKm {
			out = append(out, DriverCandidate{Driver: dr, DistanceKm: dist, EtaMin: dist * detourFactor / avgCitySpeedKm * 60})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DistanceKm < out[j].DistanceKm })
	return out
}

func driverServes(dr models.Driver, code string) bool {
	for _, c := range strings.Split(dr.TariffCodes, ",") {
		if strings.TrimSpace(c) == code {
			return true
		}
	}
	return false
}

// OfferOrder pushes the order to new candidates. Returns number of drivers notified this round.
func (d *Dispatcher) OfferOrder(o *models.Order) int {
	if o.Tariff == nil {
		var t models.Tariff
		if err := d.db.First(&t, o.TariffID).Error; err == nil {
			o.Tariff = &t
		}
	}
	code := ""
	if o.Tariff != nil {
		code = o.Tariff.Code
	}
	radius := d.radiusFor(time.Since(o.CreatedAt))
	cands := d.NearbyDrivers(o.PickupLat, o.PickupLng, radius, code)

	d.mu.Lock()
	st := d.state(o.ID)
	sent := 0
	for _, c := range cands {
		if sent >= d.cfg.DispatchBatch {
			break
		}
		if st.declined[c.Driver.UserID] {
			continue
		}
		if _, done := st.notified[c.Driver.UserID]; done {
			continue
		}
		st.notified[c.Driver.UserID] = time.Now()
		sent++
		payload := map[string]any{"order": o, "distance_km": utils.RoundMoney(c.DistanceKm*2) / 2, "eta_min": int(c.EtaMin + 0.5)}
		d.hub.SendTo(c.Driver.UserID, "order.new", payload)
	}
	d.mu.Unlock()
	return sent
}

// Run loops forever re-offering searching orders and expiring stale ones.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.tick()
		}
	}
}

func (d *Dispatcher) tick() {
	var orders []models.Order
	d.db.Preload("Tariff").Where("status = ?", models.OrderSearching).Find(&orders)
	timeout := time.Duration(d.cfg.DispatchTimeoutSec) * time.Second
	for i := range orders {
		o := &orders[i]
		if o.ScheduledAt != nil && o.ScheduledAt.After(time.Now().Add(15*time.Minute)) {
			continue // scheduled ride: start dispatching 15 min before
		}
		if time.Since(o.CreatedAt) > timeout {
			d.expire(o)
			continue
		}
		d.OfferOrder(o)
	}
	// forget states of orders no longer searching
	d.mu.Lock()
	alive := map[uint]bool{}
	for _, o := range orders {
		alive[o.ID] = true
	}
	for id := range d.offers {
		if !alive[id] {
			delete(d.offers, id)
		}
	}
	d.mu.Unlock()

	// once a minute: renew monthly plans, take offline drivers who cannot pay
	d.ticks++
	if d.Billing != nil && d.ticks%12 == 1 {
		d.Billing.Sweep()
	}

	// mark drivers offline when they stopped sending locations
	cut := time.Now().Add(-time.Duration(d.cfg.DriverOfflineSec*3) * time.Second)
	d.db.Model(&models.Driver{}).Where("is_online = ? AND last_seen_at < ?", true, cut).Update("is_online", false)
}

func (d *Dispatcher) expire(o *models.Order) {
	now := time.Now()
	res := d.db.Model(&models.Order{}).Where("id = ? AND status = ?", o.ID, models.OrderSearching).Updates(map[string]any{
		"status": models.OrderCancelled, "cancelled_at": now, "cancelled_by": "system", "cancel_reason": "no_driver",
	})
	if res.RowsAffected == 0 {
		return
	}
	o.Status = models.OrderCancelled
	o.CancelledAt = &now
	o.CancelledBy = "system"
	o.CancelReason = "no_driver"
	d.hub.SendTo(o.ClientID, "order.cancelled", o)
	d.hub.SendToMany(d.NotifiedDrivers(o.ID), "order.taken", map[string]any{"order_id": o.ID})
	d.Clear(o.ID)
	log.Printf("dispatch: order %d expired (no driver)", o.ID)
}
