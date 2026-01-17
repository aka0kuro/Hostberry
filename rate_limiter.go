package main

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.RWMutex
	maxReqs  int
	window   time.Duration
}

var globalRateLimiter *RateLimiter

func NewRateLimiter(maxReqs int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		maxReqs:  maxReqs,
		window:   window,
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	
	cutoff := now.Add(-rl.window)
	validReqs := []time.Time{}
	for _, reqTime := range rl.requests[key] {
		if reqTime.After(cutoff) {
			validReqs = append(validReqs, reqTime)
		}
	}
	rl.requests[key] = validReqs

	if len(rl.requests[key]) >= rl.maxReqs {
		return false
	}

	rl.requests[key] = append(rl.requests[key], now)
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)

		for key, reqs := range rl.requests {
			validReqs := []time.Time{}
			for _, reqTime := range reqs {
				if reqTime.After(cutoff) {
					validReqs = append(validReqs, reqTime)
				}
			}
			if len(validReqs) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = validReqs
			}
		}
		rl.mu.Unlock()
	}
}

func rateLimitMiddleware(c *fiber.Ctx) error {
	if globalRateLimiter == nil {
		globalRateLimiter = NewRateLimiter(
			appConfig.Security.RateLimitRPS,
			time.Second,
		)
	}

	key := c.IP()
	if userID := c.Locals("user_id"); userID != nil {
		key = "user_" + string(rune(userID.(int)))
	}

	if !globalRateLimiter.Allow(key) {
		return c.Status(429).JSON(fiber.Map{
			"error": "Demasiadas peticiones. Por favor, intenta más tarde.",
		})
	}

	return c.Next()
}
