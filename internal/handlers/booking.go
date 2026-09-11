package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"movemate/internal/geo"
)

type BookingHandler struct {
	DB *pgxpool.Pool
}

type CreateBookingRequest struct {
	PickupAddress     string     `json:"pickup_address"`
	PickupLat         *float64   `json:"pickup_lat"`
	PickupLng         *float64   `json:"pickup_lng"`
	DropAddress       string     `json:"drop_address"`
	DropLat           *float64   `json:"drop_lat"`
	DropLng           *float64   `json:"drop_lng"`
	VehicleType       string     `json:"vehicle_type"`
	ItemDescription   string     `json:"item_description"`
	ItemQuantity      int        `json:"item_quantity"`
	EstimatedWeightKg *float64   `json:"estimated_weight_kg"`
	ScheduledAt       *time.Time `json:"scheduled_at"`
}

func (h *BookingHandler) List(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	rows, err := h.DB.Query(
		c,
		`SELECT
			id,
			pickup_address,
			drop_address,
			vehicle_type,
			item_description,
			item_quantity,
			estimated_weight_kg,
			estimated_fare,
			final_fare,
			status,
			scheduled_at,
			created_at
		 FROM bookings
		 WHERE customer_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load bookings",
		})
		return
	}
	defer rows.Close()

	bookings := make([]gin.H, 0)

	for rows.Next() {
		var (
			id                string
			pickupAddress     string
			dropAddress       string
			vehicleType       string
			itemDescription   string
			itemQuantity      int
			estimatedWeightKg *float64
			estimatedFare     *float64
			finalFare         *float64
			status            string
			scheduledAt       *time.Time
			createdAt         time.Time
		)

		if err := rows.Scan(
			&id,
			&pickupAddress,
			&dropAddress,
			&vehicleType,
			&itemDescription,
			&itemQuantity,
			&estimatedWeightKg,
			&estimatedFare,
			&finalFare,
			&status,
			&scheduledAt,
			&createdAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read booking",
			})
			return
		}

		bookings = append(bookings, gin.H{
			"id":                  id,
			"pickup_address":      pickupAddress,
			"drop_address":        dropAddress,
			"vehicle_type":        vehicleType,
			"item_description":    itemDescription,
			"item_quantity":       itemQuantity,
			"estimated_weight_kg": estimatedWeightKg,
			"estimated_fare":      estimatedFare,
			"final_fare":          finalFare,
			"status":              status,
			"scheduled_at":        scheduledAt,
			"created_at":          createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load bookings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"bookings": bookings,
	})
}

func (h *BookingHandler) Details(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	bookingID := c.Param("id")

	var (
		id                string
		customerID        string
		driverID          *string
		vehicleID         *string
		pickupAddress     string
		pickupLat         *float64
		pickupLng         *float64
		dropAddress       string
		dropLat           *float64
		dropLng           *float64
		vehicleType       string
		itemDescription   *string
		itemQuantity      int
		estimatedWeightKg *float64
		estimatedFare     *float64
		finalFare         *float64
		status            string
		scheduledAt       *time.Time
		createdAt         time.Time
		updatedAt         time.Time

		driverName  *string
		driverPhone *string

		vehicleNumber *string
		vehicleModel  *string
	)

	query := `
		SELECT
			b.id,
			b.customer_id,
			b.driver_id,
			b.vehicle_id,
			b.pickup_address,
			b.pickup_lat,
			b.pickup_lng,
			b.drop_address,
			b.drop_lat,
			b.drop_lng,
			b.vehicle_type,
			b.item_description,
			b.item_quantity,
			b.estimated_weight_kg,
			b.estimated_fare,
			b.final_fare,
			b.status,
			b.scheduled_at,
			b.created_at,
			b.updated_at,
			d.name,
			d.phone,
			v.vehicle_number,
			v.model
		FROM bookings b
		LEFT JOIN users d ON d.id = b.driver_id
		LEFT JOIN vehicles v ON v.id = b.vehicle_id
		WHERE b.id = $1
	`

	err := h.DB.QueryRow(c, query, bookingID).Scan(
		&id,
		&customerID,
		&driverID,
		&vehicleID,
		&pickupAddress,
		&pickupLat,
		&pickupLng,
		&dropAddress,
		&dropLat,
		&dropLng,
		&vehicleType,
		&itemDescription,
		&itemQuantity,
		&estimatedWeightKg,
		&estimatedFare,
		&finalFare,
		&status,
		&scheduledAt,
		&createdAt,
		&updatedAt,
		&driverName,
		&driverPhone,
		&vehicleNumber,
		&vehicleModel,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking not found",
		})
		return
	}

	if role == "customer" {
		if customerID != userID {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "you do not have access to this booking",
			})
			return
		}
	} else if role == "driver" {
		if driverID == nil || *driverID != userID {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "you do not have access to this booking",
			})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "access denied",
		})
		return
	}

	driver := interface{}(nil)
	if driverID != nil {
		driver = gin.H{
			"id":    *driverID,
			"name":  driverName,
			"phone": driverPhone,
		}
	}

	vehicle := interface{}(nil)
	if vehicleID != nil {
		vehicle = gin.H{
			"id":     *vehicleID,
			"type":   vehicleType,
			"number": vehicleNumber,
			"model":  vehicleModel,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"booking": gin.H{
			"id":                  id,
			"pickup_address":      pickupAddress,
			"pickup_lat":          pickupLat,
			"pickup_lng":          pickupLng,
			"drop_address":        dropAddress,
			"drop_lat":            dropLat,
			"drop_lng":            dropLng,
			"vehicle_type":        vehicleType,
			"item_description":    itemDescription,
			"item_quantity":       itemQuantity,
			"estimated_weight_kg": estimatedWeightKg,
			"estimated_fare":      estimatedFare,
			"final_fare":          finalFare,
			"status":              status,
			"scheduled_at":        scheduledAt,
			"created_at":          createdAt,
			"updated_at":          updatedAt,
			"driver":              driver,
			"vehicle":             vehicle,
		},
	})
}

func (h *BookingHandler) GenerateDeliveryOTP(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "customer" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only the customer can generate the delivery OTP",
		})
		return
	}

	bookingID := c.Param("id")

	var (
		customerID string
		status     string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT customer_id, status
		 FROM bookings
		 WHERE id = $1`,
		bookingID,
	).Scan(&customerID, &status)

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

	if status != "in_transit" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "delivery OTP can only be generated when the booking is in transit",
		})
		return
	}

	var randomValue uint32
	if err := binaryReadUint32(&randomValue); err != nil {
		log.Printf(
			"failed to generate delivery OTP: booking_id=%s err=%v",
			bookingID,
			err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to generate delivery OTP",
		})
		return
	}

	otpNumber := randomValue % 1000000
	otp := fmt.Sprintf("%06d", otpNumber)

	hash := sha256.Sum256([]byte(otp))
	otpHash := hex.EncodeToString(hash[:])
	expiresAt := time.Now().Add(10 * time.Minute)

	_, err = h.DB.Exec(
		c,
		`INSERT INTO delivery_otps (
			booking_id,
			otp_hash,
			expires_at,
			verified_at,
			attempts
		)
		VALUES ($1, $2, $3, NULL, 0)
		ON CONFLICT (booking_id)
		DO UPDATE SET
			otp_hash = EXCLUDED.otp_hash,
			expires_at = EXCLUDED.expires_at,
			verified_at = NULL,
			attempts = 0`,
		bookingID,
		otpHash,
		expiresAt,
	)

	if err != nil {
		log.Printf(
			"failed to save delivery OTP: booking_id=%s err=%v",
			bookingID,
			err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to save delivery OTP",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"booking_id": bookingID,
		"otp":        otp,
		"expires_at": expiresAt,
	})
}

