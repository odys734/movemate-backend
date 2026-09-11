package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LocationHandler struct {
	DB *pgxpool.Pool
}

type updateDriverLocationRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (h *LocationHandler) Update(c *gin.Context) {
	role := c.GetString("role")
	userID := c.GetString("user_id")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can update location",
		})
		return
	}

	var req updateDriverLocationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	if req.Latitude < -90 || req.Latitude > 90 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "latitude must be between -90 and 90",
		})
		return
	}

	if req.Longitude < -180 || req.Longitude > 180 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "longitude must be between -180 and 180",
		})
		return
	}

	var updatedAt time.Time

	err := h.DB.QueryRow(
		c,
		`INSERT INTO driver_locations (
			driver_id,
			latitude,
			longitude,
			updated_at
		)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (driver_id)
		DO UPDATE SET
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			updated_at = NOW()
		RETURNING updated_at`,
		userID,
		req.Latitude,
		req.Longitude,
	).Scan(&updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to update driver location",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"latitude":   req.Latitude,
		"longitude":  req.Longitude,
		"updated_at": updatedAt.UTC(),
	})
}

func (h *LocationHandler) Get(c *gin.Context) {
	role := c.GetString("role")
	userID := c.GetString("user_id")
	bookingID := c.Param("id")

	if role != "customer" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only customers can view driver location",
		})
		return
	}

	var (
		driverID  string
		latitude  float64
		longitude float64
		updatedAt time.Time
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			b.driver_id,
			dl.latitude,
			dl.longitude,
			dl.updated_at
		FROM bookings b
		INNER JOIN driver_locations dl
			ON dl.driver_id = b.driver_id
		WHERE b.id = $1
			AND b.customer_id = $2
			AND b.driver_id IS NOT NULL`,
		bookingID,
		userID,
	).Scan(
		&driverID,
		&latitude,
		&longitude,
		&updatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "driver location not available",
		})
		return
	}

	stale := time.Since(updatedAt) > 2*time.Minute

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"booking_id": bookingID,
		"driver_id":  driverID,
		"latitude":   latitude,
		"longitude":  longitude,
		"updated_at": updatedAt.UTC(),
		"is_stale":   stale,
	})
}
