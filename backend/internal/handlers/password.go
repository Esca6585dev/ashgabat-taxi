package handlers

import (
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

const minPasswordLen = 6

// loginGuard locks a phone for a while after repeated wrong passwords (brute-force protection).
type loginGuard struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

const (
	maxLoginFails = 5
	loginLockFor  = 15 * time.Minute
)

var passwordGuard = &loginGuard{fails: map[string][]time.Time{}}

func (g *loginGuard) recent(phone string) []time.Time {
	cut := time.Now().Add(-loginLockFor)
	kept := g.fails[phone][:0]
	for _, t := range g.fails[phone] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	g.fails[phone] = kept
	return kept
}

func (g *loginGuard) locked(phone string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.recent(phone)) >= maxLoginFails
}

func (g *loginGuard) fail(phone string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fails[phone] = append(g.recent(phone), time.Now())
}

func (g *loginGuard) reset(phone string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, phone)
}

func hashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(h), err
}

// withPasswordFlag fills the computed has_password field.
func withPasswordFlag(u *models.User) *models.User {
	u.HasPassword = u.PasswordHash != ""
	return u
}

// PasswordLogin POST /auth/login {phone, password} — driver app alternative to the SMS code.
// Clients always use OTP; admins use /auth/admin/login.
func (h *Handler) PasswordLogin(c *fiber.Ctx) error {
	var req struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	phone := utils.NormalizePhone(req.Phone)
	if !utils.ValidPhone(phone) || req.Password == "" {
		return utils.Err(c, 400, "telefon belgisini we paroly giriziň")
	}
	if passwordGuard.locked(phone) {
		return utils.Err(c, 429, "gaty köp nädogry synanyşyk, 15 minutdan soň gaýtadan synanyşyň ýa-da SMS kod bilen giriň")
	}
	var u models.User
	err := h.DB.Where("phone = ? AND role = ?", phone, models.RoleDriver).First(&u).Error
	if err != nil || u.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		passwordGuard.fail(phone)
		return utils.Err(c, 401, "telefon ýa-da parol nädogry")
	}
	if u.IsBlocked {
		return utils.Err(c, 403, "hasabyňyz bloklanan")
	}
	passwordGuard.reset(phone)
	token, err := middleware.SignToken(h.Cfg, &u)
	if err != nil {
		return utils.Err(c, 500, "token döredip bolmady")
	}
	withPasswordFlag(&u)
	if d, err := h.driverOf(u.ID); err == nil {
		d.User = nil
		u.Driver = d
	}
	return utils.OK(c, fiber.Map{"token": token, "user": u, "is_new": false})
}

// SetPassword POST /auth/password {old_password, new_password} — a driver sets or changes the app password.
// The first password needs no old one: the driver is already logged in with an SMS code.
func (h *Handler) SetPassword(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return utils.Err(c, 401, err.Error())
	}
	if u.Role != models.RoleDriver {
		return utils.Err(c, 403, "parol bilen giriş diňe sürüjiler üçin")
	}
	var req struct {
		Old string `json:"old_password"`
		New string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil || len(strings.TrimSpace(req.New)) < minPasswordLen {
		return utils.Err(c, 400, "parol azyndan 6 simwol bolmaly")
	}
	if u.PasswordHash != "" && bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Old)) != nil {
		return utils.Err(c, 400, "häzirki parol nädogry")
	}
	hash, err := hashPassword(req.New)
	if err != nil {
		return utils.Err(c, 500, "parol ýatda saklanmady")
	}
	h.DB.Model(u).Update("password_hash", hash)
	return utils.OK(c, true)
}

// AdminSetPassword PUT /admin/users/:id/password {password} — set or reset a driver's app password.
func (h *Handler) AdminSetPassword(c *fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return utils.Err(c, 400, err.Error())
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil || len(strings.TrimSpace(req.Password)) < minPasswordLen {
		return utils.Err(c, 400, "parol azyndan 6 simwol bolmaly")
	}
	var u models.User
	if err := h.DB.First(&u, id).Error; err != nil {
		return utils.Err(c, 404, "ulanyjy tapylmady")
	}
	if u.Role != models.RoleDriver {
		return utils.Err(c, 400, "parol diňe sürüji hasabyna bellenýär")
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return utils.Err(c, 500, "parol ýatda saklanmady")
	}
	h.DB.Model(&u).Update("password_hash", hash)
	passwordGuard.reset(u.Phone)
	return utils.OK(c, true)
}

// AdminCreateDriver POST /admin/drivers/account {phone, name, password}
// Creates a driver login so the driver can enter the app with phone + password and register the car.
func (h *Handler) AdminCreateDriver(c *fiber.Ctx) error {
	var req struct {
		Phone    string `json:"phone"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	phone := utils.NormalizePhone(req.Phone)
	if !utils.ValidPhone(phone) {
		return utils.Err(c, 400, "telefon belgisi nädogry (+993 6X XXXXXX)")
	}
	if len(strings.TrimSpace(req.Password)) < minPasswordLen {
		return utils.Err(c, 400, "parol azyndan 6 simwol bolmaly")
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return utils.Err(c, 500, "parol ýatda saklanmady")
	}
	var u models.User
	if err := h.DB.Where("phone = ?", phone).First(&u).Error; err == nil {
		if u.Role == models.RoleAdmin {
			return utils.Err(c, 400, "bu belgi admin hasaby")
		}
		upd := map[string]any{"role": models.RoleDriver, "password_hash": hash}
		if n := strings.TrimSpace(req.Name); n != "" {
			upd["name"] = n
		}
		h.DB.Model(&u).Updates(upd)
	} else {
		u = models.User{Phone: phone, Name: strings.TrimSpace(req.Name), Role: models.RoleDriver, PasswordHash: hash, Rating: 5, Lang: "tk"}
		if err := h.DB.Create(&u).Error; err != nil {
			return utils.Err(c, 500, "ulanyjy döredip bolmady")
		}
	}
	h.DB.First(&u, u.ID)
	passwordGuard.reset(phone)
	return c.Status(201).JSON(fiber.Map{"success": true, "data": withPasswordFlag(&u)})
}