func binaryReadUint32(value *uint32) error {
	var bytes [4]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return err
	}

	*value = uint32(bytes[0])<<24 |
		uint32(bytes[1])<<16 |
		uint32(bytes[2])<<8 |
		uint32(bytes[3])

	return nil
}

func (h *BookingHandler) Create(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "customer" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only customers can create bookings",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	var req CreateBookingRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.PickupAddress = strings.TrimSpace(req.PickupAddress)
	req.DropAddress = strings.TrimSpace(req.DropAddress)
	req.VehicleType = strings.ToLower(strings.TrimSpace(req.VehicleType))
	req.ItemDescription = strings.TrimSpace(req.ItemDescription)

	if req.PickupAddress == "" || req.DropAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "pickup_address and drop_address are required",
		})
		return
	}

	validTypes := map[string]bool{
		"bike":   true,
		"auto":   true,
		"car":    true,
		"pickup": true,
		"tempo":  true,
		"truck":  true,
		"other":  true,
	}

	if !validTypes[req.VehicleType] {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid vehicle type",
		})
		return
	}

	if req.ItemQuantity < 1 {
		req.ItemQuantity = 1
	}

	var bookingID string

	err := h.DB.QueryRow(
		c,
		`INSERT INTO bookings (
			customer_id,
			pickup_address,
			pickup_lat,
			pickup_lng,
			drop_address,
			drop_lat,
			drop_lng,
			vehicle_type,
			item_description,
			item_quantity,
			estimated_weight_kg,
			estimated_fare,
			scheduled_at,
			status
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, 'requested'
		)
		RETURNING id`,
		userID,
		req.PickupAddress,
		req.PickupLat,
		req.PickupLng,
		req.DropAddress,
		req.DropLat,
		req.DropLng,
		req.VehicleType,
		req.ItemDescription,
		req.ItemQuantity,
		req.EstimatedWeightKg,
		nil,
		req.ScheduledAt,
	).Scan(&bookingID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create booking",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "booking created successfully",
		"booking": gin.H{
			"id":                  bookingID,
			"status":              "requested",
			"vehicle_type":        req.VehicleType,
			"pickup_address":      req.PickupAddress,
			"drop_address":        req.DropAddress,
			"item_description":    req.ItemDescription,
			"item_quantity":       req.ItemQuantity,
			"estimated_weight_kg": req.EstimatedWeightKg,
			"scheduled_at":        req.ScheduledAt,
		},
	})
}

