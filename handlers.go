package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func translateLoginError(c *fiber.Ctx, err error) string {
	var le *LoginError
	if errors.As(err, &le) {
		msg := T(c, le.Key, le.Default)
		if len(le.Args) > 0 {
			msg = strings.ReplaceAll(msg, "{minutes}", fmt.Sprint(le.Args[0]))
			msg = strings.ReplaceAll(msg, "{duration}", fmt.Sprint(le.Args[0]))
		}
		return msg
	}
	return err.Error()
}

func loginAPIHandler(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": T(c, "errors.invalid_data", "Invalid data"),
		})
	}

	if err := ValidateUsername(req.Username); err != nil {
		return err
	}

	if req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": T(c, "auth.password_required", "Password is required"),
		})
	}

	user, token, err := Login(req.Username, req.Password)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": translateLoginError(c, err),
		})
	}

	userID := user.ID
	InsertLog("INFO", fmt.Sprintf("Usuario %s inició sesión", user.Username), "auth", &userID)

	passwordChangeRequired := user.LoginCount == 1

	cookieExpiry := time.Duration(appConfig.Security.TokenExpiry) * time.Minute
	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   int(cookieExpiry.Seconds()), // Expira al mismo tiempo que el token
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
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": T(c, "auth.unauthorized", "Unauthorized")})
	}
	userID := user.ID
	InsertLog("INFO", fmt.Sprintf("Usuario %s cerró sesión", user.Username), "auth", &userID)

	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		MaxAge:   -1,
	})

	return c.JSON(fiber.Map{
		"message": T(c, "auth.logout_success", "Logout successful"),
	})
}

func meHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": T(c, "auth.unauthorized", "Unauthorized")})
	}
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
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": T(c, "errors.invalid_data", "Invalid data")})
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": T(c, "auth.passwords_required", "Passwords required")})
	}
	if !CheckPassword(req.CurrentPassword, user.Password) {
		return c.Status(401).JSON(fiber.Map{"error": T(c, "auth.incorrect_current_password", "Current password is incorrect")})
	}

	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": T(c, "errors.server_error", "Internal server error")})
	}
	user.Password = hashed
	if err := db.Save(user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": T(c, "errors.server_error", "Internal server error")})
	}

	userID := user.ID
	InsertLog("INFO", fmt.Sprintf("Usuario %s cambió su contraseña", user.Username), "auth", &userID)
	return c.JSON(fiber.Map{"message": T(c, "auth.password_changed", "Password changed successfully")})
}

func firstLoginChangeAPIHandler(c *fiber.Ctx) error {
	tokenString := c.Get("Authorization")
	if tokenString != "" {
		tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	} else {
		tokenString = c.Cookies("access_token")
	}

	if tokenString == "" {
		return c.Status(401).JSON(fiber.Map{
			"error": T(c, "auth.token_required", "Token required"),
		})
	}

	claims, err := ValidateToken(tokenString)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": T(c, "auth.invalid_token", "Invalid token"),
		})
	}

	var user User
	if err := db.Where("id = ? AND is_active = ?", claims.UserID, true).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": T(c, "auth.user_not_found", "User not found"),
		})
	}

	if user.LoginCount != 1 {
		return c.Status(403).JSON(fiber.Map{
			"error": T(c, "auth.first_login_only", "This endpoint is only available on first login"),
		})
	}

	var req struct {
		NewUsername string `json:"new_username"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}

	if req.NewUsername != "" {
		if err := ValidateUsername(req.NewUsername); err != nil {
			return err
		}
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

	if req.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "La nueva contraseña es requerida",
		})
	}
	if err := ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Error hasheando contraseña",
		})
	}
	user.Password = hashed

	user.LoginCount++

	if err := db.Save(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Error guardando credenciales",
		})
	}

	userID := user.ID
	InsertLog("INFO", "Usuario cambió credenciales en primer login: "+user.Username, "auth", &userID)

	// Generar nuevo token con las credenciales actualizadas y dejar al usuario logueado
	newToken, err := GenerateToken(&user)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Error generando sesión",
		})
	}
	cookieExpiry := time.Duration(appConfig.Security.TokenExpiry) * time.Minute
	c.Cookie(&fiber.Cookie{
		Name:     "access_token",
		Value:    newToken,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   int(cookieExpiry.Seconds()),
	})

	return c.JSON(fiber.Map{
		"message":      T(c, "auth.credentials_updated_redirect", "Credenciales actualizadas. Redirigiendo al dashboard."),
		"access_token": newToken,
		"user": fiber.Map{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
		},
	})
}

func updateProfileAPIHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}

	var req struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Timezone  string `json:"timezone"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}

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
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}

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

