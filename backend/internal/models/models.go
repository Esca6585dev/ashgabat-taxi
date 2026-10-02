// Package models defines the persistent entities of the taxi platform.
package models

import "time"

// Role of a user account.
type Role string

const (
	RoleClient Role = "client"
	RoleDriver Role = "driver"
	RoleAdmin  Role = "admin"
)

// User is any account: client, driver or admin.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Phone        string    `gorm:"uniqueIndex;size:20" json:"phone"`
	Name         string    `gorm:"size:100" json:"name"`
	Role         Role      `gorm:"size:16;index" json:"role"`
	PasswordHash string    `json:"-"`
	Lang         string    `gorm:"size:5;default:tk" json:"lang"`
	IsBlocked    bool      `json:"is_blocked"`
	Rating       float64   `gorm:"default:5" json:"rating"`
	RatingCount  int       `json:"rating_count"`
	FCMToken     string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Driver       *Driver   `json:"driver,omitempty"`
}

// OTPCode is a one-time SMS login code.
type OTPCode struct {
	ID        uint      `gorm:"primaryKey"`
	Phone     string    `gorm:"index;size:20"`
	Code      string    `gorm:"size:8"`
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

// DriverStatus is the moderation state of a driver.
type DriverStatus string

const (
	DriverPending  DriverStatus = "pending"
	DriverApproved DriverStatus = "approved"
	DriverRejected DriverStatus = "rejected"
	DriverBlocked  DriverStatus = "blocked"
)

// Driver holds the car and dispatch state of a driver account.
type Driver struct {
	ID             uint         `gorm:"primaryKey" json:"id"`
	UserID         uint         `gorm:"uniqueIndex" json:"user_id"`
	User           *User        `json:"user,omitempty"`
	Status         DriverStatus `gorm:"size:16;index;default:pending" json:"status"`
	IsOnline       bool         `gorm:"index" json:"is_online"`
	Lat            float64      `json:"lat"`
	Lng            float64      `json:"lng"`
	Heading        float64      `json:"heading"`
	LastSeenAt     *time.Time   `json:"last_seen_at"`
	CarBrand       string       `gorm:"size:50" json:"car_brand"`
	CarModel       string       `gorm:"size:50" json:"car_model"`
	CarColor       string       `gorm:"size:30" json:"car_color"`
	CarPlate       string       `gorm:"size:20" json:"car_plate"`
	CarYear        int          `json:"car_year"`
	LicenseNumber  string       `gorm:"size:50" json:"license_number"`
	TariffCodes    string       `gorm:"size:100;default:economy" json:"tariff_codes"`
	Balance        float64      `json:"balance"`
	CommissionRate float64      `json:"commission_rate"`
	TotalRides     int          `json:"total_rides"`
	CurrentOrderID *uint        `json:"current_order_id"`
	RejectReason   string       `json:"reject_reason"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// Tariff is a service class (economy, comfort, business, delivery).
type Tariff struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Code            string    `gorm:"uniqueIndex;size:30" json:"code"`
	Name            string    `gorm:"size:50" json:"name"`
	NameRu          string    `gorm:"size:50" json:"name_ru"`
	Description     string    `gorm:"size:200" json:"description"`
	BaseFare        float64   `json:"base_fare"`
	PerKm           float64   `json:"per_km"`
	PerMin          float64   `json:"per_min"`
	MinFare         float64   `json:"min_fare"`
	FreeWaitMin     int       `json:"free_wait_min"`
	PerWaitMin      float64   `json:"per_wait_min"`
	Multiplier      float64   `gorm:"default:1" json:"multiplier"`
	NightMultiplier float64   `gorm:"default:1" json:"night_multiplier"`
	Icon            string    `gorm:"size:30" json:"icon"`
	Seats           int       `gorm:"default:4" json:"seats"`
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// OrderStatus lifecycle: searching -> accepted -> arrived -> in_progress -> completed | cancelled.
type OrderStatus string

const (
	OrderSearching  OrderStatus = "searching"
	OrderAccepted   OrderStatus = "accepted"
	OrderArrived    OrderStatus = "arrived"
	OrderInProgress OrderStatus = "in_progress"
	OrderCompleted  OrderStatus = "completed"
	OrderCancelled  OrderStatus = "cancelled"
)

// ActiveStatuses are statuses where the order still occupies a client/driver.
var ActiveStatuses = []OrderStatus{OrderSearching, OrderAccepted, OrderArrived, OrderInProgress}

// Order is a ride or delivery request.
type Order struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	ClientID       uint        `gorm:"index" json:"client_id"`
	Client         *User       `json:"client,omitempty"`
	DriverID       *uint       `gorm:"index" json:"driver_id"`
	Driver         *Driver     `json:"driver,omitempty"`
	TariffID       uint        `json:"tariff_id"`
	Tariff         *Tariff     `json:"tariff,omitempty"`
	Status         OrderStatus `gorm:"size:20;index" json:"status"`
	OrderType      string      `gorm:"size:20;default:ride" json:"order_type"` // ride | delivery
	PickupLat      float64     `json:"pickup_lat"`
	PickupLng      float64     `json:"pickup_lng"`
	PickupAddress  string      `gorm:"size:255" json:"pickup_address"`
	DropoffLat     float64     `json:"dropoff_lat"`
	DropoffLng     float64     `json:"dropoff_lng"`
	DropoffAddress string      `gorm:"size:255" json:"dropoff_address"`
	DistanceKm     float64     `json:"distance_km"`
	DurationMin    float64     `json:"duration_min"`
	EstimatedPrice float64     `json:"estimated_price"`
	Discount       float64     `json:"discount"`
	FinalPrice     float64     `json:"final_price"`
	Commission     float64     `json:"commission"`
	PaymentMethod  string      `gorm:"size:20;default:cash" json:"payment_method"` // cash | card
	IsPaid         bool        `json:"is_paid"`
	PromoCodeID    *uint       `json:"promo_code_id"`
	Comment        string      `gorm:"size:500" json:"comment"`
	RecipientPhone string      `gorm:"size:20" json:"recipient_phone"`
	ScheduledAt    *time.Time  `json:"scheduled_at"`
	AcceptedAt     *time.Time  `json:"accepted_at"`
	ArrivedAt      *time.Time  `json:"arrived_at"`
	StartedAt      *time.Time  `json:"started_at"`
	CompletedAt    *time.Time  `json:"completed_at"`
	CancelledAt    *time.Time  `json:"cancelled_at"`
	CancelledBy    string      `gorm:"size:20" json:"cancelled_by"` // client | driver | admin | system
	CancelReason   string      `gorm:"size:255" json:"cancel_reason"`
	ClientRating   int         `json:"client_rating"` // rating given BY client TO driver
	DriverRating   int         `json:"driver_rating"` // rating given BY driver TO client
	ClientComment  string      `gorm:"size:500" json:"client_comment"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// IsActive reports whether the order still blocks client/driver from new orders.
func (o *Order) IsActive() bool {
	for _, s := range ActiveStatuses {
		if o.Status == s {
			return true
		}
	}
	return false
}

// PromoCode gives a discount to clients.
type PromoCode struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Code            string     `gorm:"uniqueIndex;size:30" json:"code"`
	DiscountPercent float64    `json:"discount_percent"`
	DiscountFixed   float64    `json:"discount_fixed"`
	MaxUses         int        `json:"max_uses"`
	Uses            int        `json:"uses"`
	ExpiresAt       *time.Time `json:"expires_at"`
	IsActive        bool       `gorm:"default:true" json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
}

// SavedAddress is a favourite place of a client (home, work...).
type SavedAddress struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Title     string    `gorm:"size:50" json:"title"`
	Address   string    `gorm:"size:255" json:"address"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	CreatedAt time.Time `json:"created_at"`
}

// Transaction is a driver balance movement.
type Transaction struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	DriverID  uint      `gorm:"index" json:"driver_id"`
	OrderID   *uint     `json:"order_id"`
	Amount    float64   `json:"amount"` // positive = credit, negative = debit
	Type      string    `gorm:"size:20" json:"type"` // commission | topup | payout | ride_income | adjustment
	Note      string    `gorm:"size:255" json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// SupportTicket is a complaint / question from a user.
type SupportTicket struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	User      *User     `json:"user,omitempty"`
	OrderID   *uint     `json:"order_id"`
	Message   string    `gorm:"size:1000" json:"message"`
	Status    string    `gorm:"size:20;default:open" json:"status"` // open | resolved
	Reply     string    `gorm:"size:1000" json:"reply"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// All returns every model for auto-migration.
func All() []any {
	return []any{&User{}, &OTPCode{}, &Driver{}, &Tariff{}, &Order{}, &PromoCode{}, &SavedAddress{}, &Transaction{}, &SupportTicket{}}
}
