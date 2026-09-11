package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VehicleHandler struct {
	DB *pgxpool.Pool
}

type CreateVehicleRequest struct {
	VehicleType   string `json:"vehicle_type"`
	VehicleNumber string `json:"vehicle_number"`
	Model         string `json:"model"`
}

func (h *VehicleHandler) Create(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can add vehicles",
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "authentication required",
		})
		return
	}

	var req CreateVehicleRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.VehicleType = strings.ToLower(strings.TrimSpace(req.VehicleType))
	req.VehicleNumber = strings.ToUpper(strings.TrimSpace(req.VehicleNumber))
	req.Model = strings.TrimSpace(req.Model)

	validTypes := map[string]bool{
		"bike":   true,
		"auto":   true,
		"car":    true,
		"pickup": true,
		"tempo":  true,
		"truck":  true,
		"other":  true,
	}

	if !validTypes[req.VehicleType] {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid vehicle type",
		})
		return
	}

	if req.VehicleNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "vehicle number is required",
		})
		return
	}

	var vehicleID string

	err := h.DB.QueryRow(
		c,
		`INSERT INTO vehicles
			(driver_id, vehicle_type, vehicle_number, model)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		userID,
		req.VehicleType,
		req.VehicleNumber,
		req.Model,
	).Scan(&vehicleID)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "vehicle number already exists or vehicle could not be added",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "vehicle added successfully",
		"vehicle": gin.H{
			"id":             vehicleID,
			"vehicle_type":   req.VehicleType,
			"vehicle_number": req.VehicleNumber,
			"model":          req.Model,
			"is_active":      true,
		},
	})
}

func (h *VehicleHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")

	rows, err := h.DB.Query(
		c,
		`SELECT id, vehicle_type, vehicle_number, model, is_active, created_at
		 FROM vehicles
		 WHERE driver_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load vehicles",
		})
		return
	}
	defer rows.Close()

	vehicles := make([]gin.H, 0)

	for rows.Next() {
		var (
			id            string
			vehicleType   string
			vehicleNumber string
			model         string
			isActive      bool
			createdAt     interface{}
		)

		if err := rows.Scan(
			&id,
			&vehicleType,
			&vehicleNumber,
			&model,
			&isActive,
			&createdAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to read vehicles",
			})
			return
		}

		vehicles = append(vehicles, gin.H{
			"id":             id,
			"vehicle_type":   vehicleType,
			"vehicle_number": vehicleNumber,
			"model":          model,
			"is_active":      isActive,
			"created_at":     createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to load vehicles",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"vehicles": vehicles,
	})
}

type UpdateVehicleRequest struct {
	VehicleType   string `json:"vehicle_type"`
	VehicleNumber string `json:"vehicle_number"`
	Model         string `json:"model"`
	IsActive      *bool  `json:"is_active"`
}

func (h *VehicleHandler) Update(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can update vehicles",
		})
		return
	}

	userID, _ := c.Get("user_id")
	vehicleID := c.Param("id")

	var req UpdateVehicleRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid request body",
		})
		return
	}

	req.VehicleType = strings.ToLower(strings.TrimSpace(req.VehicleType))
	req.VehicleNumber = strings.ToUpper(strings.TrimSpace(req.VehicleNumber))
	req.Model = strings.TrimSpace(req.Model)

	validTypes := map[string]bool{
		"bike":   true,
		"auto":   true,
		"car":    true,
		"pickup": true,
		"tempo":  true,
		"truck":  true,
		"other":  true,
	}

	if !validTypes[req.VehicleType] {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid vehicle type",
		})
		return
	}

	if req.VehicleNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "vehicle number is required",
		})
		return
	}

	if req.IsActive == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "is_active is required",
		})
		return
	}

	var (
		id            string
		vehicleType   string
		vehicleNumber string
		model         string
		isActive      bool
	)

	err := h.DB.QueryRow(
		c,
		`UPDATE vehicles
		 SET vehicle_type = $1,
		     vehicle_number = $2,
		     model = $3,
		     is_active = $4
		 WHERE id = $5 AND driver_id = $6
		 RETURNING id, vehicle_type, vehicle_number, model, is_active`,
		req.VehicleType,
		req.VehicleNumber,
		req.Model,
		*req.IsActive,
		vehicleID,
		userID,
	).Scan(
		&id,
		&vehicleType,
		&vehicleNumber,
		&model,
		&isActive,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "vehicle not found or could not be updated",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "vehicle updated successfully",
		"vehicle": gin.H{
			"id":             id,
			"vehicle_type":   vehicleType,
			"vehicle_number": vehicleNumber,
			"model":          model,
			"is_active":      isActive,
		},
	})
}

func (h *VehicleHandler) Delete(c *gin.Context) {
	role, _ := c.Get("role")

	if role != "driver" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "only drivers can delete vehicles",
		})
		return
	}

	userID, _ := c.Get("user_id")
	vehicleID := c.Param("id")

	commandTag, err := h.DB.Exec(
		c,
		`DELETE FROM vehicles
		 WHERE id = $1 AND driver_id = $2`,
		vehicleID,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to delete vehicle",
		})
		return
	}

	if commandTag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "vehicle not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "vehicle deleted successfully",
	})
}

// Keep auth package referenced while this handler evolves.
