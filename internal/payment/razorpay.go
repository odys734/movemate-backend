package payment

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/razorpay/razorpay-go"
	utils "github.com/razorpay/razorpay-go/utils"
)

type RazorpayGateway struct {
	Client        *razorpay.Client
	WebhookSecret string
}

func NewRazorpayGateway(keyID, keySecret, webhookSecret string) *RazorpayGateway {
	keyID = strings.TrimSpace(keyID)
	keySecret = strings.TrimSpace(keySecret)
	webhookSecret = strings.TrimSpace(webhookSecret)

	if keyID == "" || keySecret == "" {
		return &RazorpayGateway{}
	}

	return &RazorpayGateway{
		Client:        razorpay.NewClient(keyID, keySecret),
		WebhookSecret: webhookSecret,
	}
}

func (g *RazorpayGateway) CreateOrder(
	ctx context.Context,
	req CreateOrderRequest,
) (Order, error) {
	if g == nil || g.Client == nil {
		return Order{}, ErrGatewayUnavailable
	}

	if req.PaymentID == "" || req.Amount <= 0 || req.Currency == "" {
		return Order{}, fmt.Errorf("invalid payment order request")
	}

	amountPaise := int64(math.Round(req.Amount * 100))

	data := map[string]interface{}{
		"amount":   amountPaise,
		"currency": req.Currency,
		"receipt":  req.PaymentID,
	}

	result, err := g.Client.Order.Create(data, nil)
	if err != nil {
		return Order{}, fmt.Errorf("razorpay order creation failed: %w", err)
	}

	orderID, ok := result["id"].(string)
	if !ok || orderID == "" {
		return Order{}, fmt.Errorf("razorpay returned an invalid order id")
	}

	return Order{
		OrderID:   orderID,
		PaymentID: req.PaymentID,
		Amount:    req.Amount,
		Currency:  req.Currency,
	}, nil
}

func (g *RazorpayGateway) VerifyPayment(
	ctx context.Context,
	req VerifyPaymentRequest,
) error {
	if g == nil || g.Client == nil {
		return ErrGatewayUnavailable
	}

	if req.OrderID == "" || req.GatewayPaymentID == "" {
		return fmt.Errorf("invalid payment verification request")
	}

	// Verification will be implemented using server-side
	// signature/webhook validation in the next milestone.
	return fmt.Errorf("payment verification is not implemented")
}

func (g *RazorpayGateway) VerifyWebhook(
	ctx context.Context,
	req VerifyWebhookRequest,
) error {
	if g == nil {
		return ErrGatewayUnavailable
	}

	if req.Body == "" || req.Signature == "" {
		return fmt.Errorf("invalid webhook verification request")
	}

	if g.WebhookSecret == "" {
		return ErrGatewayUnavailable
	}

	if !utils.VerifyWebhookSignature(
		req.Body,
		req.Signature,
		g.WebhookSecret,
	) {
		return fmt.Errorf("invalid webhook signature")
	}

	return nil
}