type AcceptBookingRequest struct {
	VehicleID string `json:"vehicle_id"`
}

func (h *BookingHandler) AvailableForDriver(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can view available bookings",
		})
		return
	}

	userID, _ := c.Get("user_id")

	rows, err := h.DB.Query(
		c,
		`SELECT
			b.id,
			b.pickup_address,
			b.drop_address,
			b.vehicle_type,
			b.item_description,
			b.item_quantity,
			b.estimated_weight_kg,
			b.estimated_fare,
			b.scheduled_at,
			b.created_at
		FROM bookings b
		WHERE b.status = 'requested'
		AND EXISTS (
			SELECT 1
			FROM vehicles v
			WHERE v.driver_id = $1
			AND v.vehicle_type = b.vehicle_type
			AND v.is_active = TRUE
		)
		ORDER BY b.created_at ASC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load available bookings",
		})
		return
	}
	defer rows.Close()

	bookings := make([]gin.H, 0)

	for rows.Next() {
		var (
			id                string
			pickupAddress     string
			dropAddress       string
			vehicleType       string
			itemDescription   string
			itemQuantity      int
			estimatedWeightKg *float64
			estimatedFare     *float64
			scheduledAt       *time.Time
			createdAt         time.Time
		)

		if err := rows.Scan(
			&id,
			&pickupAddress,
			&dropAddress,
			&vehicleType,
			&itemDescription,
			&itemQuantity,
			&estimatedWeightKg,
			&estimatedFare,
			&scheduledAt,
			&createdAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read booking",
			})
			return
		}

		bookings = append(bookings, gin.H{
			"id":                  id,
			"pickup_address":      pickupAddress,
			"drop_address":        dropAddress,
			"vehicle_type":        vehicleType,
			"item_description":    itemDescription,
			"item_quantity":       itemQuantity,
			"estimated_weight_kg": estimatedWeightKg,
			"estimated_fare":      estimatedFare,
			"scheduled_at":        scheduledAt,
			"created_at":          createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load available bookings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"bookings": bookings,
	})
}

type UpdateBookingStatusRequest struct {
	Status string `json:"status"`
}

var allowedBookingTransitions = map[string]string{
	"accepted":        "driver_arriving",
	"driver_arriving": "arrived",
	"arrived":         "loading",
	"loading":         "in_transit",
	"in_transit":      "delivered",
}

func (h *BookingHandler) UpdateStatus(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can update booking status",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	bookingID := c.Param("id")

	var req UpdateBookingStatusRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Status = strings.ToLower(strings.TrimSpace(req.Status))

	if req.Status == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "status is required",
		})
		return
	}

	var (
		currentStatus string
		customerID    string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
                        status,
                        customer_id
                 FROM bookings
                 WHERE id = $1
                   AND driver_id = $2`,
		bookingID,
		userID,
	).Scan(
		&currentStatus,
		&customerID,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking not found or not assigned to you",
		})
		return
	}

	expectedNext, exists := allowedBookingTransitions[currentStatus]

	if !exists || expectedNext != req.Status {
		c.JSON(http.StatusConflict, gin.H{
			"success":          false,
			"error":            "invalid booking status transition",
			"current_status":   currentStatus,
			"requested_status": req.Status,
		})
		return
	}

	if req.Status == "delivered" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "delivery OTP verification is required to mark this booking as delivered",
		})
		return
	}

	var updatedStatus string

	err = h.DB.QueryRow(
		c,
		`UPDATE bookings
                 SET status = $1,
                     updated_at = NOW()
                 WHERE id = $2
                   AND driver_id = $3
                   AND status = $4
                 RETURNING status`,
		req.Status,
		bookingID,
		userID,
		currentStatus,
	).Scan(&updatedStatus)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking status could not be updated",
		})
		return
	}

	var notificationTitle string
	var notificationMessage string

	switch updatedStatus {
	case "driver_arriving":
		notificationTitle = "Driver is on the way"
		notificationMessage = "Your driver is on the way to the pickup location."
	case "arrived":
		notificationTitle = "Driver has arrived"
		notificationMessage = "Your driver has arrived at the pickup location."
	case "loading":
		notificationTitle = "Loading started"
		notificationMessage = "Loading for your MoveMate booking has started."
	case "in_transit":
		notificationTitle = "Move is in transit"
		notificationMessage = "Your MoveMate booking is now in transit."
	case "delivered":
		notificationTitle = "Move delivered"
		notificationMessage = "Your MoveMate booking has been marked as delivered."
	}

	if notificationTitle != "" {
		_, notificationErr := h.DB.Exec(
			c,
			`INSERT INTO notifications (
                                user_id,
                                booking_id,
                                type,
                                title,
                                message
                        )
                        VALUES ($1, $2, $3, $4, $5)`,
			customerID,
			bookingID,
			"booking_status_"+updatedStatus,
			notificationTitle,
			notificationMessage,
		)

		if notificationErr != nil {
			log.Printf(
				"failed to create booking status notification: booking_id=%s customer_id=%s status=%s err=%v",
				bookingID,
				customerID,
				updatedStatus,
				notificationErr,
			)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "booking status updated successfully",
		"booking": gin.H{
			"id":     bookingID,
			"status": updatedStatus,
		},
	})
}

