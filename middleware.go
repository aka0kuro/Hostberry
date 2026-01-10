package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// requireAuth middleware que requiere autenticación
func requireAuth(c *fiber.Ctx) error {
	// Permitir preflight CORS sin autenticación
	if c.Method() == fiber.MethodOptions {
		return c.Next()
	}

	path := c.Path()

	// IMPORTANTÍSIMO: los assets estáticos nunca deben requerir auth.
	// Si /static/* se redirige a /login, el navegador recibe HTML en vez de CSS/JS
	// y aparecen errores de MIME (nosniff) y errores de sintaxis JS.
	if strings.HasPrefix(path, "/static/") {
		return c.Next()
	}
	// También permitir páginas públicas incluso si este middleware se aplica globalmente.
	if path == "/login" || path == "/first-login" || path == "/" {
		return c.Next()
	}
	
	// Rutas públicas que NO requieren autenticación
	// Verificar primero las rutas públicas antes de cualquier otra lógica
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

	// Normalizar path (sin slash final para comparación)
	normalizedPath := strings.TrimRight(path, "/")
	if normalizedPath == "" {
		normalizedPath = "/"
	}

	// Verificar si es una ruta pública (comparación exacta)
	if publicPaths[path] || publicPaths[normalizedPath] {
		// Ruta pública, permitir sin autenticación
		return c.Next()
	}

	// Verificar rutas con prefijo (para rutas como /api/v1/translations/:lang)
	if strings.HasPrefix(path, "/api/v1/translations/") {
		return c.Next()
	}

	// Si llegamos aquí, la ruta requiere autenticación
	var token string

	// Para APIs, obtener token del header Authorization
	if strings.HasPrefix(path, "/api/") {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			// Fallback: permitir también cookie o query token para clientes que no usan Authorization
			// (p.ej. navegadores donde la sesión se maneja por cookie o links con ?token=)
			token = c.Cookies("access_token")
			if token == "" {
				token = c.Query("token")
			}
			if token == "" {
				return c.Status(401).JSON(fiber.Map{
					"error": "No autorizado - token requerido",
				})
			}
			// Ya tenemos token válido para validar más abajo
		} else {
			// Extraer token (formato: "Bearer <token>")
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				return c.Status(401).JSON(fiber.Map{
					"error": "Formato de token inválido",
				})
			}
			token = parts[1]
		}

	} else {
		// Para rutas web, intentar obtener token de cookie o query parameter
		token = c.Cookies("access_token")
		if token == "" {
			token = c.Query("token")
		}

		// Si no hay token, redirigir a login
		if token == "" {
			return c.Redirect("/login")
		}
	}

	// Validar token
	claims, err := ValidateToken(token)
	if err != nil {
		if strings.HasPrefix(path, "/api/") {
			return c.Status(401).JSON(fiber.Map{
				"error": "Token inválido o expirado",
			})
		}
		return c.Redirect("/login")
	}

	// Obtener usuario de la base de datos
	var user User
	if err := db.First(&user, claims.UserID).Error; err != nil {
		log.Printf("ERROR: No se pudo encontrar usuario con ID %d en requireAuth: %v", claims.UserID, err)
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

	// Agregar usuario al contexto
	c.Locals("user", &user)
	c.Locals("user_id", user.ID)

	return c.Next()
}

// rateLimitMiddleware está definido en rate_limiter.go

// loggingMiddleware middleware de logging de requests
func loggingMiddleware(c *fiber.Ctx) error {
	start := time.Now()

	// Continuar con el request
	err := c.Next()

	// Log después de procesar
	duration := time.Since(start)

	// Omitir logs de archivos estáticos en el middleware personalizado también
	path := c.Path()
	if strings.HasPrefix(path, "/static/") {
		return err
	}

	// Capturar valores del contexto ANTES de la goroutine
	// (el contexto no es seguro para usar en goroutines)
	method := c.Method()
	ip := c.IP()
	status := c.Response().StatusCode()

	userID := c.Locals("user_id")
	var userIDPtr *int
	if userID != nil {
		id := userID.(int)
		userIDPtr = &id
	}

	// Formato más legible para logs en BD
	statusEmoji := "✅"
	if status >= 400 && status < 500 {
		statusEmoji = "⚠️"
	} else if status >= 500 {
		statusEmoji = "❌"
	}

	// Formatear duración de forma más legible
	durationStr := duration.String()
	if duration < time.Millisecond {
		durationStr = fmt.Sprintf("%.0fµs", float64(duration.Nanoseconds())/1000)
	} else if duration < time.Second {
		durationStr = fmt.Sprintf("%.2fms", float64(duration.Nanoseconds())/1000000)
	} else {
		durationStr = fmt.Sprintf("%.2fs", duration.Seconds())
	}

	// Insertar log en BD (async, no bloquea)
	go func() {
		InsertLog(
			"INFO",
			fmt.Sprintf("%s %s %s | %s | %s | %s", statusEmoji, method, path, ip, durationStr, fmt.Sprintf("HTTP %d", status)),
			"http",
			userIDPtr,
		)
	}()

	return err
}

// errorHandler maneja errores de la aplicación
func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Error interno del servidor"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	// Capturar valores del contexto ANTES de la goroutine
	// (el contexto no es seguro para usar en goroutines)
	method := c.Method()
	path := c.Path()
	errMsg := err.Error()

	userID := c.Locals("user_id")
	var userIDPtr *int
	if userID != nil {
		id := userID.(int)
		userIDPtr = &id
	}

	// Solo registrar errores del servidor (500+) como ERROR
	// Los errores de validación (400-499) son esperados y no se registran como ERROR
	if code >= 500 {
		// Log detallado del error del servidor
		log.Printf("❌ Error en %s %s: %v", method, path, err)

		go func() {
			InsertLog(
				"ERROR",
				"Error en "+path+": "+errMsg,
				"http",
				userIDPtr,
			)
		}()
	}

	// Si es una petición de API, retornar JSON
	if strings.HasPrefix(c.Path(), "/api/") {
		return c.Status(code).JSON(fiber.Map{
			"error":   message,
			"path":    c.Path(),
			"method":  c.Method(),
			"details": err.Error(),
		})
	}

	// Si es una página web, intentar renderizar página de error
	// Si falla el render, retornar error simple
	if renderErr := renderTemplate(c, "error", fiber.Map{
		"Title":   "Error",
		"Code":    code,
		"Message": message,
		"Details": err.Error(),
	}); renderErr != nil {
		log.Printf("❌ Error al renderizar página de error: %v", renderErr)
		// Fallback: retornar HTML simple
		return c.Status(code).SendString(fmt.Sprintf(
			"<html><body><h1>Error %d</h1><p>%s</p><p>%s</p></body></html>",
			code, message, err.Error(),
		))
	}
	return nil
}
