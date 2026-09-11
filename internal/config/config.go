package config

import "os"

type Config struct {
	DatabaseURL           string
	JWTSecret             string
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
}

func Load() Config {
	return Config{
		DatabaseURL: getEnv(
			"DATABASE_URL",
			"postgres://u0_a193@127.0.0.1:5433/movemate",
		),
		JWTSecret: getEnv(
			"JWT_SECRET",
			"change-this-development-secret",
		),
		RazorpayKeyID: getEnv(
			"RAZORPAY_KEY_ID",
			"",
		),
		RazorpayKeySecret: getEnv(
			"RAZORPAY_KEY_SECRET",
			"",
		),
		RazorpayWebhookSecret: getEnv(
			"RAZORPAY_WEBHOOK_SECRET",
			"",
		),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
