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

// Directorio estándar para configuración de wpa_supplicant
const WpaSupplicantConfigDir = "/etc/wpa_supplicant"

// Variables para directorios de socket (se determinan dinámicamente)
var activeRunDir string

// getRunDir retorna el directorio de socket activo
func getRunDir() string {
	if activeRunDir != "" {
		return activeRunDir
	}
	// Intentar determinar el directorio de socket
	candidates := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			activeRunDir = dir
			return activeRunDir
		}
	}
	activeRunDir = "/run/wpa_supplicant"
	return activeRunDir
}

// ensureWpaSupplicantDirs asegura que los directorios necesarios existan con permisos correctos
func ensureWpaSupplicantDirs() error {
	// Crear directorio de configuración (generalmente siempre funciona)
	if _, err := os.Stat(WpaSupplicantConfigDir); os.IsNotExist(err) {
		log.Printf("Creando directorio de configuración: %s", WpaSupplicantConfigDir)
		cmd := exec.Command("sudo", "mkdir", "-p", WpaSupplicantConfigDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Warning: no se pudo crear %s: %v (output: %s)", WpaSupplicantConfigDir, err, string(out))
			// No es fatal, intentamos continuar
		}
	}
	exec.Command("sudo", "chmod", "755", WpaSupplicantConfigDir).Run()
	exec.Command("sudo", "chown", "root:netdev", WpaSupplicantConfigDir).Run()

	// Intentar crear directorio de socket en orden de preferencia
	runDirCandidates := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	var createdDir string

	for _, dir := range runDirCandidates {
		// Verificar si ya existe
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			log.Printf("Directorio de socket ya existe: %s", dir)
			createdDir = dir
			break
		}

		// Intentar crear
		log.Printf("Intentando crear directorio de socket: %s", dir)
		cmd := exec.Command("sudo", "mkdir", "-p", dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("No se pudo crear %s: %v (output: %s)", dir, err, string(out))
			continue
		}

		// Verificar que se creó
		if _, err := os.Stat(dir); err == nil {
			log.Printf("Directorio de socket creado: %s", dir)
			createdDir = dir
			break
		}
	}

	if createdDir == "" {
		// Último recurso: usar /tmp
		createdDir = "/tmp/wpa_supplicant"
		os.MkdirAll(createdDir, 0775)
		log.Printf("Usando directorio temporal: %s", createdDir)
	}

	// Configurar permisos
	exec.Command("sudo", "chmod", "775", createdDir).Run()
	exec.Command("sudo", "chown", "root:netdev", createdDir).Run()

	// Guardar el directorio activo
	activeRunDir = createdDir
	log.Printf("Directorio de socket activo: %s", activeRunDir)

	return nil
}

// stopWpaSupplicant detiene todas las instancias de wpa_supplicant para una interfaz
func stopWpaSupplicant(interfaceName string) {
	log.Printf("Deteniendo wpa_supplicant para interfaz %s...", interfaceName)

	// Detener por interfaz específica
	executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*-i.*%s' 2>/dev/null || true", interfaceName))
	executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))

	// Esperar a que termine
	time.Sleep(1 * time.Second)

	// Limpiar sockets en todos los posibles directorios
	for _, dir := range []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"} {
		executeCommand(fmt.Sprintf("sudo rm -f %s/%s 2>/dev/null || true", dir, interfaceName))
	}
}