func systemInfoHandler(c *fiber.Ctx) error {
	result := getSystemInfo()
	return c.JSON(result)
}

func systemShutdownHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
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

func networkStatusHandler(c *fiber.Ctx) error {
	result := getNetworkStatus()
	return c.JSON(result)
}

// librespeedCLIPath nombres posibles del binario LibreSpeed (speedtest-cli)
var librespeedCLIPath = []string{"librespeed-cli", "librespeed-cli-go", "/usr/bin/librespeed-cli", "/usr/local/bin/librespeed-cli"}

func networkSpeedtestHandler(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 120*time.Second)
	defer cancel()
	var bin string
	for _, p := range librespeedCLIPath {
		if path, err := exec.LookPath(p); err == nil {
			bin = path
			break
		}
	}
	if bin == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "LibreSpeed CLI no instalado. Instálalo desde https://github.com/librespeed/speedtest-cli",
		})
	}
	cmd := exec.CommandContext(ctx, bin, "--json", "--telemetry-level", "disabled")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return c.Status(408).JSON(fiber.Map{"success": false, "error": "Timeout del test de velocidad"})
		}
		return c.JSON(fiber.Map{
			"success": false,
			"error":   strings.TrimSpace(string(out)),
		})
	}
	// La salida puede tener líneas de log y una línea JSON; buscar la línea que empieza con {
	lines := strings.Split(string(out), "\n")
	var raw []byte
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") {
			raw = []byte(line)
			break
		}
	}
	if len(raw) == 0 {
		raw = out
	}
	var result struct {
		Timestamp      string  `json:"timestamp"`
		Ping           float64 `json:"ping"`
		Jitter         float64 `json:"jitter"`
		Download       float64 `json:"download"`
		Upload         float64 `json:"upload"`
		BytesSent      int64   `json:"bytes_sent"`
		BytesReceived  int64   `json:"bytes_received"`
		Server         struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"server"`
		Client struct {
			IP       string `json:"ip"`
			Org      string `json:"org"`
			Country  string `json:"country"`
			City     string `json:"city"`
		} `json:"client"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return c.JSON(fiber.Map{"success": false, "error": "No se pudo interpretar la salida de LibreSpeed: " + err.Error()})
	}
	return c.JSON(fiber.Map{
		"success":         true,
		"ping_ms":         result.Ping,
		"jitter_ms":       result.Jitter,
		"download_mbps":   result.Download,
		"upload_mbps":     result.Upload,
		"bytes_sent":      result.BytesSent,
		"bytes_received":  result.BytesReceived,
		"server_name":     result.Server.Name,
		"server_url":      result.Server.URL,
		"client_ip":       result.Client.IP,
		"client_org":      result.Client.Org,
		"client_country":  result.Client.Country,
		"client_city":     result.Client.City,
		"timestamp":       result.Timestamp,
	})
}

