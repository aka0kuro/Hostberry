package main

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gofiber/fiber/v2"
)

func requestIDMiddleware(c *fiber.Ctx) error {
	requestID := c.Get("X-Request-ID")
	
	if requestID == "" {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err == nil {
			requestID = hex.EncodeToString(bytes)
		} else {
			requestID = generateSimpleID()
		}
	}

	c.Locals("request_id", requestID)
	
	c.Set("X-Request-ID", requestID)

	return c.Next()
}

func generateSimpleID() string {
	return hex.EncodeToString([]byte{byte(time.Now().Unix() % 256)})
}