// startWpaSupplicant inicia wpa_supplicant con la configuración dada
func startWpaSupplicant(interfaceName, configPath string) error {
	log.Printf("Iniciando wpa_supplicant para %s con config %s", interfaceName, configPath)

	// Verificar que el archivo de configuración existe
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("archivo de configuración no existe: %s", configPath)
	}

	// Iniciar wpa_supplicant
	// -B: background
	// -i: interfaz
	// -c: archivo de configuración
	// -D: driver (nl80211 primero, luego wext como fallback)
	startCmd := fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext", interfaceName, configPath)
	out, err := executeCommand(startCmd)
	if err != nil {
		log.Printf("Error iniciando wpa_supplicant: %v, output: %s", err, out)
		return fmt.Errorf("error iniciando wpa_supplicant: %v", err)
	}

	log.Printf("wpa_supplicant iniciado, output: %s", strings.TrimSpace(out))

	// Esperar a que el proceso esté corriendo
	time.Sleep(2 * time.Second)

	// Verificar que está corriendo
	pidCmd := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName))
	pidOut, _ := pidCmd.Output()
	if strings.TrimSpace(string(pidOut)) == "" {
		return fmt.Errorf("wpa_supplicant no se inició correctamente")
	}

	log.Printf("wpa_supplicant corriendo con PID: %s", strings.TrimSpace(string(pidOut)))
	return nil
}

