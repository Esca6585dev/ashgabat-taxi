package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/ashgabat-taxi/backend/internal/middleware"
	"github.com/ashgabat-taxi/backend/internal/models"
	"github.com/ashgabat-taxi/backend/internal/services"
	"github.com/ashgabat-taxi/backend/internal/utils"
)

// SendOTP POST /auth/send-otp {phone}
func (h *Handler) SendOTP(c *fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	phone := utils.NormalizePhone(req.Phone)
	if !utils.ValidPhone(phone) {
		return utils.Err(c, 400, "telefon belgisi nädogry (+993 6X XXXXXX)")
	}
	var u models.User
	if err := h.DB.Where("phone = ?", phone).First(&u).Error; err == nil && u.IsBlocked {
		return utils.Err(c, 403, "hasabyňyz bloklanan")
	}
	code, err := h.OTP.Send(phone)
	if err != nil {
		if errors.Is(err, services.ErrTooMany) {
			return utils.Err(c, 429, err.Error())
		}
		return utils.Err(c, 500, "SMS ugradyp bolmady")
	}
	resp := fiber.Map{"phone": phone, "ttl_min": h.Cfg.OTPTTLMin}
	if h.Cfg.IsDev() {
		resp["dev_code"] = code
	}
	return utils.OK(c, resp)
}

// VerifyOTP POST /auth/verify-otp {phone, code, role?, name?}
// role is "client" (default) or "driver" depending on which app is logging in.
func (h *Handler) VerifyOTP(c *fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
		Role  string `json:"role"`
		Name  string `json:"name"`
		Lang  string `json:"lang"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	phone := utils.NormalizePhone(req.Phone)
	if !h.OTP.Verify(phone, strings.TrimSpace(req.Code)) {
		return utils.Err(c, 400, "kod nädogry ýa-da möhleti geçen")
	}
	role := models.RoleClient
	if req.Role == string(models.RoleDriver) {
		role = models.RoleDriver
	}
	var u models.User
	isNew := false
	if err := h.DB.Where("phone = ?", phone).First(&u).Error; err != nil {
		u = models.User{Phone: phone, Name: strings.TrimSpace(req.Name), Role: role, Rating: 5, Lang: "tk"}
		if req.Lang != "" {
			u.Lang = req.Lang
		}
		if err := h.DB.Create(&u).Error; err != nil {
			return utils.Err(c, 500, "ulanyjy döredip bolmady")
		}
		isNew = true
	} else {
		if u.IsBlocked {
			return utils.Err(c, 403, "hasabyňyz bloklanan")
		}
		if u.Role == models.RoleAdmin {
			return utils.Err(c, 403, "admin hasaby programma arkaly girip bilmeýär")
		}
		// a client logging in from the driver app becomes a driver account
		if role == models.RoleDriver && u.Role != models.RoleDriver {
			u.Role = models.RoleDriver
			h.DB.Model(&u).Update("role", u.Role)
		}
	}
	token, err := middleware.SignToken(h.Cfg, &u)
	if err != nil {
		return utils.Err(c, 500, "token döredip bolmady")
	}
	withPasswordFlag(&u)
	if u.Role == models.RoleDriver {
		if d, err := h.driverOf(u.ID); err == nil {
			d.User = nil
			u.Driver = d
		}
	}
	return utils.OK(c, fiber.Map{"token": token, "user": u, "is_new": isNew})
}

// Me GET /auth/me
func (h *Handler) Me(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return utils.Err(c, 401, err.Error())
	}
	withPasswordFlag(u)
	if u.Role == models.RoleDriver {
		if d, err := h.driverOf(u.ID); err == nil {
			d.User = nil
			u.Driver = d
		}
	}
	return utils.OK(c, u)
}

// UpdateProfile PUT /auth/profile {name, lang, fcm_token}
func (h *Handler) UpdateProfile(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return utils.Err(c, 401, err.Error())
	}
	var req struct {
		Name     *string `json:"name"`
		Lang     *string `json:"lang"`
		FCMToken *string `json:"fcm_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	upd := map[string]any{}
	if req.Name != nil {
		upd["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Lang != nil {
		upd["lang"] = *req.Lang
	}
	if req.FCMToken != nil {
		upd["fcm_token"] = *req.FCMToken
	}
	if len(upd) > 0 {
		h.DB.Model(u).Updates(upd)
	}
	return h.Me(c)
}

// AdminLogin POST /auth/admin/login {phone, password}
func (h *Handler) AdminLogin(c *fiber.Ctx) error {
	var req struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.Err(c, 400, "nädogry maglumat")
	}
	var u models.User
	if err := h.DB.Where("phone = ? AND role = ?", utils.NormalizePhone(req.Phone), models.RoleAdmin).First(&u).Error; err != nil {
		return utils.Err(c, 401, "telefon ýa-da parol nädogry")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		return utils.Err(c, 401, "telefon ýa-da parol nädogry")
	}
	token, err := middleware.SignToken(h.Cfg, &u)
	if err != nil {
		return utils.Err(c, 500, "token döredip bolmady")
	}
	return utils.OK(c, fiber.Map{"token": token, "user": u})
}

// ChangeAdminPassword POST /admin/password {old_password, new_password}
func (h *Handler) ChangeAdminPassword(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return utils.Err(c, 401, err.Error())
	}
	var req struct {
		Old string `json:"old_password"`
		New string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil || len(req.New) < 6 {
		return utils.Err(c, 400, "täze parol azyndan 6 simwol bolmaly")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Old)) != nil {
		return utils.Err(c, 400, "köne parol nädogry")
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	h.DB.Model(u).Update("password_hash", string(hash))
	return utils.OK(c, true)
}
