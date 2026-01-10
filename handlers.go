package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Handlers de autenticación
func loginAPIHandler(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}

	// Validar username
	if err := ValidateUsername(req.Username); err != nil {
		return err
	}

	// Validar password (solo formato, no longitud mínima para login)
	if req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "La contraseña es requerida",
		})
	}

	user, token, err := Login(req.Username, req.Password)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Log de login
	userID := user.ID
	InsertLog("INFO", fmt.Sprintf("Usuario %s inició sesión", user.Username), "auth", &userID)

	// Determinar si se requiere cambio de contraseña
	// Se requiere si es el primer login (LoginCount == 1 después del incremento en Login)
	// Esto significa que el usuario acaba de hacer su primer login exitoso
	passwordChangeRequired := user.LoginCount == 1

	// También setear cookie para permitir render protegido en rutas web (HttpOnly)
	// Configurar expiración de la cookie igual a la del token
	cookieExpiry := time.Duration(appConfig.Security.TokenExpiry) * time.Minute
	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   int(cookieExpiry.Seconds()), // Expira al mismo tiempo que el token
		// Secure: true, // si sirves por HTTPS
	})

	return c.JSON(fiber.Map{
		"access_token":            token,
		"password_change_required": passwordChangeRequired,
		"user": fiber.Map{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
		},
	})
}

func logoutAPIHandler(c *fiber.Ctx) error {
	// En JWT stateless, el logout es principalmente del lado del cliente
	// Pero podemos registrar el evento
	user := c.Locals("user").(*User)
	userID := user.ID
	InsertLog("INFO", fmt.Sprintf("Usuario %s cerró sesión", user.Username), "auth", &userID)

	// Limpiar cookie para rutas web
	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		MaxAge:   -1,
	})

	return c.JSON(fiber.Map{
		"message": "Sesión cerrada",
	})
}

func meHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)
	return c.JSON(fiber.Map{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"first_name": user.FirstName,
		"last_name":  user.LastName,
		"role":       user.Role,
		"timezone":   user.Timezone,
	})
}

func changePasswordAPIHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Contraseñas requeridas"})
	}
	if !CheckPassword(req.CurrentPassword, user.Password) {
		return c.Status(401).JSON(fiber.Map{"error": "Contraseña actual incorrecta"})
	}

	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error hasheando contraseña"})
	}
	user.Password = hashed
	if err := db.Save(user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error guardando contraseña"})
	}

	userID := user.ID
		InsertLog("INFO", fmt.Sprintf("Usuario %s cambió su contraseña", user.Username), "auth", &userID)
	return c.JSON(fiber.Map{"message": "Contraseña actualizada"})
}

func firstLoginChangeAPIHandler(c *fiber.Ctx) error {
	// Obtener token del header Authorization o de la cookie
	tokenString := c.Get("Authorization")
	if tokenString != "" {
		// Remover "Bearer " si está presente
		tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	} else {
		// Intentar obtener de la cookie
		tokenString = c.Cookies("access_token")
	}

	if tokenString == "" {
		return c.Status(401).JSON(fiber.Map{
			"error": "Token requerido",
		})
	}

	// Validar token
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": "Token inválido",
		})
	}

	// Obtener usuario
	var user User
	if err := db.Where("id = ? AND is_active = ?", claims.UserID, true).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Usuario no encontrado",
		})
	}

	// Verificar que es el primer login (LoginCount == 1)
	if user.LoginCount != 1 {
		return c.Status(403).JSON(fiber.Map{
			"error": "Este endpoint solo está disponible en el primer login",
		})
	}

	// Parsear request
	var req struct {
		NewUsername string `json:"new_username"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}

	// Validar nuevo username
	if req.NewUsername != "" {
		if err := ValidateUsername(req.NewUsername); err != nil {
			return err
		}
		// Verificar que el nuevo username no esté en uso (si es diferente al actual)
		if req.NewUsername != user.Username {
			var existingUser User
			if err := db.Where("username = ?", req.NewUsername).First(&existingUser).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{
					"error": "El nombre de usuario ya está en uso",
				})
			}
			user.Username = req.NewUsername
		}
	}

	// Validar nueva contraseña
	if req.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "La nueva contraseña es requerida",
		})
	}
	if err := ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	// Hashear nueva contraseña
	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Error hasheando contraseña",
		})
	}
	user.Password = hashed

	// Incrementar LoginCount para que no pueda volver a usar este endpoint
	user.LoginCount++

	// Guardar cambios
	if err := db.Save(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Error guardando credenciales",
		})
	}

	userID := user.ID
	InsertLog("INFO", "Usuario cambió credenciales en primer login: "+user.Username, "auth", &userID)
	
	return c.JSON(fiber.Map{
		"message": "Credenciales actualizadas. Por favor, inicia sesión nuevamente.",
	})
}

func updateProfileAPIHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)

	var req struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Timezone  string `json:"timezone"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}

	// Campos opcionales
	user.Email = req.Email
	user.FirstName = req.FirstName
	user.LastName = req.LastName
	if req.Timezone != "" {
		user.Timezone = req.Timezone
	}

	if err := db.Save(user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error guardando perfil"})
	}

	userID := user.ID
		InsertLog("INFO", fmt.Sprintf("Usuario %s actualizó su perfil", user.Username), "auth", &userID)
	return c.JSON(fiber.Map{"message": "Perfil actualizado"})
}

func updatePreferencesAPIHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)

	var req struct {
		EmailNotifications bool `json:"email_notifications"`
		SystemAlerts       bool `json:"system_alerts"`
		SecurityAlerts     bool `json:"security_alerts"`
		ShowActivity       bool `json:"show_activity"`
		DataCollection     bool `json:"data_collection"`
		Analytics          bool `json:"analytics"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}

	user.EmailNotifications = req.EmailNotifications
	user.SystemAlerts = req.SystemAlerts
	user.SecurityAlerts = req.SecurityAlerts
	user.ShowActivity = req.ShowActivity
	user.DataCollection = req.DataCollection
	user.Analytics = req.Analytics

	if err := db.Save(user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error guardando preferencias"})
	}

	userID := user.ID
		InsertLog("INFO", fmt.Sprintf("Usuario %s actualizó sus preferencias", user.Username), "auth", &userID)
	return c.JSON(fiber.Map{"message": "Preferencias actualizadas"})
}

// Handlers del sistema
func systemInfoHandler(c *fiber.Ctx) error {
	result := getSystemInfo()
	return c.JSON(result)
}

func systemShutdownHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)
	userID := user.ID

	result := systemShutdown(user.Username)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("Sistema apagado por usuario %s", user.Username), "system", &userID)
		return c.JSON(result)
	}

	if err, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error apagando sistema: %s (usuario: %s)", err, user.Username), "system", &userID)
		return c.Status(500).JSON(fiber.Map{"error": err})
	}

	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}

// Handlers de red
func networkStatusHandler(c *fiber.Ctx) error {
	result := getNetworkStatus()
	return c.JSON(result)
}

func networkInterfacesHandler(c *fiber.Ctx) error {
	// Obtener interfaces de red
	result := getNetworkInterfaces()
	if result != nil {
		if interfaces, ok := result["interfaces"]; ok {
			if interfacesArray, ok := interfaces.([]map[string]interface{}); ok && len(interfacesArray) > 0 {
				log.Printf("✅ Función Go devolvió %d interfaces", len(interfacesArray))
				return c.JSON(result)
			}
		}
	}

	// Fallback: obtener interfaces directamente
	interfaces := []map[string]interface{}{}
	
	// Obtener lista de interfaces
	cmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}'")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ Error obteniendo interfaces: %v", err)
		return c.JSON(fiber.Map{"interfaces": interfaces})
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	log.Printf("📡 Interfaces encontradas: %v", lines)
	for _, ifaceName := range lines {
		ifaceName = strings.TrimSpace(ifaceName)
		if ifaceName == "" || ifaceName == "lo" {
			continue // Saltar loopback
		}
		
		// Verificar que la interfaz realmente existe (incluyendo ap0)
		// Esto asegura que interfaces virtuales como ap0 se muestren
		ifaceCheckCmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null", ifaceName))
		if ifaceCheckErr := ifaceCheckCmd.Run(); ifaceCheckErr != nil {
			log.Printf("⚠️ Interface %s no existe o no es accesible, saltando", ifaceName)
			continue
		}
		
		log.Printf("✅ Procesando interfaz: %s", ifaceName)

		iface := map[string]interface{}{
			"name": ifaceName,
			"ip":   "N/A",
			"mac":  "N/A",
			"state": "unknown",
		}

		// Obtener estado
		stateCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/operstate 2>/dev/null", ifaceName))
		if stateOut, err := stateCmd.Output(); err == nil {
			state := strings.TrimSpace(string(stateOut))
			if state == "" {
				// Si operstate está vacío, verificar con ip link show
				ipStateCmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null | grep -o 'state [A-Z]*' | awk '{print $2}'", ifaceName))
				if ipStateOut, ipStateErr := ipStateCmd.Output(); ipStateErr == nil {
					state = strings.TrimSpace(string(ipStateOut))
				}
				if state == "" {
					state = "unknown"
				}
			}
			iface["state"] = state
		} else {
			// Si no se puede leer operstate, intentar con ip link show
			ipStateCmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null | grep -o 'state [A-Z]*' | awk '{print $2}'", ifaceName))
			if ipStateOut, ipStateErr := ipStateCmd.Output(); ipStateErr == nil {
				state := strings.TrimSpace(string(ipStateOut))
				if state != "" {
					iface["state"] = state
				}
			}
		}
		
		// Para ap0, asegurar que se muestre incluso si está down
		if ifaceName == "ap0" {
			log.Printf("📡 Interfaz ap0 encontrada, estado: %s", iface["state"])
			// Verificar si ap0 está activa, si no, intentar activarla
			if iface["state"] == "down" || iface["state"] == "unknown" {
				log.Printf("⚠️ ap0 está down, intentando activarla...")
				activateCmd := exec.Command("sh", "-c", "sudo ip link set ap0 up 2>/dev/null")
				if activateErr := activateCmd.Run(); activateErr == nil {
					time.Sleep(500 * time.Millisecond)
					// Verificar estado nuevamente
					stateCmd2 := exec.Command("sh", "-c", "cat /sys/class/net/ap0/operstate 2>/dev/null")
					if stateOut2, err2 := stateCmd2.Output(); err2 == nil {
						newState := strings.TrimSpace(string(stateOut2))
						if newState != "" {
							iface["state"] = newState
							log.Printf("✅ ap0 activada, nuevo estado: %s", newState)
						}
					}
				}
			}
		}
		
		// Para interfaces WiFi (wlan*), verificar también el estado de wpa_supplicant
		if strings.HasPrefix(ifaceName, "wlan") {
			// Verificar si wpa_supplicant reporta conexión
			wpaStatusCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s status 2>/dev/null | grep 'wpa_state=' | cut -d= -f2", ifaceName))
			if wpaStateOut, err := wpaStatusCmd.Output(); err == nil {
				wpaState := strings.TrimSpace(string(wpaStateOut))
				if wpaState == "COMPLETED" {
					// Si wpa_supplicant dice COMPLETED, verificar que tenga IP
					// (esto se verificará más adelante cuando se obtenga la IP)
					iface["wpa_state"] = "COMPLETED"
				} else if wpaState == "ASSOCIATING" || wpaState == "ASSOCIATED" || wpaState == "4WAY_HANDSHAKE" || wpaState == "GROUP_HANDSHAKE" {
					// En proceso de conexión
					iface["wpa_state"] = wpaState
					iface["state"] = "connecting"
				} else {
					// No conectado
					iface["wpa_state"] = wpaState
					if iface["state"] == "up" {
						// Si la interfaz está "up" pero wpa_supplicant no está conectado, es "down"
						iface["state"] = "down"
					}
				}
			}
		}

		// Obtener IP y máscara de red
		// Método 1: ip addr show (más confiable) - intentar con y sin sudo
		ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | head -1", ifaceName))
		if ipOut, err := ipCmd.Output(); err == nil {
			ipLine := strings.TrimSpace(string(ipOut))
			if ipLine != "" {
				// Formato: "192.168.1.100/24"
				parts := strings.Split(ipLine, "/")
				iface["ip"] = parts[0]
				if len(parts) > 1 {
					iface["netmask"] = parts[1]
				}
			}
		}
		
		// Si no se obtuvo IP, intentar con sudo
		if (iface["ip"] == "N/A" || iface["ip"] == "") {
			ipCmdSudo := exec.Command("sh", "-c", fmt.Sprintf("sudo ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | head -1", ifaceName))
			if ipOutSudo, err := ipCmdSudo.Output(); err == nil {
				ipLineSudo := strings.TrimSpace(string(ipOutSudo))
				if ipLineSudo != "" {
					parts := strings.Split(ipLineSudo, "/")
					iface["ip"] = parts[0]
					if len(parts) > 1 {
						iface["netmask"] = parts[1]
					}
				}
			}
		}
		
		// Si no se obtuvo IP con el método anterior, intentar método alternativo
		if iface["ip"] == "N/A" || iface["ip"] == "" {
			// Método 2: ifconfig (fallback)
			ifconfigCmd := exec.Command("sh", "-c", fmt.Sprintf("ifconfig %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | head -1", ifaceName))
			if ifconfigOut, err := ifconfigCmd.Output(); err == nil {
				ifconfigLine := strings.TrimSpace(string(ifconfigOut))
				if ifconfigLine != "" {
					// Limpiar cualquier prefijo (ej: "addr:192.168.1.1" -> "192.168.1.1")
					ifconfigLine = strings.TrimPrefix(ifconfigLine, "addr:")
					iface["ip"] = ifconfigLine
				}
			}
		}
		
		// Si aún no hay IP, intentar obtener desde hostname -I filtrando por interfaz
		if iface["ip"] == "N/A" || iface["ip"] == "" {
			// Método 3: hostname -I y verificar qué IP pertenece a esta interfaz
			hostnameCmd := exec.Command("sh", "-c", "hostname -I 2>/dev/null | awk '{print $1}'")
			if hostnameOut, err := hostnameCmd.Output(); err == nil {
				hostnameIP := strings.TrimSpace(string(hostnameOut))
				if hostnameIP != "" {
					// Verificar si esta IP está en la interfaz
					checkCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep -q '%s' && echo '%s'", ifaceName, hostnameIP, hostnameIP))
					if checkOut, err := checkCmd.Output(); err == nil {
						checkIP := strings.TrimSpace(string(checkOut))
						if checkIP != "" {
							iface["ip"] = checkIP
						}
					}
				}
			}
		}
		
		// Si la interfaz está "up" pero no tiene IP, podría estar esperando DHCP
		// En ese caso, mostrar un mensaje más descriptivo
		if (iface["state"] == "up" || iface["state"] == "connected" || iface["state"] == "connecting") && (iface["ip"] == "N/A" || iface["ip"] == "") {
			// Verificar si hay un proceso DHCP corriendo
			dhcpCheck := exec.Command("sh", "-c", fmt.Sprintf("ps aux | grep -E '[d]hclient|udhcpc' | grep %s", ifaceName))
			if dhcpOut, err := dhcpCheck.Output(); err == nil {
				dhcpLine := strings.TrimSpace(string(dhcpOut))
				if dhcpLine != "" {
					iface["ip"] = "Obtaining IP..."
				}
			}
		}
		
		// Para interfaces WiFi, verificar el estado real de conexión
		if strings.HasPrefix(ifaceName, "wlan") {
			// Si wpa_supplicant dice COMPLETED pero no hay IP, aún no está completamente conectado
			if wpaState, hasWpaState := iface["wpa_state"]; hasWpaState && wpaState == "COMPLETED" {
				if iface["ip"] == "N/A" || iface["ip"] == "" || iface["ip"] == "Obtaining IP..." {
					// wpa_supplicant conectado pero sin IP aún
					iface["connected"] = false
					iface["state"] = "connecting"
				} else {
					// Realmente conectado con IP
					iface["connected"] = true
					iface["state"] = "connected"
				}
			} else if wpaState, hasWpaState := iface["wpa_state"]; hasWpaState && (wpaState == "ASSOCIATING" || wpaState == "ASSOCIATED" || wpaState == "4WAY_HANDSHAKE" || wpaState == "GROUP_HANDSHAKE") {
				// En proceso de conexión
				iface["connected"] = false
				iface["state"] = "connecting"
			} else {
				// No conectado
				iface["connected"] = false
				if iface["state"] != "down" {
					iface["state"] = "down"
				}
			}
		} else {
			// Para interfaces no WiFi, usar el estado del sistema
			if iface["ip"] != "N/A" && iface["ip"] != "" && iface["ip"] != "Obtaining IP..." {
				iface["connected"] = true
				if iface["state"] == "up" {
					iface["state"] = "connected"
				}
			} else {
				iface["connected"] = false
			}
		}

		// Obtener gateway para esta interfaz
		gatewayCmd := exec.Command("sh", "-c", fmt.Sprintf("ip route | grep %s | grep default | awk '{print $3}' | head -1", ifaceName))
		if gatewayOut, err := gatewayCmd.Output(); err == nil {
			gateway := strings.TrimSpace(string(gatewayOut))
			if gateway != "" {
				iface["gateway"] = gateway
			}
		}
		
		// Si no hay gateway específico, intentar obtener el gateway por defecto
		if _, hasGateway := iface["gateway"]; !hasGateway {
			defaultGatewayCmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}' | head -1")
			if defaultGatewayOut, err := defaultGatewayCmd.Output(); err == nil {
				defaultGateway := strings.TrimSpace(string(defaultGatewayOut))
				if defaultGateway != "" {
					iface["gateway"] = defaultGateway
				}
			}
		}

		// Obtener MAC
		macCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/address 2>/dev/null", ifaceName))
		if macOut, err := macCmd.Output(); err == nil {
			mac := strings.TrimSpace(string(macOut))
			if mac != "" {
				iface["mac"] = mac
			}
		}

		interfaces = append(interfaces, iface)
	}

	log.Printf("✅ Fallback devolvió %d interfaces", len(interfaces))
	return c.JSON(fiber.Map{"interfaces": interfaces})
}

// Handlers de WiFi
func wifiConnectHandler(c *fiber.Ctx) error {
	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
		Country  string `json:"country"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}

	user := c.Locals("user").(*User)
	userID := user.ID

	// Obtener país desde el request body o query (si está disponible)
	country := req.Country
	if country == "" {
		country = c.Query("country", DefaultCountryCode)
	}
	if country == "" {
		country = DefaultCountryCode
	}
	
	interfaceName := DefaultWiFiInterface
	// country ya está definido arriba

	result := connectWiFi(req.SSID, req.Password, interfaceName, country, user.Username)

	// Asegurar que result siempre tenga success y error
	if _, hasSuccess := result["success"]; !hasSuccess {
		result["success"] = false
	}
	if _, hasError := result["error"]; !hasError {
		if result["success"] == false {
			result["error"] = "Error desconocido al conectar a la red WiFi"
		} else {
			result["error"] = ""
		}
	}

	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("WiFi conectado a %s por usuario %s", req.SSID, user.Username), "wifi", &userID)
		return c.JSON(result)
	}

	// Si hay error, retornar con formato consistente que incluya success y error
	errorMsg := "Error desconocido"
	if errorMsgVal, ok := result["error"].(string); ok && errorMsgVal != "" {
		errorMsg = errorMsgVal
	}
		InsertLog("ERROR", fmt.Sprintf("Error conectando WiFi %s: %s (usuario: %s)", req.SSID, errorMsg, user.Username), "wifi", &userID)
	return c.Status(500).JSON(fiber.Map{
		"success": false,
		"error":   errorMsg,
		"message": fmt.Sprintf("Error conectando a %s", req.SSID),
	})
}