type VerifyDeliveryOTPRequest struct {
	OTP string `json:"otp"`
}

func (h *BookingHandler) VerifyDeliveryOTP(c *gin.Context) {
	role, _ := c.Get("role")
	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only the assigned driver can verify the delivery OTP",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	bookingID := c.Param("id")

	var req VerifyDeliveryOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.OTP = strings.TrimSpace(req.OTP)

	if len(req.OTP) != 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "OTP must be 6 digits",
		})
		return
	}

	for _, ch := range req.OTP {
		if ch < '0' || ch > '9' {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "OTP must contain only digits",
			})
			return
		}
	}

	otpHashBytes := sha256.Sum256([]byte(req.OTP))
	otpHash := hex.EncodeToString(otpHashBytes[:])

	tx, err := h.DB.Begin(c)
	if err != nil {
		log.Printf(
			"failed to begin delivery OTP verification: booking_id=%s err=%v",
			bookingID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to verify delivery OTP",
		})
		return
	}
	defer tx.Rollback(c)

	var (
		currentStatus string
		customerID    string
		storedHash    string
		expiresAt     time.Time
		verifiedAt    *time.Time
		attempts      int
	)

	err = tx.QueryRow(
		c,
		`SELECT
			b.status,
			b.customer_id,
			o.otp_hash,
			o.expires_at,
			o.verified_at,
			o.attempts
		 FROM bookings b
		 JOIN delivery_otps o ON o.booking_id = b.id
		 WHERE b.id = $1
		   AND b.driver_id = $2
		 FOR UPDATE`,
		bookingID,
		userID,
	).Scan(
		&currentStatus,
		&customerID,
		&storedHash,
		&expiresAt,
		&verifiedAt,
		&attempts,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking or delivery OTP not found",
		})
		return
	}

	if currentStatus != "in_transit" {
		c.JSON(http.StatusConflict, gin.H{
			"success":        false,
			"error":          "booking must be in transit for delivery verification",
			"current_status": currentStatus,
		})
		return
	}

	if verifiedAt != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "delivery OTP has already been verified",
		})
		return
	}

	if attempts >= 5 {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"error":   "maximum delivery OTP attempts exceeded",
		})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(http.StatusGone, gin.H{
			"success": false,
			"error":   "delivery OTP has expired",
		})
		return
	}

	if storedHash != otpHash {
		_, err = tx.Exec(
			c,
			`UPDATE delivery_otps
			 SET attempts = attempts + 1
			 WHERE booking_id = $1`,
			bookingID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to record OTP attempt",
			})
			return
		}

		if err := tx.Commit(c); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to record OTP attempt",
			})
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{
			"success":            false,
			"error":              "invalid delivery OTP",
			"attempts_remaining": 4 - attempts,
		})
		return
	}

	_, err = tx.Exec(
		c,
		`UPDATE delivery_otps
		 SET verified_at = NOW()
		 WHERE booking_id = $1
		   AND verified_at IS NULL`,
		bookingID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to verify delivery OTP",
		})
		return
	}

	var updatedStatus string
	err = tx.QueryRow(
		c,
		`UPDATE bookings
		 SET status = 'delivered',
		     updated_at = NOW()
		 WHERE id = $1
		   AND driver_id = $2
		   AND status = 'in_transit'
		 RETURNING status`,
		bookingID,
		userID,
	).Scan(&updatedStatus)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking could not be marked as delivered",
		})
		return
	}

	// In-app payments are held until delivery OTP verification.
	// Successful delivery makes the held payment payout-eligible.
	// Cash payments are intentionally unaffected.
	_, payoutErr := tx.Exec(
		c,
		`UPDATE payments
                 SET payout_status = 'eligible',
                     updated_at = NOW()
                 WHERE booking_id = $1
                   AND payment_type = 'in_app'
                   AND status = 'paid'
                   AND payout_status = 'held'`,
		bookingID,
	)
	if payoutErr != nil {
		log.Printf(
			"failed to mark payment payout eligible: booking_id=%s err=%v",
			bookingID,
			payoutErr,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to finalize payment after delivery",
		})
		return
	}

	_, notificationErr := tx.Exec(
		c,
		`INSERT INTO notifications (
			user_id,
			booking_id,
			type,
			title,
			message
		)
		VALUES ($1, $2, $3, $4, $5)`,
		customerID,
		bookingID,
		"booking_status_delivered",
		"Move delivered",
		"Your MoveMate booking has been delivered after OTP verification.",
	)
	if notificationErr != nil {
		log.Printf(
			"failed to create delivery notification: booking_id=%s customer_id=%s err=%v",
			bookingID,
			customerID,
			notificationErr,
		)
	}

	if err := tx.Commit(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to complete delivery",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"booking": gin.H{
			"id":     bookingID,
			"status": updatedStatus,
		},
		"message": "delivery OTP verified and booking marked as delivered",
	})
}

