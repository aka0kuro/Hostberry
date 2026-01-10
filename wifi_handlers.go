package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// scanWiFiNetworks escanea redes WiFi disponibles (reemplaza wifi_scan.lua)
func scanWiFiNetworks(interfaceName string) map[string]interface{} {
	result := make(map[string]interface{})
	networks := []map[string]interface{}{}

	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	// Asegurar que la interfaz esté activa
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second)

	// Escanear redes usando iw (obtener salida completa sin filtrar)
	scanCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo iw dev %s scan 2>/dev/null", interfaceName))
	scanOut, err := scanCmd.Output()
	if err != nil {
		log.Printf("Error escaneando WiFi: %v", err)
		result["success"] = false
		result["error"] = fmt.Sprintf("Error escaneando redes: %v", err)
		result["networks"] = networks
		return result
	}

	// Parsear salida completa de iw scan
	lines := strings.Split(string(scanOut), "\n")
	currentNetwork := make(map[string]interface{})
	seenNetworks := make(map[string]bool) // Para evitar duplicados
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Detectar inicio de nuevo BSS (nueva red)
		if strings.HasPrefix(line, "BSS ") {
			// Guardar red anterior si existe y tiene SSID
			if len(currentNetwork) > 0 {
				if ssid, ok := currentNetwork["ssid"].(string); ok && ssid != "" {
					// Evitar duplicados basándose en SSID
					if !seenNetworks[ssid] {
						seenNetworks[ssid] = true
						networks = append(networks, currentNetwork)
					} else {
						// Si ya existe, mantener el que tiene mejor señal
						for i, net := range networks {
							if existingSSID, ok := net["ssid"].(string); ok && existingSSID == ssid {
								currentSignal := 0
								existingSignal := 0
								if s, ok := currentNetwork["signal"].(int); ok {
									currentSignal = s
								}
								if s, ok := net["signal"].(int); ok {
									existingSignal = s
								}
								// Si la señal actual es mejor (más alta, menos negativa), reemplazar
								if currentSignal > existingSignal {
									networks[i] = currentNetwork
								}
								break
							}
						}
					}
				}
			}
			// Iniciar nueva red
			currentNetwork = make(map[string]interface{})
			currentNetwork["security"] = "Open" // Por defecto
			currentNetwork["signal"] = 0
		} else if strings.HasPrefix(line, "SSID:") {
			// Extraer SSID
			ssid := strings.TrimPrefix(line, "SSID:")
			ssid = strings.TrimSpace(ssid)
			if ssid != "" {
				currentNetwork["ssid"] = ssid
			}
		} else if strings.Contains(line, "signal:") {
			// Extraer señal - formato: "signal: -45.00 dBm" o "signal: -45 dBm"
			re := regexp.MustCompile(`signal:\s*(-?\d+\.?\d*)\s*dBm?`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				if signalNum, err := strconv.ParseFloat(matches[1], 64); err == nil {
					// Asegurar que sea negativo
					if signalNum > 0 {
						signalNum = -signalNum
					}
					// Validar rango razonable
					if signalNum >= -100 && signalNum <= -30 {
						currentNetwork["signal"] = int(signalNum)
					} else {
						log.Printf("Señal fuera de rango: %.2f dBm", signalNum)
					}
				}
			} else {
				// Intentar parseo alternativo sin "dBm"
				re2 := regexp.MustCompile(`signal:\s*(-?\d+\.?\d*)`)
				matches2 := re2.FindStringSubmatch(line)
				if len(matches2) > 1 {
					if signalNum, err := strconv.ParseFloat(matches2[1], 64); err == nil {
						if signalNum > 0 {
							signalNum = -signalNum
						}
						if signalNum >= -100 && signalNum <= -30 {
							currentNetwork["signal"] = int(signalNum)
						}
					}
				}
			}
		} else if strings.Contains(line, "freq:") {
			// Extraer frecuencia y convertir a canal
			re := regexp.MustCompile(`freq:\s*(\d+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				if freq, err := strconv.Atoi(matches[1]); err == nil {
					var channel int
					if freq >= 2412 && freq <= 2484 {
						// 2.4 GHz
						channel = (freq-2412)/5 + 1
					} else if freq >= 5000 && freq <= 5825 {
						// 5 GHz
						channel = (freq - 5000) / 5
					} else if freq >= 5955 && freq <= 7115 {
						// 6 GHz
						channel = (freq - 5955) / 5
					}
					if channel > 0 {
						currentNetwork["channel"] = channel
					}
				}
			}
		} else if strings.Contains(line, "RSN:") {
			// RSN (Robust Security Network) indica WPA2 o WPA3
			if strings.Contains(line, "WPA3") || strings.Contains(line, "SAE") || strings.Contains(line, "suite-B") {
				currentNetwork["security"] = "WPA3"
			} else {
				currentNetwork["security"] = "WPA2"
			}
		} else if strings.Contains(line, "WPA:") {
			// WPA indica WPA2 (WPA1 es raro)
			currentNetwork["security"] = "WPA2"
		} else if strings.Contains(line, "capability:") {
			// Detectar si tiene Privacy (WEP o protegida)
			if strings.Contains(line, "Privacy") {
				// Solo establecer WEP si no se ha detectado otra seguridad
				if sec, ok := currentNetwork["security"].(string); !ok || sec == "Open" || sec == "" {
					currentNetwork["security"] = "WEP"
				}
			}
		}
	}

	// Agregar última red si existe
	if len(currentNetwork) > 0 {
		if ssid, ok := currentNetwork["ssid"].(string); ok && ssid != "" {
			if !seenNetworks[ssid] {
				seenNetworks[ssid] = true
				networks = append(networks, currentNetwork)
			}
		}
	}

	result["success"] = true
	result["networks"] = networks
	result["count"] = len(networks)

	return result
}

// connectWiFi conecta a una red WiFi (reemplaza wifi_connect.lua)
func connectWiFi(ssid, password, interfaceName, country, user string) map[string]interface{} {
	result := make(map[string]interface{})

	if ssid == "" {
		result["success"] = false
		result["error"] = "SSID requerido"
		return result
	}

	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}
	if country == "" {
		country = DefaultCountryCode
	}
	if user == "" {
		user = "unknown"
	}

	log.Printf("Conectando a WiFi: %s (usuario: %s) usando wpa_supplicant", ssid, user)

	// Verificar si NetworkManager está gestionando la conexión activa
	nmActiveCmd := exec.Command("sh", "-c", "nmcli -t -f STATE general status 2>/dev/null | head -1")
	nmActiveOut, _ := nmActiveCmd.Output()
	nmConnected := false
	if strings.TrimSpace(string(nmActiveOut)) == "connected" || strings.TrimSpace(string(nmActiveOut)) == "connecting" {
		nmConnected = true
		log.Printf("NetworkManager está gestionando una conexión activa, no se detendrá para preservar la sesión")
	}

	// Solo detener NetworkManager si NO está gestionando una conexión activa
	if !nmConnected {
		log.Printf("Deteniendo NetworkManager para evitar conflictos con wpa_supplicant")
		executeCommand("sudo systemctl stop NetworkManager 2>/dev/null || true")
	} else {
		log.Printf("NetworkManager permanece activo para mantener la conexión actual")
	}

	// Si hostapd está corriendo, NO lo detenemos automáticamente porque puede cortar la sesión (AP).
	// En ese caso devolvemos un error accionable.
	hostapdRunning, _ := exec.Command("sh", "-c", "pgrep hostapd 2>/dev/null").Output()
	if strings.TrimSpace(string(hostapdRunning)) != "" {
		log.Printf("hostapd está corriendo; no se detendrá automáticamente para preservar la sesión")
		result["success"] = false
		result["error"] = "WiFi AP (hostapd) está activo. Desactiva el modo AP antes de conectar a una red WiFi."
		return result
	}

	// Asegurar que la interfaz esté en modo managed
	iwInfoCmd := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s info 2>/dev/null", interfaceName))
	if iwInfoOut, err := iwInfoCmd.Output(); err == nil {
		if strings.Contains(string(iwInfoOut), "type AP") {
			// No cambiar el tipo automáticamente porque puede tumbar el AP y la sesión.
			log.Printf("Interfaz está en modo AP; no se cambiará automáticamente para preservar la sesión")
			result["success"] = false
			result["error"] = "La interfaz WiFi está en modo AP. Desactiva el modo AP antes de conectar como cliente."
			return result
		}
	}

	// Asegurar que la interfaz esté activa y no bloqueada
	executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo ip link set %s down 2>/dev/null", interfaceName))
	time.Sleep(2 * time.Second)
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null", interfaceName))
	time.Sleep(2 * time.Second)

	// Asegurar que el directorio de configuración existe
	executeCommand("sudo mkdir -p /var/wpa_supplicant")

	// Sanitizar SSID para usarlo en el nombre del archivo
	safeSSID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(ssid, "_")

	// Crear archivo de configuración wpa_supplicant usando el SSID
	// Usamos /var/wpa_supplicant y el nombre del SSID como solicitado
	wpaConfigPath := fmt.Sprintf("/var/wpa_supplicant/wpa_supplicant-%s.conf", safeSSID)
	log.Printf("Creando archivo de configuración en: %s", wpaConfigPath)

	// Eliminar archivo existente para evitar problemas de permisos si fue creado por root anteriormente
	executeCommand(fmt.Sprintf("sudo rm -f %s", wpaConfigPath))

	// Generar bloque de red usando wpa_passphrase
	var networkBlock string
	if password != "" {
		// Verificar que wpa_passphrase esté disponible
		checkCmd := exec.Command("sh", "-c", "which wpa_passphrase 2>/dev/null || echo 'not found'")
		checkOut, _ := checkCmd.Output()
		if strings.Contains(string(checkOut), "not found") {
			log.Printf("ERROR: wpa_passphrase no está instalado en el sistema")
			result["success"] = false
			result["error"] = "wpa_passphrase no está disponible. Instala el paquete wpa_supplicant"
			return result
		}
		
		// Usar exec.Command directamente para evitar problemas de escaping en shell
		cmd := exec.Command("wpa_passphrase", ssid, password)
		passphraseOutBytes, err := cmd.Output()
		passphraseOut := string(passphraseOutBytes)
		if err != nil || !strings.Contains(passphraseOut, "network=") {
			log.Printf("ERROR: wpa_passphrase falló. SSID: %s, Password length: %d", ssid, len(password))
			log.Printf("ERROR: Salida: %s", passphraseOut)
			log.Printf("ERROR: Error: %v", err)
			result["success"] = false
			result["error"] = fmt.Sprintf("Error al generar la clave PSK: %s", strings.TrimSpace(passphraseOut))
			return result
		}
		networkBlock = strings.TrimSpace(passphraseOut)
	} else {
		// Red abierta
		networkBlock = fmt.Sprintf("network={\n\tssid=\"%s\"\n\tkey_mgmt=NONE\n}", ssid)
	}
	// Crear contenido completo del archivo de configuración
	configContent := fmt.Sprintf("ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev\nctrl_interface_group=netdev\nupdate_config=1\ncountry=%s\n\n%s\n", country, networkBlock)

	// Escribir el archivo de configuración de manera robusta
	// Usamos sudo sh -c "cat > ..." para escribir directamente como root
	// Esto evita problemas de namespaces (mv), stdout (tee) y permisos de archivo existente
	
	// 1. Intentar limpiar archivo existente
	exec.Command("sudo", "rm", "-f", wpaConfigPath).Run()

	// 2. Escribir contenido
	writeCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", wpaConfigPath))
	writeCmd.Stdin = strings.NewReader(configContent)
	
	if out, err := writeCmd.CombinedOutput(); err != nil {
		log.Printf("CRITICAL ERROR: Falló escritura con sudo cat: %v", err)
		log.Printf("Output: %s", string(out))
		result["success"] = false
		result["error"] = fmt.Sprintf("Error al guardar configuración: %v", err)
		return result
	}

	// Asegurar permisos correctos (solo lectura/escritura para root)
	executeCommand(fmt.Sprintf("sudo chown root:root %s", wpaConfigPath))
	executeCommand(fmt.Sprintf("sudo chmod 600 %s", wpaConfigPath))

	log.Printf("Archivo de configuración creado exitosamente en %s", wpaConfigPath)

	// Asegurar que el directorio del socket existe con permisos y grupo correctos
	executeCommand("sudo mkdir -p /var/run/wpa_supplicant 2>/dev/null || true")
	executeCommand("sudo mkdir -p /run/wpa_supplicant 2>/dev/null || true")
	executeCommand("sudo chmod 775 /var/run/wpa_supplicant 2>/dev/null || true")
	executeCommand("sudo chmod 775 /run/wpa_supplicant 2>/dev/null || true")
	executeCommand("sudo chgrp netdev /var/run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant 2>/dev/null || true")
	executeCommand("sudo chgrp netdev /run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant 2>/dev/null || true")

	// Verificar si wpa_supplicant está corriendo para esta interfaz y detenerlo si es necesario
	wpaPid, _ := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
	if strings.TrimSpace(string(wpaPid)) != "" {
		log.Printf("Deteniendo wpa_supplicant existente para %s", interfaceName)
		executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
		time.Sleep(2 * time.Second)
	}

	// Limpiar sockets existentes
	executeCommand(fmt.Sprintf("sudo rm -rf /var/run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
	executeCommand(fmt.Sprintf("sudo rm -rf /run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))

	// Iniciar wpa_supplicant con el archivo de configuración
	ctrlDir := "/run/wpa_supplicant"
	if _, err := os.Stat(ctrlDir); err != nil {
		ctrlDir = "/var/run/wpa_supplicant"
	}
	startCmd := fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext -C %s", interfaceName, wpaConfigPath, ctrlDir)
	startOut, _ := executeCommand(startCmd)
	log.Printf("wpa_supplicant start output: %s", strings.TrimSpace(startOut))

	// Verificar que se inició correctamente
	wpaPid, _ = exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
	if strings.TrimSpace(string(wpaPid)) == "" {
		log.Printf("ERROR: wpa_supplicant no se inició correctamente")
		result["success"] = false
		result["error"] = "No se pudo iniciar wpa_supplicant"
		return result
	}
	log.Printf("wpa_supplicant iniciado correctamente")

	// Esperar a que el socket esté listo
	time.Sleep(2 * time.Second)

	// Ajustar permisos del socket
	socketPath := fmt.Sprintf("%s/%s", ctrlDir, interfaceName)
	executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath))
	executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath, socketPath))

	// Verificar que wpa_cli puede comunicarse
	wpaCliCmd := fmt.Sprintf("sudo wpa_cli -i %s -p %s", interfaceName, ctrlDir)
	pingCmd := fmt.Sprintf("%s ping", wpaCliCmd)
	pingOut, _ := executeCommand(pingCmd)
	if !strings.Contains(pingOut, "PONG") {
		log.Printf("ERROR: wpa_cli no responde después de iniciar wpa_supplicant")
		result["success"] = false
		result["error"] = "wpa_cli no puede comunicarse con wpa_supplicant"
		return result
	}
	log.Printf("wpa_cli responde correctamente")

	// Conectar a la red (debería estar configurada en el archivo)
	selectCmd := fmt.Sprintf("%s select_network 0", wpaCliCmd)
	selectResult, _ := executeCommand(selectCmd)
	log.Printf("select_network result: %s", strings.TrimSpace(selectResult))

	// Habilitar la red
	enableCmd := fmt.Sprintf("%s enable_network 0", wpaCliCmd)
	enableResult, _ := executeCommand(enableCmd)
	log.Printf("enable_network result: %s", strings.TrimSpace(enableResult))

	// Reconectar explícitamente
	reconnectOut, _ := executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
	log.Printf("reconnect result: %s", strings.TrimSpace(reconnectOut))
	time.Sleep(2 * time.Second)

	// Verificar el estado de la conexión (con múltiples intentos mejorados)
	connected := false
	statusOutput := ""
	maxAttempts := 15 // Aumentado de 8 a 15
// ...
	lastState := ""
	authFailures := 0
	
	// Asegurar que result siempre tenga success y error inicializados
	if _, hasSuccess := result["success"]; !hasSuccess {
		result["success"] = false
	}
	if _, hasError := result["error"]; !hasError {
		result["error"] = ""
	}

	for attempt := 0; attempt < maxAttempts && !connected; attempt++ {
		time.Sleep(2 * time.Second) // Reducido de 3 a 2 segundos para más intentos
		statusCmd := fmt.Sprintf("%s status", wpaCliCmd)
		statusOutput, _ = executeCommand(statusCmd)
		statusStr := strings.TrimSpace(statusOutput)
		log.Printf("Connection status (attempt %d/%d): %s", attempt+1, maxAttempts, statusStr)

		// Extraer wpa_state
		re := regexp.MustCompile(`wpa_state=([^\r\n]+)`)
		matches := re.FindStringSubmatch(statusOutput)
		currentState := ""
		if len(matches) > 1 {
			currentState = strings.TrimSpace(matches[1])
		}

		// Detectar errores de autenticación
		if strings.Contains(statusOutput, "WRONG_KEY") || 
		   strings.Contains(statusOutput, "AUTH_FAIL") ||
		   strings.Contains(statusOutput, "4WAY_HANDSHAKE_TIMEOUT") {
			authFailures++
			log.Printf("ERROR: Fallo de autenticación detectado (intento %d)", authFailures)
			if authFailures >= 3 {
				result["success"] = false
				result["error"] = "Contraseña incorrecta o red no compatible"
				result["message"] = fmt.Sprintf("Error de autenticación conectando a %s", ssid)
				result["output"] = statusOutput
				return result
			}
		}

		// Verificar si está conectado
		if strings.Contains(statusOutput, "wpa_state=COMPLETED") {
			// Verificar que el SSID coincida
			if strings.Contains(statusOutput, fmt.Sprintf("ssid=%s", ssid)) {
				connected = true
				log.Printf("✅ WiFi conectado exitosamente: %s (intento %d)", ssid, attempt+1)
				break
			} else {
				// Extraer SSID actual
				ssidRe := regexp.MustCompile(`ssid=([^\r\n]+)`)
				ssidMatches := ssidRe.FindStringSubmatch(statusOutput)
				if len(ssidMatches) > 1 {
					log.Printf("⚠️  wpa_state=COMPLETED pero SSID no coincide. Conectado a: %s, esperado: %s", 
						strings.TrimSpace(ssidMatches[1]), ssid)
				} else {
					log.Printf("⚠️  wpa_state=COMPLETED pero no se pudo extraer SSID")
				}
			}
		} else if currentState != "" {
			// Log de progreso
			if currentState != lastState {
				log.Printf("Estado cambiado: %s -> %s", lastState, currentState)
				lastState = currentState
			}

			// Estados intermedios que indican progreso
			if currentState == "ASSOCIATING" || currentState == "ASSOCIATED" || 
			   currentState == "4WAY_HANDSHAKE" || currentState == "GROUP_HANDSHAKE" {
				log.Printf("Progreso de conexión: %s", currentState)
			}

			// Estados de error
			if currentState == "DISCONNECTED" || currentState == "INACTIVE" {
				if attempt < maxAttempts-1 {
					log.Printf("Reintentando conexión... (estado: %s)", currentState)
					// Deshabilitar y volver a habilitar la red
					executeCommand(fmt.Sprintf("%s disable_network %s", wpaCliCmd, "0"))
					time.Sleep(1 * time.Second)
					executeCommand(fmt.Sprintf("%s enable_network %s", wpaCliCmd, "0"))
					executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
				}
			}
		}

		// Si no hay progreso después de varios intentos, intentar reconectar
		if attempt > 5 && !connected && (currentState == "DISCONNECTED" || currentState == "INACTIVE" || currentState == "") {
			log.Printf("Sin progreso después de %d intentos, forzando reconexión...", attempt+1)
			executeCommand(fmt.Sprintf("%s disable_network all", wpaCliCmd))
			time.Sleep(1 * time.Second)
			executeCommand(fmt.Sprintf("%s enable_network %s", wpaCliCmd, "0"))
			executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
		}
	}

	if connected {
		// Esperar más tiempo para que se establezca la IP
		log.Printf("wpa_supplicant reporta conexión, esperando IP...")
		ipObtained := false
		ipWaitAttempts := 10

		for ipWait := 0; ipWait < ipWaitAttempts && !ipObtained; ipWait++ {
			time.Sleep(2 * time.Second)
			ipCheckCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", interfaceName))
			if ipOut, err := ipCheckCmd.Output(); err == nil {
				ip := strings.TrimSpace(string(ipOut))
				if ip != "" && ip != "N/A" {
					ipObtained = true
					log.Printf("IP obtenida: %s", ip)
					result["success"] = true
					result["message"] = fmt.Sprintf("Conectado a %s (IP: %s)", ssid, ip)
					result["output"] = statusOutput
					result["ip"] = ip
					log.Printf("WiFi conectado exitosamente: %s con IP %s", ssid, ip)
				} else {
					ipWait++
					log.Printf("Esperando IP... (intento %d/%d)", ipWait, ipWaitAttempts)
					// Verificar si hay un proceso DHCP corriendo
					dhcpCheck := exec.Command("sh", "-c", fmt.Sprintf("ps aux | grep -E '[d]hclient|udhcpc' | grep %s", interfaceName))
					if dhcpOut, _ := dhcpCheck.Output(); len(dhcpOut) == 0 {
						// No hay DHCP corriendo, intentar iniciarlo
						log.Printf("No hay DHCP corriendo, intentando obtener IP...")
						executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>/dev/null || sudo udhcpc -i %s 2>/dev/null || true", interfaceName, interfaceName))
					}
				}
			}
		}

		if !ipObtained {
			log.Printf("WARNING: Conectado a WiFi pero sin IP después de %d segundos", ipWaitAttempts*2)
			result["success"] = true
			result["message"] = fmt.Sprintf("Conectado a %s (obteniendo IP...)", ssid)
			result["output"] = statusOutput
			result["warning"] = "Conectado pero sin IP asignada aún"
		}
	} else {
		// Extraer información de error más detallada
		errorMsg := fmt.Sprintf("No se pudo establecer la conexión después de %d intentos", maxAttempts)
		
		// Intentar obtener más información del estado
		re := regexp.MustCompile(`wpa_state=([^\r\n]+)`)
		matches := re.FindStringSubmatch(statusOutput)
		if len(matches) > 1 {
			state := strings.TrimSpace(matches[1])
			if state == "DISCONNECTED" {
				errorMsg = "La conexión se desconectó. Verifica que la red esté disponible y la contraseña sea correcta."
			} else if state == "4WAY_HANDSHAKE" || state == "GROUP_HANDSHAKE" {
				errorMsg = "Error durante el handshake de autenticación. Verifica la contraseña."
			} else if state == "ASSOCIATING" || state == "ASSOCIATED" {
				errorMsg = "No se pudo completar la asociación con la red. Verifica que la red esté disponible."
			} else if state != "" {
				errorMsg = fmt.Sprintf("Estado: %s. No se pudo completar la conexión.", state)
			}
		}

		// Verificar si hay mensajes de error específicos
		if strings.Contains(statusOutput, "WRONG_KEY") {
			errorMsg = "Contraseña incorrecta"
		} else if strings.Contains(statusOutput, "AUTH_FAIL") {
			errorMsg = "Error de autenticación. Verifica la contraseña."
		} else if strings.Contains(statusOutput, "TIMEOUT") {
			errorMsg = "Timeout esperando respuesta de la red. Verifica que la red esté disponible."
		}

		result["success"] = false
		result["error"] = errorMsg
		result["message"] = fmt.Sprintf("Error conectando a %s", ssid)
		result["output"] = statusOutput
		log.Printf("❌ ERROR: Error conectando WiFi: %s - Estado final: %s", ssid, statusOutput)
	}

	// Asegurar que siempre se retorne un resultado válido con success y error
	if _, hasSuccess := result["success"]; !hasSuccess {
		log.Printf("⚠️  WARNING: result no tiene 'success', estableciendo a false")
		result["success"] = false
	}
	if _, hasError := result["error"]; !hasError {
		if result["success"] == false {
			log.Printf("⚠️  WARNING: result no tiene 'error' pero success es false, estableciendo mensaje genérico")
			result["error"] = "Error desconocido al conectar a la red WiFi"
		} else {
			result["error"] = ""
		}
	}
	
	log.Printf("📤 Retornando resultado: success=%v, error=%v", result["success"], result["error"])
	return result
}

// toggleWiFi habilita o deshabilita WiFi (reemplaza wifi_toggle.lua)
func toggleWiFi(interfaceName string, enable bool) map[string]interface{} {
	result := make(map[string]interface{})

	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	if enable {
		// Habilitar WiFi
		executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
		executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
		result["success"] = true
		result["message"] = "WiFi habilitado"
		result["enabled"] = true
	} else {
		// Deshabilitar WiFi
		executeCommand("sudo rfkill block wifi 2>/dev/null || true")
		executeCommand(fmt.Sprintf("sudo ip link set %s down 2>/dev/null || true", interfaceName))
		result["success"] = true
		result["message"] = "WiFi deshabilitado"
		result["enabled"] = false
	}

	return result
}