// Handlers de VPN
func vpnStatusHandler(c *fiber.Ctx) error {
	result := getVPNStatus()
	return c.JSON(result)
}

func vpnConnectHandler(c *fiber.Ctx) error {
	var req struct {
		Config string `json:"config"`
		Type   string `json:"type"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}

	user := c.Locals("user").(*User)
	userID := user.ID

	result := connectVPN(req.Config, req.Type, user.Username)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("VPN conectado: %s (usuario: %s)", req.Type, user.Username), "vpn", &userID)
		return c.JSON(result)
	}

	if errorMsg, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error conectando VPN %s: %s (usuario: %s)", req.Type, errorMsg, user.Username), "vpn", &userID)
		return c.Status(500).JSON(fiber.Map{"error": errorMsg})
	}

	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}

// Handlers de WireGuard
func wireguardStatusHandler(c *fiber.Ctx) error {
	result := getWireGuardStatus()
	return c.JSON(result)
}

// wireguardInterfacesHandler adapta el estado a la estructura esperada por wireguard.js
func wireguardInterfacesHandler(c *fiber.Ctx) error {
	// Intentar obtener interfaces via wg (más directo que Lua para estructura)
	out, err := exec.Command("wg", "show", "interfaces").CombinedOutput()
	if err != nil {
		// fallback a función Go
		result := getWireGuardStatus()
		if interfaces, ok := result["interfaces"].([]map[string]interface{}); ok && len(interfaces) > 0 {
			var resp []fiber.Map
			for _, iface := range interfaces {
				if name, ok := iface["name"].(string); ok {
					resp = append(resp, fiber.Map{
						"name":        name,
						"status":      "up",
						"address":     "",
						"peers_count": 0,
					})
				}
			}
			return c.JSON(resp)
		}
		return c.Status(500).JSON(fiber.Map{"error": strings.TrimSpace(string(out))})
	}

	ifaces := strings.Fields(strings.TrimSpace(string(out)))
	var resp []fiber.Map
	for _, iface := range ifaces {
		// peers count
		detailsOut, _ := exec.Command("wg", "show", iface).CombinedOutput()
		details := string(detailsOut)
		peersCount := 0
		for _, line := range strings.Split(details, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "peer:") {
				peersCount++
			}
		}
		resp = append(resp, fiber.Map{
			"name":        iface,
			"status":      "up",
			"address":     "", // opcional (depende de ip)
			"peers_count": peersCount,
		})
	}
	return c.JSON(resp)
}

// wireguardPeersHandler devuelve una lista simple de peers a partir de wg show wg0
func wireguardPeersHandler(c *fiber.Ctx) error {
	out, err := exec.Command("wg", "show").CombinedOutput()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": strings.TrimSpace(string(out))})
	}
	text := string(out)
	var peers []fiber.Map

	var curPeer string
	var handshake string
	var transfer string

	flush := func() {
		if curPeer == "" {
			return
		}
		connected := true
		if strings.Contains(handshake, "never") || handshake == "" {
			connected = false
		}
		name := curPeer
		if len(name) > 12 {
			name = name[:12] + "…"
		}
		peers = append(peers, fiber.Map{
			"name":      name,
			"connected": connected,
			"bandwidth": transfer,
			"uptime":    handshake,
		})
		curPeer, handshake, transfer = "", "", ""
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "peer:") {
			flush()
			curPeer = strings.TrimSpace(strings.TrimPrefix(line, "peer:"))
			continue
		}
		if strings.HasPrefix(line, "latest handshake:") {
			handshake = strings.TrimSpace(strings.TrimPrefix(line, "latest handshake:"))
			continue
		}
		if strings.HasPrefix(line, "transfer:") {
			transfer = strings.TrimSpace(strings.TrimPrefix(line, "transfer:"))
			continue
		}
	}
	flush()
	return c.JSON(peers)
}

// wireguardGetConfigHandler devuelve el contenido actual de /etc/wireguard/wg0.conf (si existe)
func wireguardGetConfigHandler(c *fiber.Ctx) error {
	out, err := exec.Command("sh", "-c", "cat /etc/wireguard/wg0.conf 2>/dev/null").CombinedOutput()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": strings.TrimSpace(string(out))})
	}
	return c.JSON(fiber.Map{"config": string(out)})
}

func wireguardToggleHandler(c *fiber.Ctx) error {
	// Toggle basado en estado actual
	statusOut, _ := exec.Command("wg", "show").CombinedOutput()
	active := strings.TrimSpace(string(statusOut)) != ""

	var cmd *exec.Cmd
	if active {
		cmd = exec.Command("sudo", "wg-quick", "down", "wg0")
	} else {
		cmd = exec.Command("sudo", "wg-quick", "up", "wg0")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": strings.TrimSpace(string(out))})
	}
	return c.JSON(fiber.Map{"success": true, "output": strings.TrimSpace(string(out))})
}

func wireguardRestartHandler(c *fiber.Ctx) error {
	out1, err1 := exec.Command("sudo", "wg-quick", "down", "wg0").CombinedOutput()
	out2, err2 := exec.Command("sudo", "wg-quick", "up", "wg0").CombinedOutput()
	if err1 != nil || err2 != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":  "Error reiniciando WireGuard (requiere sudo NOPASSWD)",
			"down":   strings.TrimSpace(string(out1)),
			"up":     strings.TrimSpace(string(out2)),
			"downOk": err1 == nil,
			"upOk":   err2 == nil,
		})
	}
	return c.JSON(fiber.Map{"success": true})
}

func wireguardConfigHandler(c *fiber.Ctx) error {
	var req struct {
		Config string `json:"config"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}
	if strings.TrimSpace(req.Config) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "config requerido (texto completo wg0.conf)"})
	}

	user := c.Locals("user").(*User)
	userID := user.ID

	result := configureWireGuard(req.Config, user.Username)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("WireGuard configurado (usuario: %s)", user.Username), "wireguard", &userID)
		return c.JSON(result)
	}

	if errorMsg, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error configurando WireGuard: %s (usuario: %s)", errorMsg, user.Username), "wireguard", &userID)
		return c.Status(500).JSON(fiber.Map{"error": errorMsg})
	}

	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}

