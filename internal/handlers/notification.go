package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationHandler struct {
	DB *pgxpool.Pool
}

type Notification struct {
	ID        string     `json:"id"`
	BookingID *string    `json:"booking_id,omitempty"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (h *NotificationHandler) List(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Unauthorized",
		})
		return
	}

	limit := 50
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "limit must be between 1 and 100",
			})
			return
		}
		limit = parsed
	}

	rows, err := h.DB.Query(
		c.Request.Context(),
		`SELECT
			id,
			booking_id,
			type,
			title,
			message,
			read_at,
			created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2`,
		userID,
		limit,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to load notifications",
		})
		return
	}
	defer rows.Close()

	notifications := make([]Notification, 0)

	for rows.Next() {
		var n Notification

		if err := rows.Scan(
			&n.ID,
			&n.BookingID,
			&n.Type,
			&n.Title,
			&n.Message,
			&n.ReadAt,
			&n.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to read notifications",
			})
			return
		}

		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to read notifications",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"notifications": notifications,
	})
}

func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Unauthorized",
		})
		return
	}

	var count int

	err := h.DB.QueryRow(
		c.Request.Context(),
		`SELECT COUNT(*)
		 FROM notifications
		 WHERE user_id = $1
		   AND read_at IS NULL`,
		userID,
	).Scan(&count)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to load unread notification count",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   count,
	})
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Unauthorized",
		})
		return
	}

	notificationID := c.Param("id")

	var readAt time.Time

	err := h.DB.QueryRow(
		c.Request.Context(),
		`UPDATE notifications
		 SET read_at = COALESCE(read_at, NOW())
		 WHERE id = $1
		   AND user_id = $2
		 RETURNING read_at`,
		notificationID,
		userID,
	).Scan(&readAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Notification not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"read_at": readAt,
	})
}
