package main

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

type HealthCheckResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Version   string            `json:"version"`
	Services  map[string]string `json:"services"`
}

func healthCheckHandler(c *fiber.Ctx) error {
	response := HealthCheckResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "2.0.0",
		Services:  make(map[string]string),
	}

	if db != nil {
		sqlDB, err := db.DB()
		if err == nil {
			if err := sqlDB.Ping(); err == nil {
				response.Services["database"] = "healthy"
			} else {
				response.Services["database"] = "unhealthy"
				response.Status = "degraded"
			}
		} else {
			response.Services["database"] = "unhealthy"
			response.Status = "degraded"
		}
	} else {
		response.Services["database"] = "not_configured"
		response.Status = "degraded"
	}


	if i18nManager != nil {
		response.Services["i18n"] = "healthy"
	} else {
		response.Services["i18n"] = "unhealthy"
		response.Status = "degraded"
	}

	statusCode := 200
	if response.Status == "degraded" {
		statusCode = 503
	}

	return c.Status(statusCode).JSON(response)
}

func readinessCheckHandler(c *fiber.Ctx) error {
	if db == nil {
		return c.Status(503).JSON(fiber.Map{
			"status":  "not_ready",
			"message": "Database not initialized",
		})
	}

	sqlDB, err := db.DB()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status":  "not_ready",
			"message": "Database connection error",
		})
	}

	if err := sqlDB.Ping(); err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status":  "not_ready",
			"message": "Database ping failed",
		})
	}

	return c.JSON(fiber.Map{
		"status":  "ready",
		"message": "Application is ready",
	})
}

func livenessCheckHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "alive",
		"message": "Application is running",
	})
}
