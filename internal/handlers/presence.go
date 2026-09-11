package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PresenceHandler struct {
	DB *pgxpool.Pool
}

func (h *PresenceHandler) SetOnline(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can update presence",
		})
		return
	}

	_, err := h.DB.Exec(
		c,
		`INSERT INTO driver_presence (
			driver_id,
			is_online,
			last_seen_at,
			updated_at
		)
		VALUES ($1, TRUE, NOW(), NOW())
		ON CONFLICT (driver_id)
		DO UPDATE SET
			is_online = TRUE,
			last_seen_at = NOW(),
			updated_at = NOW()`,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to set driver online",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"is_online":    true,
		"last_seen_at": time.Now().UTC(),
	})
}

func (h *PresenceHandler) SetOffline(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can update presence",
		})
		return
	}

	_, err := h.DB.Exec(
		c,
		`INSERT INTO driver_presence (
			driver_id,
			is_online,
			last_seen_at,
			updated_at
		)
		VALUES ($1, FALSE, NOW(), NOW())
		ON CONFLICT (driver_id)
		DO UPDATE SET
			is_online = FALSE,
			last_seen_at = NOW(),
			updated_at = NOW()`,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to set driver offline",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"is_online": false,
	})
}

func (h *PresenceHandler) Heartbeat(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can send presence heartbeat",
		})
		return
	}

	result, err := h.DB.Exec(
		c,
		`UPDATE driver_presence
		SET
			is_online = TRUE,
			last_seen_at = NOW(),
			updated_at = NOW()
		WHERE driver_id = $1`,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to update driver heartbeat",
		})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "driver presence not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"is_online":    true,
		"last_seen_at": time.Now().UTC(),
	})
}
