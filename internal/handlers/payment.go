package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"movemate/internal/payment"
)

type PaymentHandler struct {
	DB      *pgxpool.Pool
	Gateway payment.Gateway
}

type CreatePaymentRequest struct {
	Method string `json:"method"`
}

func (h *PaymentHandler) createGatewayOrder(
	c *gin.Context,
	paymentID string,
	amount float64,
	currency string,
) (payment.Order, error) {
	if h.Gateway == nil {
		return payment.Order{}, payment.ErrGatewayUnavailable
	}

	return h.Gateway.CreateOrder(
		c,
		payment.CreateOrderRequest{
			PaymentID: paymentID,
			Amount:    amount,
			Currency:  currency,
		},
	)
}

func (h *PaymentHandler) Create(c *gin.Context) {
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
			"error":   "only customers can create payments",
		})
		return
	}

	bookingID := c.Param("id")

	var req CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Method = strings.ToLower(strings.TrimSpace(req.Method))

	switch req.Method {
	case "cash", "upi", "card", "wallet", "other":
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid payment method",
		})
		return
	}

	var (
		customerID string
		driverID   *string
		finalFare  *float64
		status     string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT customer_id, driver_id, final_fare, status
		 FROM bookings
		 WHERE id = $1`,
		bookingID,
	).Scan(&customerID, &driverID, &finalFare, &status)

	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "booking not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load booking",
		})
		return
	}

	if customerID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not own this booking",
		})
		return
	}

	if status != "accepted" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "payment can only be created after a booking offer is accepted",
		})
		return
	}

	if finalFare == nil || *finalFare <= 0 {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "booking does not have a valid final fare",
		})
		return
	}

	var (
		paymentID         string
		paymentBookingID  string
		paymentCustomerID string
		paymentDriverID   *string
		amount            float64
		currency          string
		method            string
		paymentStatus     string
		gateway           *string
		gatewayPaymentID  *string
		gatewayOrderID    *string
		paidAt            interface{}
		createdAt         interface{}
		updatedAt         interface{}
	)

	err = h.DB.QueryRow(
		c,
		`INSERT INTO payments (
			booking_id,
			customer_id,
			driver_id,
			amount,
			currency,
			method,
                        payment_type,
			status
		)
                VALUES (
                        $1, $2, $3, $4, 'INR', $5,
                        CASE WHEN $5::varchar = 'cash' THEN 'cash' ELSE 'in_app' END,
                        'pending'
                )
		RETURNING
			id,
			booking_id,
			customer_id,
			driver_id,
			amount::float8,
			currency,
			method,
			status,
			gateway,
			gateway_payment_id,
			gateway_order_id,
			paid_at,
			created_at,
			updated_at`,
		bookingID,
		customerID,
		driverID,
		*finalFare,
		req.Method,
	).Scan(
		&paymentID,
		&paymentBookingID,
		&paymentCustomerID,
		&paymentDriverID,
		&amount,
		&currency,
		&method,
		&paymentStatus,
		&gateway,
		&gatewayPaymentID,
		&gatewayOrderID,
		&paidAt,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "payments_booking_id_key") {
			var existingPaymentStatus string

			statusErr := h.DB.QueryRow(
				c,
				`SELECT status
             FROM payments
             WHERE booking_id = $1`,
				bookingID,
			).Scan(&existingPaymentStatus)

			if statusErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   "failed to load existing payment",
				})
				return
			}

			if existingPaymentStatus != "failed" {
				c.JSON(http.StatusConflict, gin.H{
					"success": false,
					"error":   "payment already exists for this booking",
				})
				return
			}

			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "failed payment retry is not implemented yet",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create payment",
		})
		return
	}

	if method != "cash" {
		if h.Gateway == nil {
			_, _ = h.DB.Exec(c,
				`UPDATE payments
				 SET status = 'failed',
				     updated_at = NOW()
				 WHERE id = $1`,
				paymentID,
			)

			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error":   "payment gateway is not configured",
			})
			return
		}

		order, gatewayErr := h.Gateway.CreateOrder(
			c,
			payment.CreateOrderRequest{
				PaymentID: paymentID,
				Amount:    amount,
				Currency:  currency,
			},
		)

		if gatewayErr != nil {
			_, _ = h.DB.Exec(c,
				`UPDATE payments
				 SET status = 'failed',
				     updated_at = NOW()
				 WHERE id = $1`,
				paymentID,
			)

			c.JSON(http.StatusBadGateway, gin.H{
				"success": false,
				"error":   "failed to create payment gateway order",
			})
			return
		}

		gatewayName := "configured_gateway"

		_, gatewayErr = h.DB.Exec(
			c,
			`UPDATE payments
			 SET gateway = $2,
			     gateway_order_id = $3,
			     status = 'processing',
			     updated_at = NOW()
			 WHERE id = $1`,
			paymentID,
			gatewayName,
			order.OrderID,
		)

		if gatewayErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to save payment gateway order",
			})
			return
		}

		gateway = &gatewayName
		gatewayOrderID = &order.OrderID
		paymentStatus = "processing"
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "payment created successfully",
		"payment": gin.H{
			"id":          paymentID,
			"booking_id":  paymentBookingID,
			"customer_id": paymentCustomerID,
			"driver_id":   paymentDriverID,
			"amount":      amount,
			"currency":    currency,
			"method":      method,
			"status":      paymentStatus,
			"payment_type": func() string {
				if method == "cash" {
					return "cash"
				}
				return "in_app"
			}(),
			"payout_status": func() string {
				if method == "cash" {
					return "not_applicable"
				}
				return "pending"
			}(),
			"gateway":             gateway,
			"gateway_payment_id":  gatewayPaymentID,
			"gateway_order_id":    gatewayOrderID,
			"paid_at":             paidAt,
			"held_at":             nil,
			"released_at":         nil,
			"refund_eligible_at":  nil,
			"refund_requested_at": nil,
			"refund_reason":       nil,
			"created_at":          createdAt,
			"updated_at":          updatedAt,
		},
	})
}

func (h *PaymentHandler) Retry(c *gin.Context) {
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
			"error":   "only customers can retry payments",
		})
		return
	}

	bookingID := c.Param("id")

	var (
		paymentID  string
		customerID string
		amount     float64
		currency   string
		method     string
		status     string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			id,
			customer_id,
			amount::float8,
			currency,
			method,
			status
		 FROM payments
		 WHERE booking_id = $1`,
		bookingID,
	).Scan(
		&paymentID,
		&customerID,
		&amount,
		&currency,
		&method,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "payment not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load payment",
		})
		return
	}

	if customerID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not own this payment",
		})
		return
	}

	if method == "cash" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "cash payments cannot be retried through the gateway",
		})
		return
	}

	if status != "failed" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "only failed payments can be retried",
		})
		return
	}

	order, gatewayErr := h.createGatewayOrder(
		c,
		paymentID,
		amount,
		currency,
	)

	if gatewayErr != nil {
		_, _ = h.DB.Exec(
			c,
			`UPDATE payments
			 SET status = 'failed',
			     gateway = NULL,
			     gateway_order_id = NULL,
			     gateway_payment_id = NULL,
			     updated_at = NOW()
			 WHERE id = $1`,
			paymentID,
		)

		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "failed to create payment gateway order",
		})
		return
	}

	gatewayName := "configured_gateway"

	_, gatewayErr = h.DB.Exec(
		c,
		`UPDATE payments
		 SET gateway = $2,
		     gateway_order_id = $3,
		     status = 'processing',
		     updated_at = NOW()
		 WHERE id = $1`,
		paymentID,
		gatewayName,
		order.OrderID,
	)

	if gatewayErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to save payment gateway order",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "payment retry initiated successfully",
		"payment": gin.H{
			"id":               paymentID,
			"booking_id":       bookingID,
			"amount":           amount,
			"currency":         currency,
			"method":           method,
			"status":           "processing",
			"payment_type":     "in_app",
			"gateway":          gatewayName,
			"gateway_order_id": order.OrderID,
		},
	})
}

