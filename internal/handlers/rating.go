package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RatingHandler struct {
	DB *pgxpool.Pool
}

type CreateRatingRequest struct {
	Rating int    `json:"rating"`
	Review string `json:"review"`
}

func (h *RatingHandler) Create(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "customer" && role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only customers and drivers can submit ratings",
		})
		return
	}

	bookingID := c.Param("id")

	var req CreateRatingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Review = strings.TrimSpace(req.Review)

	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "rating must be between 1 and 5",
		})
		return
	}

	if len(req.Review) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "review must be 1000 characters or less",
		})
		return
	}

	var (
		customerID string
		driverID   string
		status     string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			customer_id,
			driver_id,
			status
		 FROM bookings
		 WHERE id = $1`,
		bookingID,
	).Scan(
		&customerID,
		&driverID,
		&status,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking not found",
		})
		return
	}

	if status != "delivered" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking must be delivered before rating",
		})
		return
	}

	if driverID == "" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking has no assigned driver",
		})
		return
	}

	var ratedUserID string

	switch role {
	case "customer":
		if customerID != userID {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "you do not have access to this booking",
			})
			return
		}

		ratedUserID = driverID

	case "driver":
		if driverID != userID {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "you do not have access to this booking",
			})
			return
		}

		ratedUserID = customerID
	}

	if ratedUserID == userID {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "you cannot rate yourself",
		})
		return
	}

	var ratingID string

	err = h.DB.QueryRow(
		c,
		`INSERT INTO booking_ratings (
			booking_id,
			rater_id,
			rated_user_id,
			rating,
			review
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		RETURNING id`,
		bookingID,
		userID,
		ratedUserID,
		req.Rating,
		req.Review,
	).Scan(&ratingID)

	if err != nil {
		if strings.Contains(err.Error(), "booking_ratings_booking_id_rater_id_key") {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "you have already rated this booking",
			})
			return
		}

		log.Printf(
			"failed to create rating: booking_id=%s rater_id=%s err=%v",
			bookingID,
			userID,
			err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create rating",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "rating submitted successfully",
		"rating": gin.H{
			"id":            ratingID,
			"booking_id":    bookingID,
			"rater_id":      userID,
			"rated_user_id": ratedUserID,
			"rating":        req.Rating,
			"review":        req.Review,
		},
	})
}

func (h *RatingHandler) ListBookingRatings(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	bookingID := c.Param("id")

	var (
		customerID string
		driverID   *string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT customer_id, driver_id
		 FROM bookings
		 WHERE id = $1`,
		bookingID,
	).Scan(&customerID, &driverID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking not found",
		})
		return
	}

	role, _ := c.Get("role")

	allowed := false

	if role == "customer" && customerID == userID {
		allowed = true
	}

	if role == "driver" && driverID != nil && *driverID == userID {
		allowed = true
	}

	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not have access to this booking",
		})
		return
	}

	rows, err := h.DB.Query(
		c,
		`SELECT
			r.id,
			r.rater_id,
			u.name,
			r.rated_user_id,
			r.rating,
			r.review,
			r.created_at
		 FROM booking_ratings r
		 JOIN users u ON u.id = r.rater_id
		 WHERE r.booking_id = $1
		 ORDER BY r.created_at ASC`,
		bookingID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load booking ratings",
		})
		return
	}
	defer rows.Close()

	ratings := make([]gin.H, 0)

	for rows.Next() {
		var (
			id          string
			raterID     string
			raterName   string
			ratedUserID string
			rating      int
			review      *string
			createdAt   time.Time
		)

		if err := rows.Scan(
			&id,
			&raterID,
			&raterName,
			&ratedUserID,
			&rating,
			&review,
			&createdAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read booking rating",
			})
			return
		}

		ratings = append(ratings, gin.H{
			"id":            id,
			"rater_id":      raterID,
			"rater_name":    raterName,
			"rated_user_id": ratedUserID,
			"rating":        rating,
			"review":        review,
			"created_at":    createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load booking ratings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"booking_id": bookingID,
		"ratings":    ratings,
	})
}

func (h *RatingHandler) UserRating(c *gin.Context) {
	userID := c.Param("id")

	var (
		name        string
		average     *float64
		ratingCount int
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			u.name,
			AVG(r.rating)::float8,
			COUNT(r.id)
		 FROM users u
		 LEFT JOIN booking_ratings r
		   ON r.rated_user_id = u.id
		 WHERE u.id = $1
		 GROUP BY u.id, u.name`,
		userID,
	).Scan(
		&name,
		&average,
		&ratingCount,
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
			"id":             userID,
			"name":           name,
			"average_rating": average,
			"rating_count":   ratingCount,
		},
	})
}
