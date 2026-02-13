package main

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v2"
)


func ValidateUsername(username string) error {
	if len(username) < 3 {
		return fiber.NewError(400, "El nombre de usuario debe tener al menos 3 caracteres")
	}
	if len(username) > 50 {
		return fiber.NewError(400, "El nombre de usuario no puede tener más de 50 caracteres")
	}
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	if !matched {
		return fiber.NewError(400, "El nombre de usuario solo puede contener letras, números y guiones bajos")
	}
	return nil
}

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fiber.NewError(400, "La contraseña debe tener al menos 8 caracteres")
	}
	if len(password) > 100 {
		return fiber.NewError(400, "La contraseña no puede tener más de 100 caracteres")
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fiber.NewError(400, "La contraseña debe contener al menos una letra mayúscula")
	}
	if !hasLower {
		return fiber.NewError(400, "La contraseña debe contener al menos una letra minúscula")
	}
	if !hasNumber {
		return fiber.NewError(400, "La contraseña debe contener al menos un número")
	}
	if !hasSpecial {
		return fiber.NewError(400, "La contraseña debe contener al menos un carácter especial")
	}

	return nil
}

func ValidateEmail(email string) error {
	if email == "" {
		return nil // Email opcional
	}
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return fiber.NewError(400, "Formato de email inválido")
	}
	return nil
}

func ValidateIP(ip string) error {
	ipRegex := regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	if !ipRegex.MatchString(ip) {
		return fiber.NewError(400, "Formato de IP inválido")
	}
	parts := strings.Split(ip, ".")
	for _, part := range parts {
		if len(part) > 1 && part[0] == '0' {
			return fiber.NewError(400, "IP inválida: no se permiten ceros a la izquierda")
		}
	}
	return nil
}

func ValidateSSID(ssid string) error {
	if len(ssid) == 0 {
		return fiber.NewError(400, "El SSID no puede estar vacío")
	}
	if len(ssid) > 32 {
		return fiber.NewError(400, "El SSID no puede tener más de 32 caracteres")
	}
	return nil
}

const maxConfigSize = 64 * 1024 // 64 KB

// ValidateWireGuardConfig valida tamaño y estructura básica para evitar payloads maliciosos
func ValidateWireGuardConfig(config string) error {
	if len(config) == 0 {
		return fiber.NewError(400, "Configuración requerida")
	}
	if len(config) > maxConfigSize {
		return fiber.NewError(400, "Configuración demasiado grande")
	}
	if strings.Contains(config, "\x00") {
		return fiber.NewError(400, "Configuración contiene bytes nulos inválidos")
	}
	lower := strings.ToLower(config)
	if !strings.Contains(lower, "[interface]") && !strings.Contains(lower, "privatekey") {
		return fiber.NewError(400, "Configuración WireGuard inválida: debe contener [Interface] y PrivateKey")
	}
	return nil
}

// ValidateVPNConfig valida tamaño y estructura básica para configs OpenVPN
func ValidateVPNConfig(config string) error {
	if len(config) == 0 {
		return fiber.NewError(400, "Configuración requerida")
	}
	if len(config) > maxConfigSize {
		return fiber.NewError(400, "Configuración demasiado grande")
	}
	if strings.Contains(config, "\x00") {
		return fiber.NewError(400, "Configuración contiene bytes nulos inválidos")
	}
	lower := strings.ToLower(config)
	if !strings.Contains(lower, "client") && !strings.Contains(lower, "dev ") && !strings.Contains(lower, "remote ") {
		return fiber.NewError(400, "Configuración OpenVPN inválida: debe parecer un config cliente válido")
	}
	return nil
}