func (h *PaymentHandler) RazorpayWebhook(c *gin.Context) {
	signature := strings.TrimSpace(c.GetHeader("X-Razorpay-Signature"))
	eventID := strings.TrimSpace(c.GetHeader("x-razorpay-event-id"))

	if h.Gateway == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "payment gateway is not configured",
		})
		return
	}

	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "missing Razorpay webhook signature",
		})
		return
	}

	if eventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "missing Razorpay webhook event id",
		})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "failed to read webhook body",
		})
		return
	}

	if err := h.Gateway.VerifyWebhook(
		c,
		payment.VerifyWebhookRequest{
			Body:      string(body),
			Signature: signature,
		},
	); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "invalid webhook signature",
		})
		return
	}

	var event struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID       string `json:"id"`
					OrderID  string `json:"order_id"`
					Amount   int64  `json:"amount"`
					Currency string `json:"currency"`
					Status   string `json:"status"`
				} `json:"entity"`
			} `json:"payment"`
			Order struct {
				Entity struct {
					ID       string `json:"id"`
					Amount   int64  `json:"amount"`
					Currency string `json:"currency"`
					Status   string `json:"status"`
				} `json:"entity"`
			} `json:"order"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid webhook payload",
		})
		return
	}

	if event.Event != "payment.captured" && event.Event != "order.paid" {
		_, err := h.DB.Exec(
			c,
			`INSERT INTO payment_webhook_events (
				event_id,
				event_type,
				processed_at
			)
			VALUES ($1, $2, NOW())
			ON CONFLICT (event_id) DO NOTHING`,
			eventID,
			event.Event,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to record webhook event",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "webhook event ignored",
		})
		return
	}

	var gatewayOrderID string
	var gatewayPaymentID string
	var gatewayAmount int64
	var gatewayCurrency string

	if event.Event == "payment.captured" {
		gatewayPaymentID = event.Payload.Payment.Entity.ID
		gatewayOrderID = event.Payload.Payment.Entity.OrderID
		gatewayAmount = event.Payload.Payment.Entity.Amount
		gatewayCurrency = event.Payload.Payment.Entity.Currency
	} else {
		gatewayOrderID = event.Payload.Order.Entity.ID
		gatewayAmount = event.Payload.Order.Entity.Amount
		gatewayCurrency = event.Payload.Order.Entity.Currency
	}

	if gatewayOrderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "webhook is missing Razorpay order id",
		})
		return
	}

	tx, err := h.DB.Begin(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to start webhook transaction",
		})
		return
	}
	defer tx.Rollback(c)

	var (
		paymentID        string
		expectedAmount   float64
		expectedCurrency string
		paymentStatus    string
		paymentType      string
		deliveryDeadline *time.Time
	)

	err = tx.QueryRow(
		c,
		`SELECT
			p.id,
			p.amount::float8,
			p.currency,
			p.status,
			p.payment_type,
			b.delivery_deadline_at
		 FROM payments p
		 LEFT JOIN bookings b ON b.id = p.booking_id
		 WHERE p.gateway_order_id = $1`,
		gatewayOrderID,
	).Scan(
		&paymentID,
		&expectedAmount,
		&expectedCurrency,
		&paymentStatus,
		&paymentType,
		&deliveryDeadline,
	)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "payment for Razorpay order not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load payment",
		})
		return
	}

	_, err = tx.Exec(
		c,
		`SELECT id FROM payments WHERE id = $1 FOR UPDATE`,
		paymentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to lock payment",
		})
		return
	}

	if paymentType != "in_app" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "webhook payment is not an in-app payment",
		})
		return
	}

	expectedPaise := int64(expectedAmount*100 + 0.5)

	if gatewayAmount != expectedPaise || !strings.EqualFold(gatewayCurrency, expectedCurrency) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "webhook payment amount or currency mismatch",
		})
		return
	}

	var existingEventID string
	err = tx.QueryRow(
		c,
		`SELECT event_id
		 FROM payment_webhook_events
		 WHERE event_id = $1
		 FOR UPDATE`,
		eventID,
	).Scan(&existingEventID)

	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "duplicate webhook event ignored",
		})
		return
	}

	if err != pgx.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to check webhook event",
		})
		return
	}

	_, err = tx.Exec(
		c,
		`INSERT INTO payment_webhook_events (
			event_id,
			event_type,
			payment_id
		)
		VALUES ($1, $2, $3)`,
		eventID,
		event.Event,
		paymentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to record webhook event",
		})
		return
	}

	if paymentStatus == "paid" || paymentStatus == "refunded" {
		_, err = tx.Exec(
			c,
			`UPDATE payment_webhook_events
			 SET processed_at = NOW()
			 WHERE event_id = $1`,
			eventID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to finalize webhook event",
			})
			return
		}

		if err := tx.Commit(c); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to commit webhook event",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "payment webhook already processed",
		})
		return
	}

	if paymentStatus != "processing" && paymentStatus != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "payment is not in a payable state",
		})
		return
	}

	var refundEligibleAt *time.Time
	if deliveryDeadline != nil {
		t := deliveryDeadline.Add(3 * time.Hour)
		refundEligibleAt = &t
	}

	if gatewayPaymentID == "" && event.Event == "order.paid" {
		gatewayPaymentID = ""
	}

	_, err = tx.Exec(
		c,
		`UPDATE payments
		 SET
			status = 'paid',
			gateway_payment_id = NULLIF($2, ''),
			paid_at = NOW(),
			held_at = NOW(),
			refund_eligible_at = $3,
			payout_status = 'held',
			updated_at = NOW()
		 WHERE id = $1`,
		paymentID,
		gatewayPaymentID,
		refundEligibleAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to update payment",
		})
		return
	}

	_, err = tx.Exec(
		c,
		`UPDATE payment_webhook_events
		 SET processed_at = NOW()
		 WHERE event_id = $1`,
		eventID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to finalize webhook event",
		})
		return
	}

	if err := tx.Commit(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to commit webhook transaction",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "payment captured and held successfully",
	})
}

type RefundRequest struct {
	Reason string `json:"reason"`
}

func (h *PaymentHandler) RequestRefund(c *gin.Context) {
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
			"error":   "only customers can request refunds",
		})
		return
	}

	bookingID := c.Param("id")

	var req RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "refund reason is too long",
		})
		return
	}

	tx, err := h.DB.Begin(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to start refund request",
		})
		return
	}
	defer tx.Rollback(c)

	var (
		paymentID         string
		customerID        string
		amount            float64
		status            string
		paymentType       string
		payoutStatus      string
		refundEligibleAt  *time.Time
		refundRequestedAt *time.Time
	)

	err = tx.QueryRow(
		c,
		`SELECT
			id,
			customer_id,
			amount::float8,
			status,
			payment_type,
			payout_status,
			refund_eligible_at,
			refund_requested_at
		 FROM payments
		 WHERE booking_id = $1
		 FOR UPDATE`,
		bookingID,
	).Scan(
		&paymentID,
		&customerID,
		&amount,
		&status,
		&paymentType,
		&payoutStatus,
		&refundEligibleAt,
		&refundRequestedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "payment not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load payment",
		})
		return
	}

	if customerID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "you do not own this payment",
		})
		return
	}

	if paymentType != "in_app" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "cash payments are not refundable through MoveMate",
		})
		return
	}

	if status != "paid" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "payment is not eligible for a refund request",
		})
		return
	}

	if payoutStatus != "held" {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "payment is no longer held and cannot be refunded through this flow",
		})
		return
	}

	if refundRequestedAt != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success":             false,
			"error":               "refund has already been requested",
			"refund_requested_at": refundRequestedAt,
		})
		return
	}

	if refundEligibleAt == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "refund eligibility deadline is not available",
		})
		return
	}

	if time.Now().Before(*refundEligibleAt) {
		c.JSON(http.StatusConflict, gin.H{
			"success":            false,
			"error":              "refund is not yet eligible",
			"refund_eligible_at": refundEligibleAt,
		})
		return
	}

	var reason *string
	if req.Reason != "" {
		reason = &req.Reason
	}

	var requestedAt time.Time
	err = tx.QueryRow(
		c,
		`UPDATE payments
		 SET refund_requested_at = NOW(),
		     refund_reason = $2,
		     updated_at = NOW()
		 WHERE id = $1
		   AND refund_requested_at IS NULL
		   AND status = 'paid'
		   AND payment_type = 'in_app'
		   AND payout_status = 'held'
		 RETURNING refund_requested_at`,
		paymentID,
		reason,
	).Scan(&requestedAt)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "refund request could not be created",
		})
		return
	}

	if err := tx.Commit(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to save refund request",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "refund request submitted",
		"refund": gin.H{
			"booking_id":          bookingID,
			"payment_id":          paymentID,
			"amount":              amount,
			"status":              "requested",
			"refund_requested_at": requestedAt,
			"refund_reason":       req.Reason,
		},
	})
}

func (h *PaymentHandler) Get(c *gin.Context) {
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
		paymentID         string
		paymentBookingID  string
		customerID        string
		driverID          *string
		amount            float64
		currency          string
		method            string
		status            string
		gateway           *string
		gatewayPaymentID  *string
		gatewayOrderID    *string
		paidAt            interface{}
		refundedAt        interface{}
		createdAt         interface{}
		updatedAt         interface{}
		paymentType       string
		heldAt            interface{}
		releasedAt        interface{}
		refundEligibleAt  interface{}
		refundRequestedAt interface{}
		refundReason      *string
		payoutStatus      string
	)

	err := h.DB.QueryRow(
		c,
		`SELECT
			id,
			booking_id,
			customer_id,
			driver_id,
			amount::float8,
			currency,
			method,
			status,
			gateway,
			gateway_payment_id,
			gateway_order_id,
			paid_at,
			refunded_at,
			created_at,
			updated_at,
                        payment_type,
                        held_at,
                        released_at,
                        refund_eligible_at,
                        refund_requested_at,
                        refund_reason,
                        payout_status
		 FROM payments
		 WHERE booking_id = $1`,
		bookingID,
	).Scan(
		&paymentID,
		&paymentBookingID,
		&customerID,
		&driverID,
		&amount,
		&currency,
		&method,
		&status,
		&gateway,
		&gatewayPaymentID,
		&gatewayOrderID,
		&paidAt,
		&refundedAt,
		&createdAt,
		&updatedAt,
		&paymentType,
		&heldAt,
		&releasedAt,
		&refundEligibleAt,
		&refundRequestedAt,
		&refundReason,
		&payoutStatus,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "payment not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load payment",
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
			"error":   "you do not have access to this payment",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"payment": gin.H{
			"id":                  paymentID,
			"booking_id":          paymentBookingID,
			"customer_id":         customerID,
			"driver_id":           driverID,
			"amount":              amount,
			"currency":            currency,
			"method":              method,
			"status":              status,
			"gateway":             gateway,
			"gateway_payment_id":  gatewayPaymentID,
			"gateway_order_id":    gatewayOrderID,
			"paid_at":             paidAt,
			"refunded_at":         refundedAt,
			"created_at":          createdAt,
			"updated_at":          updatedAt,
			"payment_type":        paymentType,
			"held_at":             heldAt,
			"released_at":         releasedAt,
			"refund_eligible_at":  refundEligibleAt,
			"refund_requested_at": refundRequestedAt,
			"refund_reason":       refundReason,
			"payout_status":       payoutStatus,
		},
	})
}
