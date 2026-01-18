package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func requireAuth(c *fiber.Ctx) error {
	if c.Method() == fiber.MethodOptions {
		return c.Next()
	}

	path := c.Path()

	if strings.HasPrefix(path, "/static/") {
		return c.Next()
	}
	if path == "/login" || path == "/first-login" || path == "/" {
		return c.Next()
	}
	
	publicPaths := map[string]bool{
		"/api/v1/auth/login":     true,
		"/api/v1/auth/login/":    true, // Con slash final
		"/api/v1/translations":   true,
		"/api/v1/translations/":  true,
		"/health":                 true,
		"/health/":                true,
		"/health/ready":           true,
		"/health/ready/":          true,
		"/health/live":            true,
		"/health/live/":           true,
	}

	normalizedPath := strings.TrimRight(path, "/")
	if normalizedPath == "" {
		normalizedPath = "/"
	}

	if publicPaths[path] || publicPaths[normalizedPath] {
		return c.Next()
	}

	if strings.HasPrefix(path, "/api/v1/translations/") {
		return c.Next()
	}

	var token string

	if strings.HasPrefix(path, "/api/") {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			token = c.Cookies("access_token")
			if token == "" {
				token = c.Query("token")
			}
			if token == "" {
				return c.Status(401).JSON(fiber.Map{
					"error": "No autorizado - token requerido",
				})
			}
		} else {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				return c.Status(401).JSON(fiber.Map{
					"error": "Formato de token inválido",
				})
			}
			token = parts[1]
		}

	} else {
		token = c.Cookies("access_token")
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			return c.Redirect("/login")
		}
	}

	claims, err := ValidateToken(token)
	if err != nil {
		if strings.HasPrefix(path, "/api/") {
			return c.Status(401).JSON(fiber.Map{
				"error": "Token inválido o expirado",
			})
		}
		return c.Redirect("/login")
	}

	var user User
	if err := db.First(&user, claims.UserID).Error; err != nil {
		LogTf("logs.middleware_user_not_found", claims.UserID, err)
		if strings.HasPrefix(path, "/api/") {
			return c.Status(401).JSON(fiber.Map{
				"error": "Usuario no encontrado. Por favor, inicia sesión nuevamente.",
				"code":   "USER_NOT_FOUND",
			})
		}
		return c.Redirect("/login")
	}

	if !user.IsActive {
		if strings.HasPrefix(path, "/api/") {
			return c.Status(401).JSON(fiber.Map{
				"error": "Usuario inactivo",
			})
		}
		return c.Redirect("/login")
	}

	c.Locals("user", &user)
	c.Locals("user_id", user.ID)

	return c.Next()
}


func loggingMiddleware(c *fiber.Ctx) error {
	start := time.Now()

	err := c.Next()

	duration := time.Since(start)

	path := c.Path()
	if strings.HasPrefix(path, "/static/") {
		return err
	}

	method := c.Method()
	ip := c.IP()
	status := c.Response().StatusCode()

	userID := c.Locals("user_id")
	var userIDPtr *int
	if userID != nil {
		id := userID.(int)
		userIDPtr = &id
	}

	statusEmoji := "✅"
	if status >= 400 && status < 500 {
		statusEmoji = "⚠️"
	} else if status >= 500 {
		statusEmoji = "❌"
	}

	durationStr := duration.String()
	if duration < time.Millisecond {
		durationStr = fmt.Sprintf("%.0fµs", float64(duration.Nanoseconds())/1000)
	} else if duration < time.Second {
		durationStr = fmt.Sprintf("%.2fms", float64(duration.Nanoseconds())/1000000)
	} else {
		durationStr = fmt.Sprintf("%.2fs", duration.Seconds())
	}

	if appConfig.Server.Debug || status >= 400 {
		go func() {
			InsertLog(
				"INFO",
				fmt.Sprintf("%s %s %s | %s | %s | %s", statusEmoji, method, path, ip, durationStr, fmt.Sprintf("HTTP %d", status)),
				"http",
				userIDPtr,
			)
		}()
	}

	return err
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Error interno del servidor"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	method := c.Method()
	path := c.Path()
	errMsg := err.Error()

	userID := c.Locals("user_id")
	var userIDPtr *int
	if userID != nil {
		id := userID.(int)
		userIDPtr = &id
	}

	if code >= 500 {
		if appConfig.Server.Debug {
			LogTf("logs.middleware_error", method, path, err)
		}
		go func() {
			InsertLog(
				"ERROR",
				"Error en "+path+": "+errMsg,
				"http",
				userIDPtr,
			)
		}()
	}

	if strings.HasPrefix(c.Path(), "/api/") {
		return c.Status(code).JSON(fiber.Map{
			"error":   message,
			"path":    c.Path(),
			"method":  c.Method(),
			"details": err.Error(),
		})
	}

	if renderErr := renderTemplate(c, "error", fiber.Map{
		"Title":   "Error",
		"Code":    code,
		"Message": message,
		"Details": err.Error(),
	}); renderErr != nil {
		LogTf("logs.middleware_render_error", renderErr)
		return c.Status(code).SendString(fmt.Sprintf(
			"<html><body><h1>Error %d</h1><p>%s</p><p>%s</p></body></html>",
			code, message, err.Error(),
		))
	}
	return nil
}
