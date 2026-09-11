package payment

import "context"

type CreateOrderRequest struct {
	PaymentID string
	Amount    float64
	Currency  string
}

type Order struct {
	OrderID   string
	PaymentID string
	Amount    float64
	Currency  string
}

type VerifyPaymentRequest struct {
	OrderID          string
	GatewayPaymentID string
}

type VerifyWebhookRequest struct {
	Body      string
	Signature string
}

type Gateway interface {
	CreateOrder(ctx context.Context, req CreateOrderRequest) (Order, error)
	VerifyPayment(ctx context.Context, req VerifyPaymentRequest) error
	VerifyWebhook(ctx context.Context, req VerifyWebhookRequest) error
}