type CreateBookingOfferRequest struct {
	VehicleID           string    `json:"vehicle_id"`
	Amount              float64   `json:"amount"`
	EstimatedDeliveryAt time.Time `json:"estimated_delivery_at"`
}

func (h *BookingHandler) CreateOffer(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can make booking offers",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	bookingID := c.Param("id")

	var req CreateBookingOfferRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.VehicleID = strings.TrimSpace(req.VehicleID)

	if req.VehicleID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "vehicle_id is required",
		})
		return
	}

	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "amount must be greater than 0",
		})
		return
	}

	if req.EstimatedDeliveryAt.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "estimated_delivery_at is required",
		})
		return
	}

	if !req.EstimatedDeliveryAt.After(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "estimated_delivery_at must be in the future",
		})
		return
	}

	var (
		offerID             string
		amount              float64
		status              string
		estimatedDeliveryAt time.Time
		vehicleType         string
		customerID          string
		driverName          string
	)

	var (
		pickupLat *float64
		pickupLng *float64
		driverLat float64
		driverLng float64
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			b.customer_id,
                        u.name,
                        b.pickup_lat,
                        b.pickup_lng,
                        dl.latitude,
                        dl.longitude
		FROM bookings b
		INNER JOIN users u
                        ON u.id = $2
                        AND u.role = 'driver'
                        AND u.is_active = TRUE
                INNER JOIN driver_presence dp
			ON dp.driver_id = $2
			AND dp.is_online = TRUE
		INNER JOIN driver_locations dl
			ON dl.driver_id = $2
			AND dl.updated_at >= NOW() - INTERVAL '2 minutes'
		WHERE b.id = $1
			AND b.status = 'requested'`,
		bookingID,
		userID,
	).Scan(
		&customerID,
		&driverName,
		&pickupLat,
		&pickupLng,
		&driverLat,
		&driverLng,
	)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "driver is offline, location is stale, or booking is unavailable",
		})
		return
	}

	if pickupLat == nil || pickupLng == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking pickup coordinates are required for nearby offers",
		})
		return
	}

	distanceKm := geo.HaversineKm(
		*pickupLat,
		*pickupLng,
		driverLat,
		driverLng,
	)

	if distanceKm > 10 {
		c.JSON(http.StatusConflict, gin.H{
			"success":     false,
			"error":       "driver is outside the 10 km booking radius",
			"distance_km": distanceKm,
		})
		return
	}

	err = h.DB.QueryRow(
		c,
		`INSERT INTO booking_offers (
				booking_id,
				driver_id,
				vehicle_id,
				amount,
				estimated_delivery_at
		)
		SELECT
				b.id,
				$1,
				v.id,
				$3,
				$4
		FROM bookings b
		JOIN vehicles v
				ON v.id = $2
				AND v.driver_id = $1
				AND v.vehicle_type = b.vehicle_type
				AND v.is_active = TRUE
		WHERE b.id = $5
				AND b.status = 'requested'
		RETURNING
				id,
				amount,
				status,
				estimated_delivery_at`,
		userID,
		req.VehicleID,
		req.Amount,
		req.EstimatedDeliveryAt,
		bookingID,
	).Scan(
		&offerID,
		&amount,
		&status,
		&estimatedDeliveryAt,
	)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking is unavailable, vehicle is invalid/inactive, or you already made an offer",
		})
		return
	}

	err = h.DB.QueryRow(
		c,
		`SELECT vehicle_type
		 FROM bookings
		 WHERE id = $1`,
		bookingID,
	).Scan(&vehicleType)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "offer created but failed to load booking details",
		})
		return
	}

	_, notificationErr := h.DB.Exec(
		c,
		`INSERT INTO notifications (
                        user_id,
                        booking_id,
                        type,
                        title,
                        message
                )
                VALUES ($1, $2, $3, $4, $5)`,
		customerID,
		bookingID,
		"offer_created",
		"New driver offer",
		driverName+" has offered ₹"+strconv.FormatFloat(amount, 'f', 2, 64)+" for your MoveMate booking.",
	)

	if notificationErr != nil {
		log.Printf(
			"failed to create offer notification: booking_id=%s customer_id=%s err=%v",
			bookingID,
			customerID,
			notificationErr,
		)
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "booking offer created successfully",
		"offer": gin.H{
			"id":                    offerID,
			"booking_id":            bookingID,
			"vehicle_type":          vehicleType,
			"vehicle_id":            req.VehicleID,
			"amount":                amount,
			"estimated_delivery_at": estimatedDeliveryAt,
			"status":                status,
		},
	})
}

func (h *BookingHandler) ListOffers(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "customer" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only customers can view booking offers",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	bookingID := c.Param("id")

	rows, err := h.DB.Query(
		c,
		`SELECT
			o.id,
			o.booking_id,
			o.driver_id,
			u.name,
			o.vehicle_id,
			v.vehicle_type,
			v.vehicle_number,
			v.model,
			o.amount,
			o.status,
				o.estimated_delivery_at,
			o.created_at
		FROM booking_offers o
		JOIN bookings b
			ON b.id = o.booking_id
		JOIN users u
			ON u.id = o.driver_id
		JOIN vehicles v
			ON v.id = o.vehicle_id
		WHERE o.booking_id = $1
			AND b.customer_id = $2
		ORDER BY o.amount ASC, o.created_at ASC`,
		bookingID,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load booking offers",
		})
		return
	}
	defer rows.Close()

	type Offer struct {
		ID                  string     `json:"id"`
		BookingID           string     `json:"booking_id"`
		DriverID            string     `json:"driver_id"`
		DriverName          string     `json:"driver_name"`
		VehicleID           string     `json:"vehicle_id"`
		VehicleType         string     `json:"vehicle_type"`
		VehicleNumber       string     `json:"vehicle_number"`
		VehicleModel        *string    `json:"vehicle_model"`
		Amount              float64    `json:"amount"`
		Status              string     `json:"status"`
		EstimatedDeliveryAt *time.Time `json:"estimated_delivery_at"`
		CreatedAt           time.Time  `json:"created_at"`
	}

	offers := make([]Offer, 0)

	for rows.Next() {
		var offer Offer

		if err := rows.Scan(
			&offer.ID,
			&offer.BookingID,
			&offer.DriverID,
			&offer.DriverName,
			&offer.VehicleID,
			&offer.VehicleType,
			&offer.VehicleNumber,
			&offer.VehicleModel,
			&offer.Amount,
			&offer.Status,
			&offer.EstimatedDeliveryAt,
			&offer.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read booking offers",
			})
			return
		}

		offers = append(offers, offer)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to read booking offers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"offers":  offers,
	})
}

func (h *BookingHandler) AcceptOffer(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "customer" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only customers can accept booking offers",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	bookingID := c.Param("id")
	offerID := c.Param("offer_id")

	tx, err := h.DB.Begin(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to start booking transaction",
		})
		return
	}
	defer tx.Rollback(c)

	var (
		offerDriverID         string
		offerVehicleID        string
		offerAmount           float64
		offerStatus           string
		offerDeliveryDeadline *time.Time
		bookingStatus         string
	)

	err = tx.QueryRow(
		c,
		`SELECT
			b.status,
			o.driver_id,
			o.vehicle_id,
			o.amount,
			o.status,
					o.estimated_delivery_at
		FROM bookings b
		JOIN booking_offers o
			ON o.booking_id = b.id
		WHERE b.id = $1
			AND o.id = $2
			AND b.customer_id = $3
		FOR UPDATE OF b, o`,
		bookingID,
		offerID,
		userID,
	).Scan(
		&bookingStatus,
		&offerDriverID,
		&offerVehicleID,
		&offerAmount,
		&offerStatus,
		&offerDeliveryDeadline,
	)

	if err != nil {
		log.Printf("AcceptOffer lookup failed: booking_id=%s offer_id=%s customer_id=%s err=%v",
			bookingID, offerID, userID, err)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking or offer not found",
		})
		return
	}

	if bookingStatus != "requested" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking is no longer available",
		})
		return
	}

	if offerStatus != "pending" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "offer is no longer available",
		})
		return
	}

	var vehicleType string

	err = tx.QueryRow(
		c,
		`SELECT vehicle_type
		FROM bookings
		WHERE id = $1`,
		bookingID,
	).Scan(&vehicleType)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load booking vehicle type",
		})
		return
	}

	var updatedBookingID string

	err = tx.QueryRow(
		c,
		`UPDATE bookings
		SET
			driver_id = $1,
			vehicle_id = $2,
			final_fare = $3,
			status = 'accepted',
				delivery_deadline_at = $4,
			updated_at = NOW()
		WHERE id = $5
			AND status = 'requested'
		RETURNING id`,
		offerDriverID,
		offerVehicleID,
		offerAmount,
		offerDeliveryDeadline,
		bookingID,
	).Scan(&updatedBookingID)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking could not be accepted",
		})
		return
	}

	_, err = tx.Exec(
		c,
		`UPDATE booking_offers
		SET
			status = 'accepted',
			updated_at = NOW()
		WHERE id = $1`,
		offerID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to accept offer",
		})
		return
	}

	_, err = tx.Exec(
		c,
		`UPDATE booking_offers
		SET
			status = 'rejected',
			updated_at = NOW()
		WHERE booking_id = $1
			AND id <> $2
			AND status = 'pending'`,
		bookingID,
		offerID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to reject other offers",
		})
		return
	}

	_, notificationErr := tx.Exec(
		c,
		`INSERT INTO notifications (
				user_id,
				booking_id,
				type,
				title,
				message
			)
			VALUES ($1, $2, $3, $4, $5)`,
		offerDriverID,
		bookingID,
		"offer_accepted",
		"Offer accepted",
		"Your ₹"+strconv.FormatFloat(offerAmount, 'f', 2, 64)+" offer has been accepted by the customer.",
	)

	if notificationErr != nil {
		log.Printf(
			"failed to create offer accepted notification: booking_id=%s driver_id=%s err=%v",
			bookingID,
			offerDriverID,
			notificationErr,
		)
	}

	if err := tx.Commit(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to complete offer acceptance",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "booking offer accepted successfully",
		"booking": gin.H{
			"id":           updatedBookingID,
			"driver_id":    offerDriverID,
			"vehicle_id":   offerVehicleID,
			"vehicle_type": vehicleType,
			"final_fare":   offerAmount,
			"status":       "accepted",
		},
		"offer": gin.H{
			"id":     offerID,
			"amount": offerAmount,
			"status": "accepted",
		},
	})
}

func (h *BookingHandler) ListDriverBookings(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can view assigned bookings",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	rows, err := h.DB.Query(
		c,
		`SELECT
			b.id,
			b.customer_id,
			u.name,
			b.pickup_address,
			b.pickup_lat,
			b.pickup_lng,
			b.drop_address,
			b.drop_lat,
			b.drop_lng,
			b.vehicle_type,
			b.item_description,
			b.item_quantity,
			b.estimated_weight_kg,
			b.final_fare,
			b.status,
			b.scheduled_at,
			b.created_at,
			b.updated_at
		FROM bookings b
		JOIN users u
			ON u.id = b.customer_id
		WHERE b.driver_id = $1
		ORDER BY
			CASE
				WHEN b.status IN ('accepted', 'driver_arriving', 'arrived', 'loading', 'in_transit')
				THEN 0
				ELSE 1
			END,
			COALESCE(b.scheduled_at, b.created_at) ASC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load assigned bookings",
		})
		return
	}
	defer rows.Close()

	type DriverBooking struct {
		ID                string     `json:"id"`
		CustomerID        string     `json:"customer_id"`
		CustomerName      string     `json:"customer_name"`
		PickupAddress     string     `json:"pickup_address"`
		PickupLat         *float64   `json:"pickup_lat"`
		PickupLng         *float64   `json:"pickup_lng"`
		DropAddress       string     `json:"drop_address"`
		DropLat           *float64   `json:"drop_lat"`
		DropLng           *float64   `json:"drop_lng"`
		VehicleType       string     `json:"vehicle_type"`
		ItemDescription   *string    `json:"item_description"`
		ItemQuantity      int        `json:"item_quantity"`
		EstimatedWeightKg *float64   `json:"estimated_weight_kg"`
		FinalFare         *float64   `json:"final_fare"`
		Status            string     `json:"status"`
		ScheduledAt       *time.Time `json:"scheduled_at"`
		CreatedAt         time.Time  `json:"created_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
	}

	bookings := make([]DriverBooking, 0)

	for rows.Next() {
		var booking DriverBooking

		if err := rows.Scan(
			&booking.ID,
			&booking.CustomerID,
			&booking.CustomerName,
			&booking.PickupAddress,
			&booking.PickupLat,
			&booking.PickupLng,
			&booking.DropAddress,
			&booking.DropLat,
			&booking.DropLng,
			&booking.VehicleType,
			&booking.ItemDescription,
			&booking.ItemQuantity,
			&booking.EstimatedWeightKg,
			&booking.FinalFare,
			&booking.Status,
			&booking.ScheduledAt,
			&booking.CreatedAt,
			&booking.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read assigned booking",
			})
			return
		}

		bookings = append(bookings, booking)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load assigned bookings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"bookings": bookings,
	})
}

type SendMessageRequest struct {
	Message     string `json:"message"`
	MessageType string `json:"message_type"`
}

func (h *BookingHandler) SendMessage(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")

	bookingID := c.Param("id")

	var req SendMessageRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	req.MessageType = strings.ToLower(strings.TrimSpace(req.MessageType))

	if req.MessageType == "" {
		req.MessageType = "text"
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "message is required",
		})
		return
	}

	if req.MessageType != "text" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "only text messages are currently supported",
		})
		return
	}

	var authorized bool

	if role == "customer" {
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND customer_id = $2
					AND driver_id IS NOT NULL
					AND status NOT IN ('cancelled')
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}
	} else if role == "driver" {
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND driver_id = $2
					AND status NOT IN ('cancelled')
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "chat is only available to customers and drivers",
		})
		return
	}

	if !authorized {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not have access to this booking chat",
		})
		return
	}

	_, err := h.DB.Exec(
		c,
		`INSERT INTO booking_chats (booking_id)
		VALUES ($1)
		ON CONFLICT (booking_id) DO NOTHING`,
		bookingID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create booking chat",
		})
		return
	}

	var (
		messageID string
		createdAt time.Time
	)

	err = h.DB.QueryRow(
		c,
		`INSERT INTO booking_messages (
			booking_id,
			sender_id,
			message,
			message_type
		)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		bookingID,
		userID,
		req.Message,
		req.MessageType,
	).Scan(
		&messageID,
		&createdAt,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to send message",
		})
		return
	}

	_, _ = h.DB.Exec(
		c,
		`UPDATE booking_chats
		SET updated_at = NOW()
		WHERE booking_id = $1`,
		bookingID,
	)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "message sent successfully",
		"chat_message": gin.H{
			"id":           messageID,
			"booking_id":   bookingID,
			"sender_id":    userID,
			"message":      req.Message,
			"message_type": req.MessageType,
			"created_at":   createdAt,
		},
	})
}

