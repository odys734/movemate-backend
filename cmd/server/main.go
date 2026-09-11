package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"movemate/internal/config"
	"movemate/internal/database"
	"movemate/internal/handlers"
	"movemate/internal/middleware"
	"movemate/internal/payment"
	"movemate/internal/realtime"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("database connection failed: ", err)
	}
	defer db.Close()

	log.Println("MoveMate database connected")

	router := gin.Default()

	// CORS for MoveMate frontend
	router.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if origin == "http://127.0.0.1:5173" ||
			origin == "http://localhost:5173" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	authHandler := &handlers.AuthHandler{
		DB:  db,
		Cfg: cfg,
	}

	vehicleHandler := &handlers.VehicleHandler{
		DB: db,
	}

	bookingHandler := &handlers.BookingHandler{
		DB: db,
	}

	ratingHandler := &handlers.RatingHandler{
		DB: db,
	}

	gateway := payment.Gateway(payment.UnavailableGateway{})

	if cfg.RazorpayKeyID != "" && cfg.RazorpayKeySecret != "" {
		gateway = payment.NewRazorpayGateway(
			cfg.RazorpayKeyID,
			cfg.RazorpayKeySecret,
			cfg.RazorpayWebhookSecret,
		)
	}

	paymentHandler := &handlers.PaymentHandler{
		DB:      db,
		Gateway: gateway,
	}

	presenceHandler := &handlers.PresenceHandler{
		DB: db,
	}

	locationHandler := &handlers.LocationHandler{
		DB: db,
	}

	nearbyDriversHandler := &handlers.NearbyDriversHandler{
		DB: db,
	}

	notificationHandler := &handlers.NotificationHandler{
		DB: db,
	}

	realtimeHub := realtime.NewHub()
	realtimeHandler := &realtime.ChatHandler{
		Hub:       realtimeHub,
		JWTSecret: cfg.JWTSecret,
		DB:        db,
	}

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handlers.StartPresenceCleanup(appCtx, db)

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"app":      "MoveMate",
			"status":   "online",
			"database": "connected",
		})
	})

	api := router.Group("/api/v1")

	auth := api.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
	}

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(cfg.JWTSecret))
	{
		protected.GET("/me", authHandler.Me)
		protected.PATCH("/me", authHandler.UpdateProfile)

		protected.GET("/notifications", notificationHandler.List)
		protected.GET("/notifications/unread", notificationHandler.UnreadCount)
		protected.POST("/notifications/:id/read", notificationHandler.MarkRead)

		protected.POST("/vehicles", vehicleHandler.Create)
		protected.GET("/vehicles", vehicleHandler.List)
		protected.PATCH("/vehicles/:id", vehicleHandler.Update)
		protected.DELETE("/vehicles/:id", vehicleHandler.Delete)
		protected.POST("/bookings", bookingHandler.Create)
		protected.GET("/bookings", bookingHandler.List)
		protected.GET("/bookings/:id", bookingHandler.Details)
		protected.GET("/driver/bookings/available", bookingHandler.AvailableForDriver)
		protected.GET("/driver/bookings", bookingHandler.ListDriverBookings)
		protected.POST("/driver/presence/online", presenceHandler.SetOnline)
		protected.POST("/driver/presence/offline", presenceHandler.SetOffline)
		protected.POST("/driver/presence/heartbeat", presenceHandler.Heartbeat)
		protected.POST("/driver/location", locationHandler.Update)
		protected.GET("/bookings/:id/driver/location", locationHandler.Get)
		protected.GET("/bookings/:id/drivers/nearby", nearbyDriversHandler.List)
		protected.POST("/bookings/:id/offers", bookingHandler.CreateOffer)
		protected.GET("/bookings/:id/offers", bookingHandler.ListOffers)
		protected.POST("/bookings/:id/offers/:offer_id/accept", bookingHandler.AcceptOffer)
		protected.POST("/bookings/:id/chat/messages", bookingHandler.SendMessage)
		protected.GET("/bookings/:id/chat/messages", bookingHandler.ListMessages)
		protected.POST("/bookings/:id/chat/read", bookingHandler.MarkMessagesRead)
		protected.GET("/bookings/:id/chat/unread", bookingHandler.UnreadMessageCount)
		protected.PATCH("/bookings/:id/status", bookingHandler.UpdateStatus)
		protected.POST("/bookings/:id/delivery-otp", bookingHandler.GenerateDeliveryOTP)
		protected.POST("/bookings/:id/rating", ratingHandler.Create)
		protected.POST("/bookings/:id/payment", paymentHandler.Create)
		protected.GET("/bookings/:id/payment", paymentHandler.Get)
		protected.GET("/bookings/:id/ratings", ratingHandler.ListBookingRatings)
		protected.POST("/bookings/:id/payment/retry", paymentHandler.Retry)
		protected.POST("/bookings/:id/payment/refund", paymentHandler.RequestRefund)
		protected.GET("/users/:id/rating", ratingHandler.UserRating)
		protected.POST("/bookings/:id/delivery/verify-otp", bookingHandler.VerifyDeliveryOTP)
	}

	api.POST("/webhooks/razorpay", paymentHandler.RazorpayWebhook)
	api.GET("/bookings/:id/chat/ws", realtimeHandler.Connect)

	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