// waitForWpaCliConnection espera a que wpa_cli pueda comunicarse con wpa_supplicant
func waitForWpaCliConnection(interfaceName string, maxAttempts int) (string, error) {
	log.Printf("Esperando conexión con wpa_cli para %s...", interfaceName)

	// Intentar encontrar el socket en todos los posibles directorios
	socketPaths := []string{
		fmt.Sprintf("/run/wpa_supplicant/%s", interfaceName),
		fmt.Sprintf("/var/run/wpa_supplicant/%s", interfaceName),
		fmt.Sprintf("/tmp/wpa_supplicant/%s", interfaceName),
	}
	// Agregar el directorio activo al inicio si está configurado
	if activeRunDir != "" {
		socketPaths = append([]string{fmt.Sprintf("%s/%s", activeRunDir, interfaceName)}, socketPaths...)
	}

	var workingSocketDir string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Buscar socket existente
		for _, socketPath := range socketPaths {
			if _, err := os.Stat(socketPath); err == nil {
				workingSocketDir = socketPath[:strings.LastIndex(socketPath, "/")]
				log.Printf("Socket encontrado en: %s", socketPath)

				// Ajustar permisos del socket
				executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath))
				executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", socketPath))
				break
			}
		}

		if workingSocketDir != "" {
			// Intentar ping
			pingCmd := fmt.Sprintf("sudo wpa_cli -i %s -p %s ping", interfaceName, workingSocketDir)
			pingOut, _ := executeCommand(pingCmd)
			if strings.Contains(pingOut, "PONG") {
				log.Printf("wpa_cli respondió correctamente desde %s", workingSocketDir)
				return workingSocketDir, nil
			}
			log.Printf("Intento %d/%d: wpa_cli no respondió PONG, respuesta: %s", attempt+1, maxAttempts, strings.TrimSpace(pingOut))
		} else {
			log.Printf("Intento %d/%d: Socket no encontrado aún", attempt+1, maxAttempts)
		}

		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("wpa_cli no pudo comunicarse con wpa_supplicant después de %d intentos", maxAttempts)
}

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
	result["success"] = false
	result["error"] = ""

	if ssid == "" {
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

	log.Printf("========================================")
	log.Printf("Conectando a WiFi: %s (usuario: %s)", ssid, user)
	log.Printf("Interfaz: %s, País: %s", interfaceName, country)
	log.Printf("========================================")

	// Paso 1: Asegurar que los directorios necesarios existan
	log.Printf("Paso 1: Verificando directorios...")
	if err := ensureWpaSupplicantDirs(); err != nil {
		log.Printf("ERROR: No se pudieron crear los directorios: %v", err)
		result["error"] = fmt.Sprintf("Error preparando sistema: %v", err)
		return result
	}

	// Paso 2: Verificar conflictos con otros servicios
	log.Printf("Paso 2: Verificando conflictos...")

	// Verificar si hostapd está corriendo y desactivarlo automáticamente
	hostapdRunning, _ := exec.Command("sh", "-c", "pgrep hostapd 2>/dev/null").Output()
	if strings.TrimSpace(string(hostapdRunning)) != "" {
		log.Printf("hostapd está corriendo; desactivándolo automáticamente...")
		
		// Detener hostapd y dnsmasq
		executeCommand("sudo systemctl stop hostapd 2>/dev/null || true")
		executeCommand("sudo systemctl stop dnsmasq 2>/dev/null || true")
		
		// Esperar un momento para que se detenga completamente
		time.Sleep(2 * time.Second)
		
		// Verificar que se detuvo
		hostapdCheck, _ := exec.Command("sh", "-c", "pgrep hostapd 2>/dev/null").Output()
		if strings.TrimSpace(string(hostapdCheck)) != "" {
			log.Printf("Warning: hostapd aún está corriendo después de intentar detenerlo")
			// Intentar matar el proceso
			executeCommand("sudo pkill -9 hostapd 2>/dev/null || true")
			time.Sleep(1 * time.Second)
		}
		
		log.Printf("hostapd desactivado")
	}

	// Verificar y eliminar interfaz ap0 si existe (modo AP+STA)
	ap0Check := exec.Command("sh", "-c", "ip link show ap0 2>/dev/null")
	if ap0Out, err := ap0Check.Output(); err == nil && strings.TrimSpace(string(ap0Out)) != "" {
		log.Printf("Interfaz ap0 detectada; eliminándola...")
		executeCommand("sudo iw dev ap0 del 2>/dev/null || true")
		time.Sleep(1 * time.Second)
		log.Printf("Interfaz ap0 eliminada")
	}

	// Verificar modo de la interfaz
	iwInfoCmd := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s info 2>/dev/null", interfaceName))
	if iwInfoOut, err := iwInfoCmd.Output(); err == nil {
		if strings.Contains(string(iwInfoOut), "type AP") {
			log.Printf("La interfaz %s está en modo AP; cambiándola a modo managed...", interfaceName)
			// Cambiar la interfaz de modo AP a modo managed
			executeCommand(fmt.Sprintf("sudo iw dev %s set type managed 2>/dev/null || true", interfaceName))
			time.Sleep(1 * time.Second)
			log.Printf("Interfaz %s cambiada a modo managed", interfaceName)
		}
	}

	// Verificar NetworkManager
	nmActiveCmd := exec.Command("sh", "-c", "nmcli -t -f STATE general status 2>/dev/null | head -1")
	nmActiveOut, _ := nmActiveCmd.Output()
	nmState := strings.TrimSpace(string(nmActiveOut))
	if nmState == "connected" || nmState == "connecting" {
		log.Printf("NetworkManager está gestionando una conexión activa, no se detendrá")
	} else {
		log.Printf("Deteniendo NetworkManager para evitar conflictos...")
		executeCommand("sudo systemctl stop NetworkManager 2>/dev/null || true")
	}

	// Paso 3: Preparar la interfaz
	log.Printf("Paso 3: Preparando interfaz %s...", interfaceName)
	executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo ip link set %s down 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second)
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second)

	// Paso 4: Detener wpa_supplicant existente
	log.Printf("Paso 4: Deteniendo wpa_supplicant existente...")
	stopWpaSupplicant(interfaceName)

	// Paso 5: Crear archivo de configuración
	log.Printf("Paso 5: Creando archivo de configuración...")

	// Sanitizar SSID para nombre de archivo
	safeSSID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(ssid, "_")
	wpaConfigPath := fmt.Sprintf("%s/wpa_supplicant-%s.conf", WpaSupplicantConfigDir, safeSSID)

	// Generar bloque de red
	var networkBlock string
	if password != "" {
		// Verificar que wpa_passphrase esté disponible
		checkCmd := exec.Command("sh", "-c", "which wpa_passphrase 2>/dev/null")
		checkOut, _ := checkCmd.Output()
		if strings.TrimSpace(string(checkOut)) == "" {
			result["error"] = "wpa_passphrase no está disponible. Instala el paquete wpa_supplicant"
			return result
		}

		// Generar PSK
		cmd := exec.Command("wpa_passphrase", ssid, password)
		passphraseOut, err := cmd.Output()
		if err != nil || !strings.Contains(string(passphraseOut), "network=") {
			log.Printf("ERROR: wpa_passphrase falló: %v", err)
			result["error"] = "Error al generar la clave PSK. Verifica el SSID y la contraseña."
			return result
		}
		networkBlock = strings.TrimSpace(string(passphraseOut))
	} else {
		// Red abierta
		networkBlock = fmt.Sprintf("network={\n\tssid=\"%s\"\n\tkey_mgmt=NONE\n}", ssid)
	}

	// Crear contenido del archivo de configuración
	// ctrl_interface apunta al directorio de socket
	runDir := getRunDir()
	configContent := fmt.Sprintf(`ctrl_interface=DIR=%s GROUP=netdev
ctrl_interface_group=netdev
update_config=1
country=%s

%s
`, runDir, country, networkBlock)

	log.Printf("Contenido de configuración:\n%s", configContent)

	// Eliminar archivo existente
	executeCommand(fmt.Sprintf("sudo rm -f %s", wpaConfigPath))

	// Escribir archivo de configuración usando un archivo temporal y cp (más confiable)
	tmpConfigFile := fmt.Sprintf("/tmp/wpa_supplicant_%s_%d.conf", safeSSID, time.Now().Unix())
	if err := os.WriteFile(tmpConfigFile, []byte(configContent), 0644); err != nil {
		log.Printf("ERROR: No se pudo crear archivo temporal: %v", err)
		result["error"] = fmt.Sprintf("Error al guardar configuración: %v", err)
		return result
	}
	
	// Asegurar que el directorio existe
	if _, err := os.Stat(WpaSupplicantConfigDir); os.IsNotExist(err) {
		log.Printf("Creando directorio de configuración: %s", WpaSupplicantConfigDir)
		mkdirCmd := exec.Command("sudo", "mkdir", "-p", WpaSupplicantConfigDir)
		mkdirCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
		if out, err := mkdirCmd.CombinedOutput(); err != nil {
			log.Printf("Warning: no se pudo crear %s: %v (output: %s)", WpaSupplicantConfigDir, err, string(out))
		}
		exec.Command("sudo", "chmod", "755", WpaSupplicantConfigDir).Run()
		exec.Command("sudo", "chown", "root:netdev", WpaSupplicantConfigDir).Run()
	}
	
	// Copiar archivo temporal a la ubicación final usando cp (tiene permisos en sudoers)
	cpPath := "/bin/cp"
	if _, err := os.Stat(cpPath); os.IsNotExist(err) {
		cpPath = "/usr/bin/cp"
	}
	cpCmd := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
	cpCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	cpOut, cpErr := cpCmd.CombinedOutput()
	
	// Si falla por sistema de solo lectura, intentar remontar
	if cpErr != nil && strings.Contains(string(cpOut), "Read-only file system") {
		log.Printf("Sistema de archivos de solo lectura detectado, intentando remontar...")
		remountCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/")
		remountCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
		if remountOut, remountErr := remountCmd.CombinedOutput(); remountErr != nil {
			log.Printf("No se pudo remontar como lectura-escritura: %v, output: %s", remountErr, string(remountOut))
			// Intentar usar directorio alternativo en /tmp
			altConfigDir := "/tmp/hostberry/wpa_supplicant"
			if err := os.MkdirAll(altConfigDir, 0755); err == nil {
				log.Printf("Usando directorio alternativo: %s", altConfigDir)
				wpaConfigPath = fmt.Sprintf("%s/wpa_supplicant-%s.conf", altConfigDir, safeSSID)
				// Intentar copiar de nuevo
				cpCmd2 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
				cpCmd2.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				if cpOut2, cpErr2 := cpCmd2.CombinedOutput(); cpErr2 != nil {
					log.Printf("ERROR: Falló escritura incluso en directorio alternativo: %v, output: %s", cpErr2, string(cpOut2))
					os.Remove(tmpConfigFile)
					result["error"] = fmt.Sprintf("Error al guardar configuración: sistema de archivos de solo lectura")
					return result
				}
				log.Printf("Archivo guardado en directorio alternativo: %s", wpaConfigPath)
			} else {
				log.Printf("ERROR: No se pudo crear directorio alternativo: %v", err)
				os.Remove(tmpConfigFile)
				result["error"] = fmt.Sprintf("Error al guardar configuración: sistema de archivos de solo lectura")
				return result
			}
		} else {
			log.Printf("Sistema remontado como lectura-escritura, intentando copiar de nuevo...")
			// Intentar copiar de nuevo después del remontaje
			cpCmd2 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
			cpCmd2.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
			if cpOut2, cpErr2 := cpCmd2.CombinedOutput(); cpErr2 != nil {
				log.Printf("ERROR: Falló escritura después del remontaje: %v, output: %s", cpErr2, string(cpOut2))
				os.Remove(tmpConfigFile)
				result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr2)
				return result
			}
			log.Printf("Archivo guardado exitosamente después del remontaje")
		}
	} else if cpErr != nil {
		log.Printf("ERROR: Falló escritura del archivo de configuración: %v, output: %s", cpErr, string(cpOut))
		os.Remove(tmpConfigFile)
		result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr)
		return result
	}
	
	// Limpiar archivo temporal
	os.Remove(tmpConfigFile)

	// Establecer permisos correctos
	executeCommand(fmt.Sprintf("sudo chmod 600 %s", wpaConfigPath))
	executeCommand(fmt.Sprintf("sudo chown root:root %s", wpaConfigPath))

	log.Printf("Archivo de configuración creado: %s", wpaConfigPath)

	// Paso 6: Iniciar wpa_supplicant
	log.Printf("Paso 6: Iniciando wpa_supplicant...")
	if err := startWpaSupplicant(interfaceName, wpaConfigPath); err != nil {
		log.Printf("ERROR: %v", err)
		result["error"] = "No se pudo iniciar wpa_supplicant. Verifica la instalación."
		return result
	}

	// Paso 7: Esperar conexión con wpa_cli
	log.Printf("Paso 7: Estableciendo comunicación con wpa_cli...")
	socketDir, err := waitForWpaCliConnection(interfaceName, 10)
	if err != nil {
		log.Printf("ERROR: %v", err)
		result["error"] = "wpa_cli no puede comunicarse con wpa_supplicant. Verifica permisos del socket."
		return result
	}

	wpaCliCmd := fmt.Sprintf("sudo wpa_cli -i %s -p %s", interfaceName, socketDir)

	// Paso 8: Configurar y conectar a la red
	log.Printf("Paso 8: Conectando a la red...")

	// Listar redes configuradas
	listOut, _ := executeCommand(fmt.Sprintf("%s list_networks", wpaCliCmd))
	log.Printf("Redes configuradas: %s", strings.TrimSpace(listOut))

	// Habilitar la red 0 (la que acabamos de configurar)
	executeCommand(fmt.Sprintf("%s select_network 0", wpaCliCmd))
	executeCommand(fmt.Sprintf("%s enable_network 0", wpaCliCmd))
	executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))

	// Paso 9: Esperar conexión
	log.Printf("Paso 9: Esperando conexión...")
	connected := false
	statusOutput := ""
	maxAttempts := 20
	lastState := ""
	authFailures := 0

	for attempt := 0; attempt < maxAttempts && !connected; attempt++ {
		time.Sleep(2 * time.Second)

		statusOutput, _ = executeCommand(fmt.Sprintf("%s status", wpaCliCmd))
		log.Printf("Estado (intento %d/%d): %s", attempt+1, maxAttempts, strings.TrimSpace(statusOutput))

		// Extraer wpa_state
		stateRe := regexp.MustCompile(`wpa_state=([^\r\n]+)`)
		stateMatches := stateRe.FindStringSubmatch(statusOutput)
		currentState := ""
		if len(stateMatches) > 1 {
			currentState = strings.TrimSpace(stateMatches[1])
		}

		// Detectar errores de autenticación
		if strings.Contains(statusOutput, "WRONG_KEY") ||
			strings.Contains(statusOutput, "AUTH_FAIL") ||
			strings.Contains(statusOutput, "4WAY_HANDSHAKE_TIMEOUT") {
			authFailures++
			log.Printf("Fallo de autenticación detectado (%d)", authFailures)
			if authFailures >= 3 {
				result["error"] = "Contraseña incorrecta o red no compatible"
				return result
			}
		}

		// Verificar si está conectado
		if strings.Contains(statusOutput, "wpa_state=COMPLETED") {
			connected = true
			log.Printf("✅ WiFi conectado exitosamente a %s", ssid)
			break
		}

		// Log de cambio de estado
		if currentState != "" && currentState != lastState {
			log.Printf("Estado cambiado: %s -> %s", lastState, currentState)
			lastState = currentState
		}

		// Reintentar si está desconectado
		if currentState == "DISCONNECTED" || currentState == "INACTIVE" {
			if attempt > 3 && attempt%3 == 0 {
				log.Printf("Reintentando conexión...")
				executeCommand(fmt.Sprintf("%s disconnect", wpaCliCmd))
				time.Sleep(1 * time.Second)
				executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
			}
		}
	}

	// Paso 10: Obtener IP
	if connected {
		log.Printf("Paso 10: Obteniendo dirección IP...")
		ipObtained := false
		var ip string

		for ipAttempt := 0; ipAttempt < 10 && !ipObtained; ipAttempt++ {
			time.Sleep(2 * time.Second)

			ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", interfaceName))
			ipOut, _ := ipCmd.Output()
			ip = strings.TrimSpace(string(ipOut))

			if ip != "" && ip != "N/A" && !strings.HasPrefix(ip, "169.254") {
				ipObtained = true
				log.Printf("IP obtenida: %s", ip)
			} else {
				log.Printf("Esperando IP... (intento %d/10)", ipAttempt+1)
				// Intentar DHCP si no hay proceso corriendo
				dhcpCheck := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'dhclient.*%s\\|udhcpc.*%s' 2>/dev/null", interfaceName, interfaceName))
				if dhcpOut, _ := dhcpCheck.Output(); len(dhcpOut) == 0 {
					executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>/dev/null || sudo udhcpc -i %s 2>/dev/null || true", interfaceName, interfaceName))
				}
			}
		}

		result["success"] = true
		if ipObtained {
			result["message"] = fmt.Sprintf("Conectado a %s (IP: %s)", ssid, ip)
			result["ip"] = ip
		} else {
			result["message"] = fmt.Sprintf("Conectado a %s (obteniendo IP...)", ssid)
			result["warning"] = "Conectado pero sin IP asignada aún"
		}
		log.Printf("✅ Conexión exitosa: %s", result["message"])
	} else {
		// Extraer información de error
		errorMsg := fmt.Sprintf("No se pudo conectar después de %d intentos", maxAttempts)

		stateRe := regexp.MustCompile(`wpa_state=([^\r\n]+)`)
		stateMatches := stateRe.FindStringSubmatch(statusOutput)
		if len(stateMatches) > 1 {
			state := strings.TrimSpace(stateMatches[1])
			switch state {
			case "DISCONNECTED":
				errorMsg = "La red no está disponible o la contraseña es incorrecta"
			case "4WAY_HANDSHAKE", "GROUP_HANDSHAKE":
				errorMsg = "Error de autenticación. Verifica la contraseña."
			case "ASSOCIATING", "ASSOCIATED":
				errorMsg = "No se pudo completar la asociación con la red"
			default:
				if state != "" {
					errorMsg = fmt.Sprintf("Estado final: %s. No se pudo completar la conexión.", state)
				}
			}
		}

		if strings.Contains(statusOutput, "WRONG_KEY") {
			errorMsg = "Contraseña incorrecta"
		} else if strings.Contains(statusOutput, "AUTH_FAIL") {
			errorMsg = "Error de autenticación"
		}

		result["error"] = errorMsg
		log.Printf("❌ Error de conexión: %s", errorMsg)
	}

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
