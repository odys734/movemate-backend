package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"movemate/internal/auth"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type ChatHandler struct {
	Hub       *Hub
	JWTSecret string
	DB        *pgxpool.Pool
}

type incomingMessage struct {
	Message string `json:"message"`
}

type outgoingMessage struct {
	ID          string    `json:"id"`
	BookingID   string    `json:"booking_id"`
	SenderID    string    `json:"sender_id"`
	Message     string    `json:"message"`
	MessageType string    `json:"message_type"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *ChatHandler) Connect(c *gin.Context) {
	tokenString := c.Query("token")
	bookingID := c.Param("id")

	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication token required",
		})
		return
	}

	claims, err := auth.ParseToken(tokenString, h.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "invalid authentication token",
		})
		return
	}

	if claims.Role != "customer" && claims.Role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "chat is only available to customers and drivers",
		})
		return
	}

	if h.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "database is not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var authorized bool

	switch claims.Role {
	case "customer":
		err = h.DB.QueryRow(
			ctx,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND customer_id = $2
					AND driver_id IS NOT NULL
					AND status <> 'cancelled'
			)`,
			bookingID,
			claims.UserID,
		).Scan(&authorized)

	case "driver":
		err = h.DB.QueryRow(
			ctx,
			`SELECT EXISTS (
				SELECT 1
				FROM bookings
				WHERE id = $1
					AND driver_id = $2
					AND status <> 'cancelled'
			)`,
			bookingID,
			claims.UserID,
		).Scan(&authorized)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to verify booking access",
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

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &Client{
		UserID:    claims.UserID,
		BookingID: bookingID,
		Conn:      conn,
		Send:      make(chan []byte, 16),
	}

	h.Hub.AddClient(client)

	defer func() {
		h.Hub.RemoveClient(client)
		conn.Close()
	}()

	go h.writePump(client)

	h.readPump(client)
}

func (h *ChatHandler) readPump(client *Client) {
	defer func() {
		client.Conn.Close()
	}()

	for {
		_, data, err := client.Conn.ReadMessage()
		if err != nil {
			return
		}

		var input incomingMessage

		if err := json.Unmarshal(data, &input); err != nil {
			h.sendError(client, "invalid message format")
			continue
		}

		message := strings.TrimSpace(input.Message)

		if message == "" {
			h.sendError(client, "message cannot be empty")
			continue
		}

		if len(message) > 5000 {
			h.sendError(client, "message is too long")
			continue
		}

		h.saveAndBroadcast(client, message)
	}
}

func (h *ChatHandler) writePump(client *Client) {
	for message := range client.Send {
		if err := client.Conn.WriteMessage(
			websocket.TextMessage,
			message,
		); err != nil {
			return
		}
	}
}

func (h *ChatHandler) saveAndBroadcast(client *Client, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var outgoing outgoingMessage

	err := h.DB.QueryRow(
		ctx,
		`INSERT INTO booking_messages (
			booking_id,
			sender_id,
			message,
			message_type
		)
		VALUES ($1, $2, $3, 'text')
		RETURNING
			id,
			booking_id,
			sender_id,
			message,
			message_type,
			created_at`,
		client.BookingID,
		client.UserID,
		message,
	).Scan(
		&outgoing.ID,
		&outgoing.BookingID,
		&outgoing.SenderID,
		&outgoing.Message,
		&outgoing.MessageType,
		&outgoing.CreatedAt,
	)

	if err != nil {
		h.sendError(client, "failed to save message")
		return
	}

	payload, err := json.Marshal(outgoing)
	if err != nil {
		h.sendError(client, "failed to encode message")
		return
	}

	h.Hub.Broadcast(client.BookingID, payload, client)
}

func (h *ChatHandler) sendError(client *Client, message string) {
	payload, err := json.Marshal(gin.H{
		"success": false,
		"error":   message,
	})
	if err != nil {
		return
	}

	select {
	case client.Send <- payload:
	default:
	}
}