func (h *BookingHandler) ListMessages(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	bookingID := c.Param("id")

	var authorized bool

	switch role {
	case "customer":
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND customer_id = $2
					AND driver_id IS NOT NULL
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}

	case "driver":
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND driver_id = $2
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}

	default:
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "chat is only available to customers and drivers",
		})
		return
	}

	if !authorized {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not have access to this booking chat",
		})
		return
	}

	rows, err := h.DB.Query(
		c,
		`SELECT
			m.id,
			m.booking_id,
			m.sender_id,
			u.name,
			u.role,
			m.message,
			m.message_type,
			m.read_at,
			m.created_at
		FROM booking_messages m
		JOIN users u
			ON u.id = m.sender_id
		WHERE m.booking_id = $1
		ORDER BY m.created_at ASC`,
		bookingID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load chat messages",
		})
		return
	}
	defer rows.Close()

	type ChatMessage struct {
		ID          string     `json:"id"`
		BookingID   string     `json:"booking_id"`
		SenderID    string     `json:"sender_id"`
		SenderName  string     `json:"sender_name"`
		SenderRole  string     `json:"sender_role"`
		Message     string     `json:"message"`
		MessageType string     `json:"message_type"`
		ReadAt      *time.Time `json:"read_at"`
		CreatedAt   time.Time  `json:"created_at"`
	}

	messages := make([]ChatMessage, 0)

	for rows.Next() {
		var message ChatMessage

		if err := rows.Scan(
			&message.ID,
			&message.BookingID,
			&message.SenderID,
			&message.SenderName,
			&message.SenderRole,
			&message.Message,
			&message.MessageType,
			&message.ReadAt,
			&message.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read chat message",
			})
			return
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load chat messages",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"messages": messages,
	})
}

