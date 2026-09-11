package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"movemate/internal/auth"
	"movemate/internal/config"
)

type AuthHandler struct {
	DB  *pgxpool.Pool
	Cfg config.Config
}

type RegisterRequest struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))

	if req.Name == "" || req.Phone == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "name, phone and password are required",
		})
		return
	}

	if req.Role == "" {
		req.Role = "customer"
	}

	if req.Role != "customer" && req.Role != "driver" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "role must be customer or driver",
		})
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to secure password",
		})
		return
	}

	var userID string

	err = h.DB.QueryRow(
		c,
		`INSERT INTO users
			(name, phone, email, password_hash, role)
		 VALUES
			($1, $2, NULLIF($3, ''), $4, $5)
		 RETURNING id`,
		req.Name,
		req.Phone,
		req.Email,
		passwordHash,
		req.Role,
	).Scan(&userID)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "phone or email may already be registered",
		})
		return
	}

	token, err := auth.GenerateToken(
		userID,
		req.Role,
		h.Cfg.JWTSecret,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create authentication token",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "registration successful",
		"user": gin.H{
			"id":    userID,
			"name":  req.Name,
			"phone": req.Phone,
			"email": req.Email,
			"role":  req.Role,
		},
		"token": token,
	})
}

type LoginRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Phone = strings.TrimSpace(req.Phone)

	if req.Phone == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "phone and password are required",
		})
		return
	}

	var (
		userID       string
		name         string
		phone        string
		email        *string
		passwordHash string
		role         string
		isActive     bool
	)

	err := h.DB.QueryRow(
		c,
		`SELECT id, name, phone, email, password_hash, role, is_active
		 FROM users
		 WHERE phone = $1`,
		req.Phone,
	).Scan(
		&userID,
		&name,
		&phone,
		&email,
		&passwordHash,
		&role,
		&isActive,
	)

	if err != nil || !isActive || !auth.CheckPassword(req.Password, passwordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "invalid phone or password",
		})
		return
	}

	token, err := auth.GenerateToken(
		userID,
		role,
		h.Cfg.JWTSecret,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create authentication token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "login successful",
		"user": gin.H{
			"id":     userID,
			"name":   name,
			"phone":  phone,
			"email":  email,
			"role":   role,
			"active": isActive,
		},
		"token": token,
	})
}

type UpdateProfileRequest struct {
	Name  *string `json:"name"`
	Phone *string `json:"phone"`
	Email *string `json:"email"`
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "user authentication required",
		})
		return
	}

	userID, ok := userIDValue.(string)
	if !ok || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "invalid authenticated user",
		})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	if req.Name == nil && req.Phone == nil && req.Email == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "at least one profile field is required",
		})
		return
	}

	if req.Name != nil {
		value := strings.TrimSpace(*req.Name)
		if value == "" || len(value) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "name must be between 1 and 100 characters",
			})
			return
		}
		req.Name = &value
	}

	if req.Phone != nil {
		value := strings.TrimSpace(*req.Phone)
		if value == "" || len(value) > 20 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "phone must be between 1 and 20 characters",
			})
			return
		}
		req.Phone = &value
	}

	if req.Email != nil {
		value := strings.TrimSpace(*req.Email)
		if value == "" {
			req.Email = nil
		} else {
			if len(value) > 255 {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "email is too long",
				})
				return
			}
			req.Email = &value
		}
	}

	var (
		name     string
		phone    string
		email    *string
		role     string
		isActive bool
	)

	err := h.DB.QueryRow(
		c,
		`UPDATE users
		 SET
			name = COALESCE($2, name),
			phone = COALESCE($3, phone),
			email = COALESCE($4, email),
			updated_at = NOW()
		 WHERE id = $1
		 RETURNING name, phone, email, role, is_active`,
		userID,
		req.Name,
		req.Phone,
		req.Email,
	).Scan(
		&name,
		&phone,
		&email,
		&role,
		&isActive,
	)

	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "phone or email is already in use",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to update profile",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "profile updated successfully",
		"user": gin.H{
			"id":     userID,
			"name":   name,
			"phone":  phone,
			"email":  email,
			"role":   role,
			"active": isActive,
		},
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "user authentication required",
		})
		return
	}

	userID, ok := userIDValue.(string)
	if !ok || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "invalid authenticated user",
		})
		return
	}

	var (
		name     string
		phone    string
		email    *string
		role     string
		isActive bool
	)

	err := h.DB.QueryRow(
		c,
		`SELECT name, phone, email, role, is_active
		 FROM users
		 WHERE id = $1`,
		userID,
	).Scan(
		&name,
		&phone,
		&email,
		&role,
		&isActive,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "user not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"user": gin.H{
			"id":     userID,
			"name":   name,
			"phone":  phone,
			"email":  email,
			"role":   role,
			"active": isActive,
		},
	})
}
