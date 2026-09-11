package handlers

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartPresenceCleanup(ctx context.Context, db *pgxpool.Pool) {
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				cleanupStaleDrivers(ctx, db)
			}
		}
	}()
}

func cleanupStaleDrivers(ctx context.Context, db *pgxpool.Pool) {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := db.Exec(
		queryCtx,
		`UPDATE driver_presence
		SET
			is_online = FALSE,
			updated_at = NOW()
		WHERE is_online = TRUE
			AND (
				last_seen_at IS NULL
				OR last_seen_at < NOW() - INTERVAL '2 minutes'
			)`,
	)

	if err != nil {
		log.Printf("presence cleanup failed: %v", err)
		return
	}

	if result.RowsAffected() > 0 {
		log.Printf("presence cleanup: %d stale driver(s) marked offline", result.RowsAffected())
	}
}
