package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"movemate/internal/geo"
)

type NearbyDriversHandler struct {
	DB *pgxpool.Pool
}

type nearbyDriver struct {
	ID            string
	Name          string
	VehicleID     string
	VehicleType   string
	VehicleNumber string
	VehicleModel  *string
	Latitude      float64
	Longitude     float64
	DistanceKm    float64
	LastSeenAt    *time.Time
}

func (h *NearbyDriversHandler) List(c *gin.Context) {
	role := c.GetString("role")

	if role != "customer" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only customers can discover nearby drivers",
		})
		return
	}

	bookingID := c.Param("id")
	userID := c.GetString("user_id")

	var (
		customerID  string
		vehicleType string
		pickupLat   *float64
		pickupLng   *float64
		status      string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			customer_id,
			vehicle_type,
			pickup_lat,
			pickup_lng,
			status
		FROM bookings
		WHERE id = $1`,
		bookingID,
	).Scan(
		&customerID,
		&vehicleType,
		&pickupLat,
		&pickupLng,
		&status,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking not found",
		})
		return
	}

	if customerID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not have access to this booking",
		})
		return
	}

	if status != "requested" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "nearby drivers are only available for requested bookings",
		})
		return
	}

	if pickupLat == nil || pickupLng == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "booking pickup coordinates are required",
		})
		return
	}

	rows, err := h.DB.Query(
		c,
		`SELECT
			u.id,
			u.name,
			v.id,
			v.vehicle_type,
			v.vehicle_number,
			v.model,
			dl.latitude,
			dl.longitude,
			dp.last_seen_at
		FROM users u
		INNER JOIN vehicles v
			ON v.driver_id = u.id
			AND v.is_active = TRUE
			AND v.vehicle_type = $1
		INNER JOIN driver_presence dp
			ON dp.driver_id = u.id
			AND dp.is_online = TRUE
		INNER JOIN driver_locations dl
			ON dl.driver_id = u.id
		WHERE u.role = 'driver'
			AND u.is_active = TRUE
			AND dl.updated_at >= NOW() - INTERVAL '2 minutes'
		ORDER BY dl.updated_at DESC`,
		vehicleType,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load nearby drivers",
		})
		return
	}
	defer rows.Close()

	drivers := make([]gin.H, 0)

	for rows.Next() {
		var driver nearbyDriver

		if err := rows.Scan(
			&driver.ID,
			&driver.Name,
			&driver.VehicleID,
			&driver.VehicleType,
			&driver.VehicleNumber,
			&driver.VehicleModel,
			&driver.Latitude,
			&driver.Longitude,
			&driver.LastSeenAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read nearby driver",
			})
			return
		}

		driver.DistanceKm = geo.HaversineKm(
			*pickupLat,
			*pickupLng,
			driver.Latitude,
			driver.Longitude,
		)

		if driver.DistanceKm > 10 {
			continue
		}

		drivers = append(drivers, gin.H{
			"id":   driver.ID,
			"name": driver.Name,
			"vehicle": gin.H{
				"id":     driver.VehicleID,
				"type":   driver.VehicleType,
				"number": driver.VehicleNumber,
				"model":  driver.VehicleModel,
			},
			"latitude":     driver.Latitude,
			"longitude":    driver.Longitude,
			"distance_km":  driver.DistanceKm,
			"last_seen_at": driver.LastSeenAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load nearby drivers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"booking_id":   bookingID,
		"vehicle_type": vehicleType,
		"radius_km":    10,
		"drivers":      drivers,
	})
}