// Handlers de AdBlock
func adblockStatusHandler(c *fiber.Ctx) error {
	result := getAdBlockStatus()
	return c.JSON(result)
}

func adblockEnableHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)
	userID := user.ID

	result := enableAdBlock(user.Username)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("AdBlock habilitado (usuario: %s)", user.Username), "adblock", &userID)
		return c.JSON(result)
	}

	if errorMsg, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error habilitando AdBlock: %s (usuario: %s)", errorMsg, user.Username), "adblock", &userID)
		return c.Status(500).JSON(fiber.Map{"error": errorMsg})
	}

	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}

func adblockDisableHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)
	userID := user.ID

	result := disableAdBlock(user.Username)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("AdBlock deshabilitado (usuario: %s)", user.Username), "adblock", &userID)
		return c.JSON(result)
	}

	if errorMsg, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error deshabilitando AdBlock: %s (usuario: %s)", errorMsg, user.Username), "adblock", &userID)
		return c.Status(500).JSON(fiber.Map{"error": errorMsg})
	}

	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}

// Handler de logs del sistema
// Handlers de páginas web
func networkPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "network", fiber.Map{
		"Title": T(c, "network.title", "Network Management"),
	})
}

func wifiPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "wifi", fiber.Map{
		"Title":         T(c, "wifi.overview", "WiFi Overview"),
		"wifi_stats":    fiber.Map{},
		"wifi_status":   fiber.Map{},
		"wifi_config":   fiber.Map{},
		"guest_network": fiber.Map{},
		"last_update":   time.Now().Unix(),
	})
}

func wifiScanPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "wifi_scan", fiber.Map{
		"Title": T(c, "wifi.scan", "WiFi Scan"),
	})
}

func vpnPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "vpn", fiber.Map{
		"Title":        T(c, "vpn.overview", "VPN Overview"),
		"vpn_stats":    fiber.Map{},
		"vpn_status":   fiber.Map{},
		"vpn_config":   fiber.Map{},
		"vpn_security": fiber.Map{},
		"last_update":  time.Now().Unix(),
	})
}

func wireguardPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "wireguard", fiber.Map{
		"Title":            T(c, "wireguard.overview", "WireGuard Overview"),
		"wireguard_stats":  fiber.Map{},
		"wireguard_status": fiber.Map{},
		"wireguard_config": fiber.Map{},
		"last_update":      time.Now().Unix(),
	})
}

func adblockPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "adblock", fiber.Map{
		"Title":          T(c, "adblock.overview", "AdBlock Overview"),
		"adblock_stats":  fiber.Map{},
		"adblock_status": fiber.Map{},
		"adblock_config": fiber.Map{},
	})
}

func hostapdPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "hostapd", fiber.Map{
		"Title":          T(c, "hostapd.overview", "Hotspot Overview"),
		"hostapd_stats":  fiber.Map{},
		"hostapd_status": fiber.Map{},
		"hostapd_config": fiber.Map{},
		"last_update":    time.Now().Unix(),
	})
}

func profilePageHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)
	// Actividad real: últimos logs
	logs, _, _ := GetLogs("all", 10, 0)
	type activity struct {
		Action      string
		Timestamp   string
		Description string
		IPAddress   string
	}
	var activities []activity
	for _, l := range logs {
		activities = append(activities, activity{
			Action:      l.Source,
			Timestamp:   l.CreatedAt.Format(time.RFC3339),
			Description: l.Message,
			IPAddress:   "-",
		})
	}

	configs, _ := GetAllConfigs()
	configsJSON, _ := json.Marshal(configs)
	return renderTemplate(c, "profile", fiber.Map{
		"Title": T(c, "auth.profile", "Profile"),
		"user":  user,
		"recent_activities": activities,
		"settings":          configs,
		"settings_json":     string(configsJSON),
		"last_update":       time.Now().Unix(),
	})
}

func systemPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "system", fiber.Map{
		"Title": T(c, "system.title", "System Manager"),
	})
}

func monitoringPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "monitoring", fiber.Map{
		"Title": T(c, "monitoring.title", "Monitoring"),
	})
}

func updatePageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "update", fiber.Map{
		"Title": T(c, "update.title", "Updates"),
	})
}

func firstLoginPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "first_login", fiber.Map{
		"Title": T(c, "auth.first_login", "First Login"),
	})
}