func (h *BookingHandler) MarkMessagesRead(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	bookingID := c.Param("id")

	var authorized bool

	switch role {
	case "customer":
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND customer_id = $2
					AND driver_id IS NOT NULL
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}

	case "driver":
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND driver_id = $2
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}

	default:
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "chat is only available to customers and drivers",
		})
		return
	}

	if !authorized {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not have access to this booking chat",
		})
		return
	}

	result, err := h.DB.Exec(
		c,
		`UPDATE booking_messages
		SET read_at = NOW()
		WHERE booking_id = $1
			AND sender_id <> $2
			AND read_at IS NULL`,
		bookingID,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to mark messages as read",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "messages marked as read",
		"messages_read": result.RowsAffected(),
	})
}

func (h *BookingHandler) UnreadMessageCount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	bookingID := c.Param("id")

	var authorized bool

	switch role {
	case "customer":
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND customer_id = $2
					AND driver_id IS NOT NULL
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}

	case "driver":
		err := h.DB.QueryRow(
			c,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND driver_id = $2
			)`,
			bookingID,
			userID,
		).Scan(&authorized)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to verify chat access",
			})
			return
		}

	default:
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "chat is only available to customers and drivers",
		})
		return
	}

	if !authorized {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not have access to this booking chat",
		})
		return
	}

	var unreadCount int

	err := h.DB.QueryRow(
		c,
		`SELECT COUNT(*)
		FROM booking_messages
		WHERE booking_id = $1
			AND sender_id <> $2
			AND read_at IS NULL`,
		bookingID,
		userID,
	).Scan(&unreadCount)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load unread message count",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"unread_count": unreadCount,
	})
}