func networkInterfacesHandler(c *fiber.Ctx) error {
	result := getNetworkInterfaces()
	if result != nil {
		if interfaces, ok := result["interfaces"]; ok {
			if interfacesArray, ok := interfaces.([]map[string]interface{}); ok && len(interfacesArray) > 0 {
				if appConfig.Server.Debug {
					LogTf("logs.handlers_interfaces_count", len(interfacesArray))
				}
				return c.JSON(result)
			}
		}
	}

	interfaces := []map[string]interface{}{}
	
	cmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}'")
	output, err := cmd.Output()
	if err != nil {
		LogTf("logs.handlers_interfaces_error", err)
		return c.JSON(fiber.Map{"interfaces": interfaces})
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	LogTf("logs.handlers_interfaces_found", lines)
	for _, ifaceName := range lines {
		ifaceName = strings.TrimSpace(ifaceName)
		if ifaceName == "" || ifaceName == "lo" {
			continue // Saltar loopback
		}
		
		ifaceCheckCmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null", ifaceName))
		if ifaceCheckErr := ifaceCheckCmd.Run(); ifaceCheckErr != nil {
			LogTf("logs.handlers_interface_skip", ifaceName)
			continue
		}
		
		LogTf("logs.handlers_interface_processing", ifaceName)

		iface := map[string]interface{}{
			"name": ifaceName,
			"ip":   "N/A",
			"mac":  "N/A",
			"state": "unknown",
		}

		stateCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/operstate 2>/dev/null", ifaceName))
		if stateOut, err := stateCmd.Output(); err == nil {
			state := strings.TrimSpace(string(stateOut))
			if state == "" {
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
			ipStateCmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null | grep -o 'state [A-Z]*' | awk '{print $2}'", ifaceName))
			if ipStateOut, ipStateErr := ipStateCmd.Output(); ipStateErr == nil {
				state := strings.TrimSpace(string(ipStateOut))
				if state != "" {
					iface["state"] = state
				}
			}
		}
		
		if ifaceName == "ap0" {
			LogTf("logs.handlers_ap0_found", iface["state"])
			if iface["state"] == "down" || iface["state"] == "unknown" {
				LogT("logs.handlers_ap0_down")
				activateCmd := exec.Command("sh", "-c", "sudo ip link set ap0 up 2>/dev/null")
				if activateErr := activateCmd.Run(); activateErr == nil {
					time.Sleep(500 * time.Millisecond)
					stateCmd2 := exec.Command("sh", "-c", "cat /sys/class/net/ap0/operstate 2>/dev/null")
					if stateOut2, err2 := stateCmd2.Output(); err2 == nil {
						newState := strings.TrimSpace(string(stateOut2))
						if newState != "" {
							iface["state"] = newState
							LogTf("logs.handlers_ap0_activated", newState)
						}
					}
				}
			}
		}
		
		if strings.HasPrefix(ifaceName, "wlan") {
			wpaStatusCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s status 2>/dev/null | grep 'wpa_state=' | cut -d= -f2", ifaceName))
			if wpaStateOut, err := wpaStatusCmd.Output(); err == nil {
				wpaState := strings.TrimSpace(string(wpaStateOut))
				if wpaState == "COMPLETED" {
					iface["wpa_state"] = "COMPLETED"
				} else if wpaState == "ASSOCIATING" || wpaState == "ASSOCIATED" || wpaState == "4WAY_HANDSHAKE" || wpaState == "GROUP_HANDSHAKE" {
					iface["wpa_state"] = wpaState
					iface["state"] = "connecting"
				} else {
					iface["wpa_state"] = wpaState
					if iface["state"] == "up" {
						iface["state"] = "down"
					}
				}
			}
		}

		ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | head -1", ifaceName))
		if ipOut, err := ipCmd.Output(); err == nil {
			ipLine := strings.TrimSpace(string(ipOut))
			if ipLine != "" {
				parts := strings.Split(ipLine, "/")
				iface["ip"] = parts[0]
				if len(parts) > 1 {
					iface["netmask"] = parts[1]
				}
			}
		}
		
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
		
		if iface["ip"] == "N/A" || iface["ip"] == "" {
			ifconfigCmd := exec.Command("sh", "-c", fmt.Sprintf("ifconfig %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | head -1", ifaceName))
			if ifconfigOut, err := ifconfigCmd.Output(); err == nil {
				ifconfigLine := strings.TrimSpace(string(ifconfigOut))
				if ifconfigLine != "" {
					ifconfigLine = strings.TrimPrefix(ifconfigLine, "addr:")
					iface["ip"] = ifconfigLine
				}
			}
		}
		
		if iface["ip"] == "N/A" || iface["ip"] == "" {
			hostnameCmd := exec.Command("sh", "-c", "hostname -I 2>/dev/null | awk '{print $1}'")
			if hostnameOut, err := hostnameCmd.Output(); err == nil {
				hostnameIP := strings.TrimSpace(string(hostnameOut))
				if hostnameIP != "" {
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
		
		if (iface["state"] == "up" || iface["state"] == "connected" || iface["state"] == "connecting") && (iface["ip"] == "N/A" || iface["ip"] == "") {
			dhcpCheck := exec.Command("sh", "-c", fmt.Sprintf("ps aux | grep -E '[d]hclient|udhcpc' | grep %s", ifaceName))
			if dhcpOut, err := dhcpCheck.Output(); err == nil {
				dhcpLine := strings.TrimSpace(string(dhcpOut))
				if dhcpLine != "" {
					iface["ip"] = "Obtaining IP..."
				}
			}
		}
		
		gatewayCmd := exec.Command("sh", "-c", fmt.Sprintf("ip route | grep %s | grep default | awk '{print $3}' | head -1", ifaceName))
		if gatewayOut, err := gatewayCmd.Output(); err == nil {
			gateway := strings.TrimSpace(string(gatewayOut))
			if gateway != "" {
				iface["gateway"] = gateway
			}
		}
		
		if _, hasGateway := iface["gateway"]; !hasGateway {
			defaultGatewayCmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}' | head -1")
			if defaultGatewayOut, err := defaultGatewayCmd.Output(); err == nil {
				defaultGateway := strings.TrimSpace(string(defaultGatewayOut))
				if defaultGateway != "" {
					iface["gateway"] = defaultGateway
				}
			}
		}

		isAPMode := false
		if iface["ip"] != "N/A" && iface["ip"] != "" && iface["ip"] != "Obtaining IP..." {
			ipStr, ok := iface["ip"].(string)
			if !ok {
				ipStr = fmt.Sprintf("%v", iface["ip"])
			}
			gatewayStr := ""
			if iface["gateway"] != nil {
				if gw, ok := iface["gateway"].(string); ok {
					gatewayStr = gw
				} else {
					gatewayStr = fmt.Sprintf("%v", iface["gateway"])
				}
			}
			if ipStr == "192.168.4.1" || (strings.HasPrefix(ipStr, "192.168.4.") && (gatewayStr == "" || gatewayStr == "192.168.4.1")) {
				hostapdCheck := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null || pgrep hostapd > /dev/null && echo active || echo inactive")
				if hostapdOut, err := hostapdCheck.Output(); err == nil {
					if strings.TrimSpace(string(hostapdOut)) == "active" {
						isAPMode = true
						iface["ap_mode"] = true
					}
				}
			}
		}

		if strings.HasPrefix(ifaceName, "wlan") {
			if isAPMode {
				iface["connected"] = false
				iface["state"] = "ap_mode"
				iface["internet_connected"] = false
			} else if wpaState, hasWpaState := iface["wpa_state"]; hasWpaState && wpaState == "COMPLETED" {
				if iface["ip"] == "N/A" || iface["ip"] == "" || iface["ip"] == "Obtaining IP..." {
					iface["connected"] = false
					iface["state"] = "connecting"
					iface["internet_connected"] = false
				} else {
					iface["connected"] = true
					iface["state"] = "connected"
					hasInternet := false
					ipStr, ok := iface["ip"].(string)
					if !ok {
						ipStr = fmt.Sprintf("%v", iface["ip"])
					}
					
					gatewayStr := ""
					if iface["gateway"] != nil {
						if gw, ok := iface["gateway"].(string); ok {
							gatewayStr = gw
						} else {
							gatewayStr = fmt.Sprintf("%v", iface["gateway"])
						}
					}
					
					if gatewayStr == "" {
						defaultGatewayCmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}' | head -1")
						if defaultGatewayOut, err := defaultGatewayCmd.Output(); err == nil {
							defaultGateway := strings.TrimSpace(string(defaultGatewayOut))
							if defaultGateway != "" {
								gatewayStr = defaultGateway
								iface["gateway"] = defaultGateway
							}
						}
					}
					
					if !strings.HasPrefix(ipStr, "192.168.4.") && gatewayStr != "" && gatewayStr != "192.168.4.1" {
						hasInternet = true
					} else if strings.HasPrefix(ipStr, "192.168.4.") {
						hasInternet = false
					} else {
						pingCmd := exec.Command("sh", "-c", fmt.Sprintf("timeout 2 ping -c 1 -W 1 8.8.8.8 > /dev/null 2>&1 && echo 'ok' || echo 'fail'"))
						if pingOut, err := pingCmd.Output(); err == nil {
							if strings.TrimSpace(string(pingOut)) == "ok" {
								hasInternet = true
							}
						}
						if !hasInternet && !strings.HasPrefix(ipStr, "192.168.4.") && ipStr != "" {
							hasInternet = true
						}
					}
					iface["internet_connected"] = hasInternet
				}
			} else if wpaState, hasWpaState := iface["wpa_state"]; hasWpaState && (wpaState == "ASSOCIATING" || wpaState == "ASSOCIATED" || wpaState == "4WAY_HANDSHAKE" || wpaState == "GROUP_HANDSHAKE") {
				iface["connected"] = false
				iface["state"] = "connecting"
				iface["internet_connected"] = false
			} else {
				iface["connected"] = false
				iface["internet_connected"] = false
				if iface["state"] != "down" {
					iface["state"] = "down"
				}
			}
		} else {
			if iface["ip"] != "N/A" && iface["ip"] != "" && iface["ip"] != "Obtaining IP..." {
				iface["connected"] = true
				if iface["state"] == "up" {
					iface["state"] = "connected"
				}
				hasInternet := false
				ipStr, ok := iface["ip"].(string)
				if !ok {
					ipStr = fmt.Sprintf("%v", iface["ip"])
				}
				
				gatewayStr := ""
				if iface["gateway"] != nil {
					if gw, ok := iface["gateway"].(string); ok {
						gatewayStr = gw
					} else {
						gatewayStr = fmt.Sprintf("%v", iface["gateway"])
					}
				}
				
				if gatewayStr == "" {
					defaultGatewayCmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}' | head -1")
					if defaultGatewayOut, err := defaultGatewayCmd.Output(); err == nil {
						defaultGateway := strings.TrimSpace(string(defaultGatewayOut))
						if defaultGateway != "" {
							gatewayStr = defaultGateway
							iface["gateway"] = defaultGateway
						}
					}
				}
				
				if strings.HasPrefix(ipStr, "192.168.4.") {
					hasInternet = false
				} else if !strings.HasPrefix(ipStr, "192.168.4.") && ipStr != "" {
					hasInternet = true
					
					if gatewayStr != "" && gatewayStr != "192.168.4.1" {
						pingCmd := exec.Command("sh", "-c", fmt.Sprintf("timeout 2 ping -c 1 -W 1 8.8.8.8 > /dev/null 2>&1 && echo 'ok' || echo 'fail'"))
						if pingOut, err := pingCmd.Output(); err == nil {
							if strings.TrimSpace(string(pingOut)) == "ok" {
								hasInternet = true
							} else {
								hasInternet = true
							}
						}
					}
				} else {
					hasInternet = false
				}
				iface["internet_connected"] = hasInternet
			} else {
				iface["connected"] = false
				iface["internet_connected"] = false
			}
		}

		macCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/address 2>/dev/null", ifaceName))
		if macOut, err := macCmd.Output(); err == nil {
			mac := strings.TrimSpace(string(macOut))
			if mac != "" {
				iface["mac"] = mac
			}
		}

		interfaces = append(interfaces, iface)
	}

	LogTf("logs.handlers_fallback_interfaces", len(interfaces))
	return c.JSON(fiber.Map{"interfaces": interfaces})
}

func wifiConnectHandler(c *fiber.Ctx) error {
	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
		Country  string `json:"country"`
		Interface string `json:"interface"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Datos inválidos",
		})
	}

	if err := ValidateSSID(req.SSID); err != nil {
		return err
	}

	if len(req.Password) > 128 {
		return c.Status(400).JSON(fiber.Map{
			"error": "La contraseña no puede tener más de 128 caracteres",
		})
	}

	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	country := req.Country
	if country == "" {
		country = c.Query("country", DefaultCountryCode)
	}
	if country == "" {
		country = DefaultCountryCode
	}
	
	interfaceName := req.Interface
	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	if len(interfaceName) > 16 || !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(interfaceName) {
		return c.Status(400).JSON(fiber.Map{
			"error": "Nombre de interfaz inválido",
		})
	}

	result := connectWiFi(req.SSID, req.Password, interfaceName, country, user.Username)

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
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
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

func wireguardStatusHandler(c *fiber.Ctx) error {
	result := getWireGuardStatus()
	return c.JSON(result)
}

func wireguardInterfacesHandler(c *fiber.Ctx) error {
	out, err := exec.Command("wg", "show", "interfaces").CombinedOutput()
	if err != nil {
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

func wireguardGetConfigHandler(c *fiber.Ctx) error {
	out, err := exec.Command("sh", "-c", "cat /etc/wireguard/wg0.conf 2>/dev/null").CombinedOutput()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": strings.TrimSpace(string(out))})
	}
	return c.JSON(fiber.Map{"config": string(out)})
}

func wireguardToggleHandler(c *fiber.Ctx) error {
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
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	if err := ValidateWireGuardConfig(req.Config); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	config := req.Config
	return RunActionWithUser(c, "wireguard", "WireGuard configurado por usuario %s", "Error configurando WireGuard: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return configureWireGuard(config, user.Username)
	})
}

func adblockStatusHandler(c *fiber.Ctx) error {
	result := getAdBlockStatus()
	return c.JSON(result)
}

func adblockEnableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "AdBlock habilitado por usuario %s", "Error habilitando AdBlock: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return enableAdBlock(user.Username)
	})
}

func adblockDisableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "AdBlock deshabilitado por usuario %s", "Error deshabilitando AdBlock: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return disableAdBlock(user.Username)
	})
}

// Handlers para DNSCrypt
func dnscryptStatusHandler(c *fiber.Ctx) error {
	result := getDNSCryptStatus()
	return c.JSON(result)
}

func dnscryptInstallHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "DNSCrypt instalado por usuario %s", "Error instalando DNSCrypt: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return installDNSCrypt(user.Username)
	})
}

func dnscryptConfigureHandler(c *fiber.Ctx) error {
	var req struct {
		ServerName string `json:"server_name"`
		BlockAds   bool   `json:"block_ads"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	if req.ServerName == "" {
		req.ServerName = "adguard-dns"
	}
	return RunActionWithUser(c, "adblock", "DNSCrypt configurado por usuario %s", "Error configurando DNSCrypt: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return configureDNSCrypt(req.ServerName, req.BlockAds, user.Username)
	})
}

func dnscryptEnableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "DNSCrypt habilitado por usuario %s", "Error habilitando DNSCrypt: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return enableDNSCrypt(user.Username)
	})
}

func dnscryptDisableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "DNSCrypt deshabilitado por usuario %s", "Error deshabilitando DNSCrypt: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return disableDNSCrypt(user.Username)
	})
}

// Handlers para Blocky
func blockyStatusHandler(c *fiber.Ctx) error {
	result := getBlockyStatus()
	return c.JSON(result)
}

func blockyConfigHandler(c *fiber.Ctx) error {
	cfg := getBlockyConfig()
	return c.JSON(cfg)
}

func blockyInstallHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "Blocky instalado por usuario %s", "Error instalando Blocky: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return installBlocky(user.Username)
	})
}

func blockyConfigureHandler(c *fiber.Ctx) error {
	var req struct {
		Upstreams  []string `json:"upstreams"`
		BlockLists []string `json:"block_lists"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	return RunActionWithUser(c, "adblock", "Blocky configurado por usuario %s", "Error configurando Blocky: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return configureBlocky(req.Upstreams, req.BlockLists, user.Username)
	})
}

func blockyEnableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "Blocky habilitado por usuario %s", "Error habilitando Blocky: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return enableBlocky(user.Username)
	})
}

func blockyDisableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "adblock", "Blocky deshabilitado por usuario %s", "Error deshabilitando Blocky: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return disableBlocky(user.Username)
	})
}

func blockyAPIProxyHandler(c *fiber.Ctx) error {
	path := c.Params("*")
	if path == "" {
		path = c.Path()
	}
	// path puede ser "blocking/status", "lists/refresh", etc.
	method := c.Method()
	var body []byte
	if method == "POST" && c.Body() != nil {
		body = c.Body()
	}
	code, data := blockyAPIProxy(method, path, body)
	if code == 0 {
		return c.Status(502).JSON(fiber.Map{"error": "Blocky no responde. ¿Está el servicio activo?"})
	}
	c.Set("Content-Type", "application/json")
	return c.Status(code).Send(data)
}

// Handlers para Tor
func torStatusHandler(c *fiber.Ctx) error {
	result := getTorStatus()
	return c.JSON(result)
}

func torInstallHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "tor", "Tor instalado por usuario %s", "Error instalando Tor: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return installTor(user.Username)
	})
}

func torConfigureHandler(c *fiber.Ctx) error {
	var req struct {
		EnableSocks           bool `json:"enable_socks"`
		SocksPort             int  `json:"socks_port"`
		EnableControlPort     bool `json:"enable_control_port"`
		ControlPort           int  `json:"control_port"`
		EnableHiddenService   bool `json:"enable_hidden_service"`
		EnableTransPort       bool `json:"enable_trans_port"`
		TransPort             int  `json:"trans_port"`
		EnableDNSPort         bool `json:"enable_dns_port"`
		DNSPort               int  `json:"dns_port"`
		ClientOnly            bool `json:"client_only"`
		AutomapHostsOnResolve bool `json:"automap_hosts_on_resolve"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}

	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	if req.SocksPort == 0 {
		req.SocksPort = 9050
	}
	if req.ControlPort == 0 {
		req.ControlPort = 9051
	}
	if req.TransPort == 0 {
		req.TransPort = 9040
	}
	if req.DNSPort == 0 {
		req.DNSPort = 53
	}

	opts := TorConfigOptions{
		User:                  user.Username,
		EnableSocks:           req.EnableSocks,
		SocksPort:             req.SocksPort,
		EnableControlPort:     req.EnableControlPort,
		ControlPort:           req.ControlPort,
		EnableHiddenService:   req.EnableHiddenService,
		EnableTransPort:       req.EnableTransPort,
		TransPort:             req.TransPort,
		EnableDNSPort:         req.EnableDNSPort,
		DNSPort:               req.DNSPort,
		ClientOnly:            req.ClientOnly,
		AutomapHostsOnResolve: req.AutomapHostsOnResolve,
	}
	result := configureTor(opts)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("Tor configurado por usuario %s", user.Username), "tor", &userID)
		return c.JSON(result)
	}

	if errorMsg, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error configurando Tor: %s (usuario: %s)", errorMsg, user.Username), "tor", &userID)
		return c.Status(500).JSON(fiber.Map{"error": errorMsg})
	}

	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}

func torEnableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "tor", "Tor habilitado por usuario %s", "Error habilitando Tor: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return enableTor(user.Username)
	})
}

func torIptablesEnableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "tor", "Red Hostberry torificada por usuario %s", "Error torificando red: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return enableTorIptables(user.Username)
	})
}

func torIptablesDisableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "tor", "Torificación de red desactivada por usuario %s", "Error desactivando torificación: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return disableTorIptables(user.Username)
	})
}

func torDisableHandler(c *fiber.Ctx) error {
	return RunActionWithUser(c, "tor", "Tor deshabilitado por usuario %s", "Error deshabilitando Tor: %s (usuario: %s)", func(user *User) map[string]interface{} {
		return disableTor(user.Username)
	})
}

func torCircuitHandler(c *fiber.Ctx) error {
	result := getTorCircuitInfo()
	return c.JSON(result)
}

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

func torPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "tor", fiber.Map{
		"Title": T(c, "tor.title", "Tor Configuration"),
		"tor_status": getTorStatus(),
	})
}

func adblockPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "adblock", fiber.Map{
		"Title": T(c, "adblock.overview", "AdBlock (Blocky)"),
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
	user, ok := GetUser(c)
	if !ok {
		return c.Redirect("/login")
	}
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

func setupWizardPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "setup_wizard", fiber.Map{
		"Title":      T(c, "setup_wizard.title", "Configuración inicial"),
		"last_update": time.Now().Unix(),
	})
}

func setupWizardVpnPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "setup_wizard_vpn", fiber.Map{
		"Title":      T(c, "setup_wizard.security_vpn", "VPN"),
		"last_update": time.Now().Unix(),
	})
}

func setupWizardWireguardPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "setup_wizard_wireguard", fiber.Map{
		"Title":      T(c, "setup_wizard.security_wireguard", "WireGuard"),
		"last_update": time.Now().Unix(),
	})
}

func setupWizardTorPageHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "setup_wizard_tor", fiber.Map{
		"Title":      T(c, "setup_wizard.security_tor", "Tor"),
		"last_update": time.Now().Unix(),
	})
}

func systemLogsHandler(c *fiber.Ctx) error {
	level := c.Query("level", "all")
	limitStr := c.Query("limit", "20")
	offsetStr := c.Query("offset", "0")

	switch level {
	case "all", "INFO", "WARNING", "ERROR", "DEBUG":
	default:
		level = "all"
	}

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 || offset > 10000 {
		offset = 0
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

func systemServicesHandler(c *fiber.Ctx) error {
	services := make(map[string]interface{})
	
	wgOut, _ := exec.Command("wg", "show").CombinedOutput()
	wgActive := strings.TrimSpace(string(wgOut)) != ""
	services["wireguard"] = map[string]interface{}{
		"status": wgActive,
		"active": wgActive,
	}
	
	openvpnOut, _ := exec.Command("sh", "-c", "systemctl is-active openvpn 2>/dev/null || pgrep openvpn > /dev/null && echo active || echo inactive").CombinedOutput()
	openvpnStatus := strings.TrimSpace(string(openvpnOut))
	openvpnActive := openvpnStatus == "active"
	services["openvpn"] = map[string]interface{}{
		"status": openvpnStatus,
		"active": openvpnActive,
	}
	
	pgrepOut, _ := exec.Command("sh", "-c", "pgrep hostapd > /dev/null 2>&1 && echo active || echo inactive").CombinedOutput()
	pgrepStatus := strings.TrimSpace(string(pgrepOut))
	
	hostapdOut, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null || echo inactive").CombinedOutput()
	hostapdStatus := strings.TrimSpace(string(hostapdOut))
	
	hostapdEnabledOut, _ := exec.Command("sh", "-c", "systemctl is-enabled hostapd 2>/dev/null || echo disabled").CombinedOutput()
	hostapdEnabledStatus := strings.TrimSpace(string(hostapdEnabledOut))
	hostapdEnabled := hostapdEnabledStatus == "enabled"
	
	hostapdActive := hostapdStatus == "active" || pgrepStatus == "active"
	
	if hostapdStatus == "inactive" && pgrepStatus == "active" {
		hostapdStatus = "active"
	}
	
	services["hostapd"] = map[string]interface{}{
		"status":  hostapdStatus,
		"active":  hostapdActive,
		"enabled": hostapdEnabled,
	}
	
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