func systemLogsHandler(c *fiber.Ctx) error {
	level := c.Query("level", "all")
	limitStr := c.Query("limit", "20")
	offsetStr := c.Query("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	logs, total, err := GetLogs(level, limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"logs":  logs,
		"total": total,
		"limit": limit,
		"offset": offset,
	})
}

// systemServicesHandler devuelve el estado de los servicios principales del proyecto
func systemServicesHandler(c *fiber.Ctx) error {
	services := make(map[string]interface{})
	
	// Verificar WireGuard
	wgOut, _ := exec.Command("wg", "show").CombinedOutput()
	wgActive := strings.TrimSpace(string(wgOut)) != ""
	services["wireguard"] = map[string]interface{}{
		"status": wgActive,
		"active": wgActive,
	}
	
	// Verificar OpenVPN
	openvpnOut, _ := exec.Command("sh", "-c", "systemctl is-active openvpn 2>/dev/null || pgrep openvpn > /dev/null && echo active || echo inactive").CombinedOutput()
	openvpnStatus := strings.TrimSpace(string(openvpnOut))
	openvpnActive := openvpnStatus == "active"
	services["openvpn"] = map[string]interface{}{
		"status": openvpnStatus,
		"active": openvpnActive,
	}
	
	// Verificar HostAPD
	// Primero verificar si el proceso está corriendo
	pgrepOut, _ := exec.Command("sh", "-c", "pgrep hostapd > /dev/null 2>&1 && echo active || echo inactive").CombinedOutput()
	pgrepStatus := strings.TrimSpace(string(pgrepOut))
	
	// Luego verificar el estado de systemd (running)
	hostapdOut, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null || echo inactive").CombinedOutput()
	hostapdStatus := strings.TrimSpace(string(hostapdOut))
	
	// Verificar si el servicio está habilitado para iniciar al arranque (enabled)
	hostapdEnabledOut, _ := exec.Command("sh", "-c", "systemctl is-enabled hostapd 2>/dev/null || echo disabled").CombinedOutput()
	hostapdEnabledStatus := strings.TrimSpace(string(hostapdEnabledOut))
	hostapdEnabled := hostapdEnabledStatus == "enabled"
	
	// El servicio está activo si systemd dice "active" o si el proceso está corriendo
	hostapdActive := hostapdStatus == "active" || pgrepStatus == "active"
	
	// Usar el estado de systemd como estado principal, pero si el proceso está corriendo, considerarlo activo
	if hostapdStatus == "inactive" && pgrepStatus == "active" {
		hostapdStatus = "active"
	}
	
	services["hostapd"] = map[string]interface{}{
		"status":  hostapdStatus,
		"active":  hostapdActive,
		"enabled": hostapdEnabled,
	}
	
	// Verificar AdBlock (dnsmasq o pihole)
	dnsmasqOut, _ := exec.Command("sh", "-c", "systemctl is-active dnsmasq 2>/dev/null || echo inactive").CombinedOutput()
	dnsmasqStatus := strings.TrimSpace(string(dnsmasqOut))
	piholeOut, _ := exec.Command("sh", "-c", "systemctl is-active pihole-FTL 2>/dev/null || echo inactive").CombinedOutput()
	piholeStatus := strings.TrimSpace(string(piholeOut))
	adblockActive := dnsmasqStatus == "active" || piholeStatus == "active"
	services["adblock"] = map[string]interface{}{
		"status": adblockActive,
		"active": adblockActive,
		"type": func() string {
			if dnsmasqStatus == "active" {
				return "dnsmasq"
			}
			if piholeStatus == "active" {
				return "pihole"
			}
			return "none"
		}(),
	}
	
	return c.JSON(fiber.Map{
		"services": services,
	})
}
