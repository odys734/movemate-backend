package payment

import (
	"context"
	"errors"
)

var ErrGatewayUnavailable = errors.New("payment gateway is not configured")

type UnavailableGateway struct{}

func (UnavailableGateway) CreateOrder(ctx context.Context, req CreateOrderRequest) (Order, error) {
	return Order{}, ErrGatewayUnavailable
}

func (UnavailableGateway) VerifyPayment(ctx context.Context, req VerifyPaymentRequest) error {
	return ErrGatewayUnavailable
}

func (UnavailableGateway) VerifyWebhook(
	ctx context.Context,
	req VerifyWebhookRequest,
) error {
	return ErrGatewayUnavailable
}
