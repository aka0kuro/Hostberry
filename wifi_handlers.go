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
// Directorio alternativo persistente si /etc es de solo lectura (no se borra al reiniciar)
const WpaSupplicantAltConfigDir = "/var/lib/hostberry/wpa_supplicant"

// Variables para directorios de socket (se determinan dinámicamente)
var activeRunDir string

// getRunDir retorna el directorio de socket activo (escribible)
func getRunDir() string {
	if activeRunDir != "" {
		return activeRunDir
	}
	// Intentar determinar el directorio de socket escribible
	candidates := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	for _, dir := range candidates {
		// Verificar que el directorio existe o puede crearse
		if _, err := os.Stat(dir); err == nil {
			// Verificar que es escribible intentando crear un archivo temporal
			testFile := fmt.Sprintf("%s/.test_write", dir)
			if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
				os.Remove(testFile)
				activeRunDir = dir
				log.Printf("Directorio de socket seleccionado (escribible): %s", activeRunDir)
				return activeRunDir
			} else {
				log.Printf("Directorio %s no es escribible: %v", dir, err)
			}
		} else {
			// Intentar crear el directorio
			if err := os.MkdirAll(dir, 0755); err == nil {
				// Verificar que es escribible
				testFile := fmt.Sprintf("%s/.test_write", dir)
				if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
					os.Remove(testFile)
					activeRunDir = dir
					log.Printf("Directorio de socket creado y seleccionado: %s", activeRunDir)
					return activeRunDir
				}
			}
		}
	}
	// Fallback: usar /tmp que siempre debería ser escribible
	activeRunDir = "/tmp/wpa_supplicant"
	os.MkdirAll(activeRunDir, 0755)
	log.Printf("Usando directorio de socket por defecto (fallback): %s", activeRunDir)
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
	
	// Intento agresivo con killall si sigue corriendo
	executeCommand("sudo killall wpa_supplicant 2>/dev/null || true")

	// Esperar a que termine con verificación
	for i := 0; i < 5; i++ {
		checkCmd := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName))
		if out, _ := checkCmd.Output(); strings.TrimSpace(string(out)) == "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
		// Si en el último intento sigue vivo, kill -9
		if i == 4 {
			log.Printf("Forzando cierre de wpa_supplicant (kill -9)...")
			executeCommand(fmt.Sprintf("sudo pkill -9 -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
			executeCommand("sudo killall -9 wpa_supplicant 2>/dev/null || true")
		}
	}

	// Limpiar sockets en todos los posibles directorios
	for _, dir := range []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"} {
		// Eliminar archivo de socket específico
		executeCommand(fmt.Sprintf("sudo rm -f %s/%s 2>/dev/null || true", dir, interfaceName))
		// Eliminar directorio si está vacío (opcional, pero ayuda a limpiar)
		// executeCommand(fmt.Sprintf("sudo rmdir %s 2>/dev/null || true", dir))
	}
}

// startWpaSupplicant inicia wpa_supplicant con la configuración dada
func startWpaSupplicant(interfaceName, configPath, runDir string) error {
	if runDir == "" {
		runDir = "/run/wpa_supplicant"
	}
	log.Printf("Iniciando wpa_supplicant para %s con config %s (runDir: %s)", interfaceName, configPath, runDir)

	// Asegurar que el directorio de socket exista con permisos correctos (usar sudo)
	executeCommand(fmt.Sprintf("sudo mkdir -p %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chmod 775 %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", runDir))
	// Limpiar socket stale si existe
	executeCommand(fmt.Sprintf("sudo rm -f %s/%s 2>/dev/null || true", runDir, interfaceName))

	// Verificar que el archivo de configuración existe
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("archivo de configuración no existe: %s", configPath)
	}

	// Buscar la ruta completa de wpa_supplicant
	wpaSupplicantPath := ""
	possiblePaths := []string{
		"/usr/sbin/wpa_supplicant",
		"/sbin/wpa_supplicant",
		"/usr/bin/wpa_supplicant",
		"/bin/wpa_supplicant",
	}
	
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			wpaSupplicantPath = path
			break
		}
	}
	
	// Si no se encontró en las rutas estándar, intentar con which
	if wpaSupplicantPath == "" {
		whichCmd := exec.Command("sh", "-c", "which wpa_supplicant 2>/dev/null")
		if whichOut, err := whichCmd.Output(); err == nil {
			wpaSupplicantPath = strings.TrimSpace(string(whichOut))
		}
	}
	
	if wpaSupplicantPath == "" {
		return fmt.Errorf("wpa_supplicant no se encontró en el sistema. Instala el paquete wpa_supplicant")
	}
	
	log.Printf("Usando wpa_supplicant en: %s", wpaSupplicantPath)
	
	// Verificar que el ejecutable existe y es ejecutable
	if fi, err := os.Stat(wpaSupplicantPath); err != nil || fi.Mode()&0111 == 0 {
		return fmt.Errorf("wpa_supplicant no es ejecutable en %s", wpaSupplicantPath)
	}
	
	// Iniciar wpa_supplicant usando exec.Command directamente para mejor control
	// -B: background
	// -i: interfaz
	// -c: archivo de configuración
	// -D: driver (nl80211 primero, luego wext como fallback)
	args := []string{wpaSupplicantPath, "-B", "-i", interfaceName, "-c", configPath, "-D", "nl80211,wext"}
	if runDir != "" {
		args = append(args, "-C", runDir)
	}
	startCmd := exec.Command("sudo", args...)
	startCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	startOut, startErr := startCmd.CombinedOutput()
	if startErr != nil {
		outStr := string(startOut)
		log.Printf("Error iniciando wpa_supplicant: %v, output: %s", startErr, outStr)
		// Verificar si el error es por permisos o por el ejecutable
		if strings.Contains(outStr, "not found") || strings.Contains(outStr, "No such file") {
			return fmt.Errorf("wpa_supplicant no se encontró en %s. Verifica la instalación", wpaSupplicantPath)
		}
		// Si el socket ya existe, limpiar y reintentar una vez
		if strings.Contains(outStr, "ctrl_iface exists") || strings.Contains(outStr, "cannot override it") {
			log.Printf("Socket de control en uso, limpiando y reintentando...")
			executeCommand(fmt.Sprintf("sudo rm -f %s/%s 2>/dev/null || true", runDir, interfaceName))
			retryCmd := exec.Command("sudo", args...)
			retryCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
			retryOut, retryErr := retryCmd.CombinedOutput()
			if retryErr != nil {
				return fmt.Errorf("error iniciando wpa_supplicant tras limpiar socket: %v, output: %s", retryErr, string(retryOut))
			}
		} else {
			return fmt.Errorf("error iniciando wpa_supplicant: %v, output: %s", startErr, outStr)
		}
	}

	log.Printf("Comando wpa_supplicant ejecutado, output: %s", strings.TrimSpace(string(startOut)))

	// Esperar a que el proceso esté corriendo
	time.Sleep(2 * time.Second)

	// Verificar que está corriendo usando múltiples métodos
	pidFound := false
	var pid string
	
	// Método 1: pgrep por nombre de proceso
	pidCmd := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName))
	if pidOut, err := pidCmd.Output(); err == nil {
		pid = strings.TrimSpace(string(pidOut))
		if pid != "" {
			pidFound = true
			log.Printf("wpa_supplicant encontrado con pgrep, PID: %s", pid)
		}
	}
	
	// Método 2: pgrep por nombre de archivo
	if !pidFound {
		pidCmd2 := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f '%s.*%s'", wpaSupplicantPath, interfaceName))
		if pidOut2, err2 := pidCmd2.Output(); err2 == nil {
			pid = strings.TrimSpace(string(pidOut2))
			if pid != "" {
				pidFound = true
				log.Printf("wpa_supplicant encontrado con pgrep (método 2), PID: %s", pid)
			}
		}
	}
	
	// Método 3: ps aux | grep
	if !pidFound {
		psCmd := exec.Command("sh", "-c", fmt.Sprintf("ps aux | grep '[w]pa_supplicant.*%s' | awk '{print $2}' | head -1", interfaceName))
		if psOut, err := psCmd.Output(); err == nil {
			pid = strings.TrimSpace(string(psOut))
			if pid != "" {
				pidFound = true
				log.Printf("wpa_supplicant encontrado con ps, PID: %s", pid)
			}
		}
	}
	
	if !pidFound {
		log.Printf("Warning: wpa_supplicant no se encontró corriendo después de iniciarlo")
		log.Printf("Verificando si hay errores en los logs del sistema...")
		// Intentar ver los últimos logs de wpa_supplicant
		dmesgCmd := exec.Command("sh", "-c", "dmesg | tail -20 | grep -i wpa || echo 'No hay mensajes de wpa en dmesg'")
		if dmesgOut, err := dmesgCmd.Output(); err == nil {
			log.Printf("Últimos mensajes de dmesg relacionados con wpa: %s", string(dmesgOut))
		}
		return fmt.Errorf("wpa_supplicant no se inició correctamente o se detuvo inmediatamente")
	}

	log.Printf("wpa_supplicant corriendo con PID: %s", pid)
	return nil
}

// waitForWpaCliConnection espera a que wpa_cli pueda comunicarse con wpa_supplicant
func waitForWpaCliConnection(interfaceName string, maxAttempts int) (string, error) {
	log.Printf("Esperando conexión con wpa_cli para %s...", interfaceName)

	// Intentar encontrar el socket en todos los posibles directorios
	socketDirs := []string{}
	if activeRunDir != "" {
		socketDirs = append(socketDirs, activeRunDir)
	}
	socketDirs = append(socketDirs, "/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant")
	// Dedupe simple
	seen := map[string]bool{}
	uniqueDirs := []string{}
	for _, dir := range socketDirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		uniqueDirs = append(uniqueDirs, dir)
	}

	var workingSocketDir string
	var lastPingOutput string
	var lastStatusOutput string
	var lastPingErr error
	var lastStatusErr error

	runWpaCli := func(args ...string) (string, error) {
		cmd := exec.Command("sudo", args...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		workingSocketDir = ""
		for _, dir := range uniqueDirs {
			socketPath := fmt.Sprintf("%s/%s", dir, interfaceName)
			if _, err := os.Stat(socketPath); err == nil {
				log.Printf("Socket encontrado en: %s", socketPath)
				workingSocketDir = dir
				// Ajustar permisos del socket
				executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath))
				executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", socketPath))
			}

			// Intentar ping aunque el socket no se haya detectado aún
			pingOut, pingErr := runWpaCli("wpa_cli", "-i", interfaceName, "-p", dir, "ping")
			lastPingOutput = pingOut
			lastPingErr = pingErr
			if lastPingOutput != "" {
				log.Printf("wpa_cli ping (%s): %s", dir, lastPingOutput)
			}
			if pingErr != nil && lastPingOutput != "" {
				log.Printf("wpa_cli ping error (%s): %v", dir, pingErr)
			}
			if strings.Contains(lastPingOutput, "PONG") {
				log.Printf("wpa_cli respondió correctamente desde %s", dir)
				return dir, nil
			}

			// Fallback: intentar status (algunas builds responden aquí aunque ping falle)
			statusOut, statusErr := runWpaCli("wpa_cli", "-i", interfaceName, "-p", dir, "status")
			lastStatusOutput = statusOut
			lastStatusErr = statusErr
			if lastStatusOutput != "" {
				log.Printf("wpa_cli status (%s): %s", dir, lastStatusOutput)
			}
			if statusErr != nil && lastStatusOutput != "" {
				log.Printf("wpa_cli status error (%s): %v", dir, statusErr)
			}
			if strings.Contains(lastStatusOutput, "wpa_state=") {
				log.Printf("wpa_cli respondió con status válido desde %s", dir)
				return dir, nil
			}

			// Fallback 2: intentar interfaz global si existe
			globalSocket := fmt.Sprintf("%s/global", dir)
			if _, err := os.Stat(globalSocket); err == nil {
				globalPingOut, globalPingErr := runWpaCli("wpa_cli", "-g", dir, "-i", interfaceName, "ping")
				if strings.TrimSpace(globalPingOut) != "" {
					log.Printf("wpa_cli global ping (%s): %s", dir, strings.TrimSpace(globalPingOut))
				}
				if globalPingErr == nil && strings.Contains(globalPingOut, "PONG") {
					log.Printf("wpa_cli respondió correctamente desde interfaz global en %s", dir)
					return dir, nil
				}
				globalStatusOut, globalStatusErr := runWpaCli("wpa_cli", "-g", dir, "-i", interfaceName, "status")
				if strings.TrimSpace(globalStatusOut) != "" {
					log.Printf("wpa_cli global status (%s): %s", dir, strings.TrimSpace(globalStatusOut))
				}
				if globalStatusErr == nil && strings.Contains(globalStatusOut, "wpa_state=") {
					log.Printf("wpa_cli respondió con status válido desde interfaz global en %s", dir)
					return dir, nil
				}
			}
		}

		if workingSocketDir != "" {
			log.Printf("Intento %d/%d: wpa_cli sin PONG aún (socket detectado en %s)", attempt+1, maxAttempts, workingSocketDir)
		} else {
			log.Printf("Intento %d/%d: Socket no encontrado aún", attempt+1, maxAttempts)
		}

		time.Sleep(1 * time.Second)
	}

	if lastPingOutput != "" || lastStatusOutput != "" {
		return "", fmt.Errorf("wpa_cli no pudo comunicarse con wpa_supplicant después de %d intentos (último ping: %s, error: %v; último status: %s, error: %v)", maxAttempts, lastPingOutput, lastPingErr, lastStatusOutput, lastStatusErr)
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

	// En modo AP+STA según TheWalrus (Raspberry Pi 3 B+), NO desactivamos hostapd ni eliminamos ap0
	// Ambos pueden funcionar simultáneamente: ap0 como AP y wlan0 como STA
	// Solo verificamos que wlan0 esté en modo managed (no AP)
	hostapdRunning, _ := exec.Command("sh", "-c", "pgrep hostapd 2>/dev/null").Output()
	if strings.TrimSpace(string(hostapdRunning)) != "" {
		log.Printf("hostapd está corriendo (modo AP+STA); manteniéndolo activo...")
		log.Printf("En modo AP+STA, ap0 funciona como AP y wlan0 como STA simultáneamente")
	}

	// Detener wpa_supplicant administrado por systemd si está activo (evita conflictos con nuestra instancia)
	log.Printf("Verificando wpa_supplicant gestionado por systemd...")
	executeCommand("sudo systemctl stop wpa_supplicant 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo systemctl stop wpa_supplicant@%s 2>/dev/null || true", interfaceName))
	executeCommand("sudo systemctl disable wpa_supplicant 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo systemctl disable wpa_supplicant@%s 2>/dev/null || true", interfaceName))

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
		log.Printf("NetworkManager está gestionando una conexión activa; intentando liberar %s...", interfaceName)
		// Forzar a NM a dejar de gestionar la interfaz WiFi para evitar conflictos con DHCP
		executeCommand(fmt.Sprintf("sudo nmcli dev disconnect %s 2>/dev/null || true", interfaceName))
		executeCommand(fmt.Sprintf("sudo nmcli dev set %s managed no 2>/dev/null || true", interfaceName))
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

	// Paso 4: Detener wpa_supplicant existente y limpiar sockets antiguos
	log.Printf("Paso 4: Deteniendo wpa_supplicant existente...")
	stopWpaSupplicant(interfaceName)
	
	// Limpiar sockets antiguos que puedan estar bloqueando (redundante pero seguro)
	socketDirs := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	for _, socketDir := range socketDirs {
		socketFile := fmt.Sprintf("%s/%s", socketDir, interfaceName)
		// Ejecutar rm ciegamente sin verificar os.Stat para evitar problemas de permisos
		executeCommand(fmt.Sprintf("sudo rm -f %s 2>/dev/null || true", socketFile))
		// También limpiar cualquier archivo en el directorio
		executeCommand(fmt.Sprintf("sudo rm -f %s/* 2>/dev/null || true", socketDir))
	}
	
	// Resetear el directorio activo para forzar una nueva verificación
	activeRunDir = ""

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
	// ctrl_interface apunta al directorio de socket (debe ser escribible)
	// Resetear el directorio activo para forzar una nueva verificación después de limpiar sockets
	activeRunDir = ""
	runDir := getRunDir()
	log.Printf("Usando directorio de socket escribible: %s", runDir)
	
	// Asegurar que el directorio de socket existe y es escribible
	if err := os.MkdirAll(runDir, 0755); err != nil {
		log.Printf("Warning: No se pudo crear directorio de socket %s: %v", runDir, err)
		// Intentar usar /tmp como último recurso
		runDir = "/tmp/wpa_supplicant"
		if err := os.MkdirAll(runDir, 0755); err != nil {
			log.Printf("ERROR: No se pudo crear directorio de socket en /tmp: %v", err)
			result["error"] = "No se pudo crear directorio de socket para wpa_supplicant"
			return result
		}
		activeRunDir = runDir
		log.Printf("Usando directorio de socket alternativo: %s", runDir)
	}
	
	// Verificar que el directorio es realmente escribible
	testFile := fmt.Sprintf("%s/.test_write", runDir)
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		log.Printf("ERROR: Directorio de socket %s no es escribible: %v", runDir, err)
		// Intentar usar /tmp como último recurso
		runDir = "/tmp/wpa_supplicant"
		os.MkdirAll(runDir, 0755)
		activeRunDir = runDir
		log.Printf("Cambiando a directorio de socket alternativo: %s", runDir)
	} else {
		os.Remove(testFile)
		log.Printf("Directorio de socket verificado como escribible: %s", runDir)
	}

	// Asegurar permisos del directorio de socket (para wpa_supplicant y wpa_cli)
	executeCommand(fmt.Sprintf("sudo mkdir -p %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chmod 775 %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", runDir))
	
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
	cpOutStr := string(cpOut)
	
	// Si falla por sistema de solo lectura, intentar remontar o usar directorio alternativo
	if cpErr != nil {
		cpOutLower := strings.ToLower(cpOutStr)
		if strings.Contains(cpOutLower, "read-only") || strings.Contains(cpOutLower, "readonly") {
			log.Printf("ERROR detectado (sistema de solo lectura): %s", cpOutStr)
			log.Printf("Sistema de archivos de solo lectura detectado, intentando remontar...")
			remountCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/")
			remountCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
			remountOut, remountErr := remountCmd.CombinedOutput()
			if remountErr != nil {
				log.Printf("No se pudo remontar como lectura-escritura: %v, output: %s", remountErr, string(remountOut))
				// Intentar usar directorio alternativo persistente (no se borra al reiniciar)
				log.Printf("Usando directorio alternativo persistente: %s", WpaSupplicantAltConfigDir)
				// Crear directorio padre primero si no existe
				parentDir := "/var/lib/hostberry"
				mkdirParentCmd := exec.Command("sudo", "mkdir", "-p", parentDir)
				mkdirParentCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				if mkdirParentOut, mkdirParentErr := mkdirParentCmd.CombinedOutput(); mkdirParentErr != nil {
					log.Printf("Warning: No se pudo crear directorio padre %s: %v, output: %s", parentDir, mkdirParentErr, string(mkdirParentOut))
					// Intentar remontar /var primero
					remountVarCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/var")
					remountVarCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
					remountVarCmd.Run() // Intentar remontar
					// Intentar crear de nuevo
					if mkdirParentOut2, mkdirParentErr2 := mkdirParentCmd.CombinedOutput(); mkdirParentErr2 != nil {
						log.Printf("ERROR: No se pudo crear directorio padre incluso después de remontar: %v, output: %s", mkdirParentErr2, string(mkdirParentOut2))
						os.Remove(tmpConfigFile)
						result["error"] = fmt.Sprintf("Error al guardar configuración: no se pudo crear directorio alternativo")
						return result
					}
				}
				// Crear directorio alternativo con sudo
				mkdirAltCmd := exec.Command("sudo", "mkdir", "-p", WpaSupplicantAltConfigDir)
				mkdirAltCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				if mkdirOut, mkdirErr := mkdirAltCmd.CombinedOutput(); mkdirErr != nil {
					log.Printf("ERROR: No se pudo crear directorio alternativo: %v, output: %s", mkdirErr, string(mkdirOut))
					os.Remove(tmpConfigFile)
					result["error"] = fmt.Sprintf("Error al guardar configuración: no se pudo crear directorio alternativo")
					return result
				}
				// Establecer permisos del directorio alternativo
				exec.Command("sudo", "chmod", "755", WpaSupplicantAltConfigDir).Run()
				exec.Command("sudo", "chown", "root:netdev", WpaSupplicantAltConfigDir).Run()
				
				// Actualizar ruta de configuración al directorio alternativo
				wpaConfigPath = fmt.Sprintf("%s/wpa_supplicant-%s.conf", WpaSupplicantAltConfigDir, safeSSID)
				
				// Intentar copiar de nuevo
				cpCmd2 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
				cpCmd2.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				cpOut2, cpErr2 := cpCmd2.CombinedOutput()
				if cpErr2 != nil {
					// Si también falla en /var/lib, puede que también esté en solo lectura
					// Intentar remontar /var también
					if strings.Contains(string(cpOut2), "Read-only file system") {
						log.Printf("ERROR: /var/lib también está en solo lectura, intentando remontar /var...")
						remountVarCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/var")
						remountVarCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
						if remountVarOut, remountVarErr := remountVarCmd.CombinedOutput(); remountVarErr == nil {
							log.Printf("Remontado /var como lectura-escritura, intentando copiar de nuevo...")
							cpCmd3 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
							cpCmd3.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
							if cpOut3, cpErr3 := cpCmd3.CombinedOutput(); cpErr3 != nil {
								log.Printf("ERROR: Falló escritura incluso después de remontar /var: %v, output: %s", cpErr3, string(cpOut3))
								os.Remove(tmpConfigFile)
								result["error"] = fmt.Sprintf("Error al guardar configuración: sistema de archivos de solo lectura (incluso /var)")
								return result
							}
							log.Printf("Archivo guardado exitosamente después de remontar /var: %s", wpaConfigPath)
						} else {
							log.Printf("ERROR: No se pudo remontar /var: %v, output: %s", remountVarErr, string(remountVarOut))
							log.Printf("ERROR: Falló escritura en directorio alternativo: %v, output: %s", cpErr2, string(cpOut2))
							os.Remove(tmpConfigFile)
							result["error"] = fmt.Sprintf("Error al guardar configuración: sistema de archivos de solo lectura")
							return result
						}
					} else {
						log.Printf("ERROR: Falló escritura en directorio alternativo: %v, output: %s", cpErr2, string(cpOut2))
						os.Remove(tmpConfigFile)
						result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr2)
						return result
					}
				} else {
					log.Printf("Archivo guardado en directorio alternativo persistente: %s", wpaConfigPath)
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
		} else {
			log.Printf("ERROR: Falló escritura del archivo de configuración: %v, output: %s", cpErr, string(cpOut))
			os.Remove(tmpConfigFile)
			result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr)
			return result
		}
	}
	
	// Limpiar archivo temporal
	os.Remove(tmpConfigFile)

	// Establecer permisos correctos
	executeCommand(fmt.Sprintf("sudo chmod 600 %s", wpaConfigPath))
	executeCommand(fmt.Sprintf("sudo chown root:root %s", wpaConfigPath))

	log.Printf("Archivo de configuración creado: %s", wpaConfigPath)

	// Paso 6: Iniciar wpa_supplicant
	log.Printf("Paso 6: Iniciando wpa_supplicant...")
	if err := startWpaSupplicant(interfaceName, wpaConfigPath, runDir); err != nil {
		log.Printf("ERROR: %v", err)
		result["error"] = "No se pudo iniciar wpa_supplicant. Verifica la instalación."
		return result
	}

	// Verificar que el socket de control exista
	existsOut, _ := executeCommand(fmt.Sprintf("sudo ls -l %s/%s 2>/dev/null || true", runDir, interfaceName))
	if strings.TrimSpace(existsOut) == "" {
		log.Printf("Advertencia: no se encontró socket en %s/%s tras iniciar wpa_supplicant", runDir, interfaceName)
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
	runWpaCli := func(args ...string) (string, error) {
		base := []string{"wpa_cli", "-i", interfaceName, "-p", socketDir}
		cmd := exec.Command("sudo", append(base, args...)...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	// Paso 8: Configurar y conectar a la red
	log.Printf("Paso 8: Conectando a la red...")

	// Listar redes configuradas
	listOut, listErr := runWpaCli("list_networks")
	if listErr != nil {
		log.Printf("Error listando redes: %v, output: %s", listErr, listOut)
	} else {
		log.Printf("Redes configuradas: %s", strings.TrimSpace(listOut))
	}

	// Si no hay redes, agregarlas manualmente vía wpa_cli
	lines := []string{}
	if listOut != "" {
		lines = strings.Split(listOut, "\n")
	}
	needsAdd := len(lines) <= 1
	if needsAdd {
		log.Printf("No se encontraron redes en wpa_supplicant, agregando vía wpa_cli...")
		netIDOut, netIDErr := runWpaCli("add_network")
		if netIDErr != nil || netIDOut == "" {
			result["error"] = fmt.Sprintf("Error agregando red en wpa_supplicant: %v", netIDErr)
			return result
		}
		netID := strings.TrimSpace(netIDOut)
		escape := func(v string) string {
			v = strings.ReplaceAll(v, "\\", "\\\\")
			v = strings.ReplaceAll(v, "\"", "\\\"")
			return v
		}
		ssidArg := fmt.Sprintf("\"%s\"", escape(ssid))
		if _, err := runWpaCli("set_network", netID, "ssid", ssidArg); err != nil {
			result["error"] = "Error configurando SSID en wpa_supplicant"
			return result
		}
		if password != "" {
			pskArg := fmt.Sprintf("\"%s\"", escape(password))
			if _, err := runWpaCli("set_network", netID, "psk", pskArg); err != nil {
				result["error"] = "Error configurando PSK en wpa_supplicant"
				return result
			}
		} else {
			if _, err := runWpaCli("set_network", netID, "key_mgmt", "NONE"); err != nil {
				result["error"] = "Error configurando red abierta en wpa_supplicant"
				return result
			}
		}
		runWpaCli("enable_network", netID)
		runWpaCli("select_network", netID)
	} else {
		// Habilitar la red 0 (la que acabamos de configurar)
		runWpaCli("select_network", "0")
		runWpaCli("enable_network", "0")
	}

	runWpaCli("reconnect")

	// Paso 9: Esperar conexión
	log.Printf("Paso 9: Esperando conexión...")
	connected := false
	statusOutput := ""
	maxAttempts := 20
	lastState := ""
	authFailures := 0

	for attempt := 0; attempt < maxAttempts && !connected; attempt++ {
		time.Sleep(2 * time.Second)

		statusOutput, err = runWpaCli("status")
		if err != nil && statusOutput == "" {
			log.Printf("Estado (intento %d/%d): error %v", attempt+1, maxAttempts, err)
		} else {
			log.Printf("Estado (intento %d/%d): %s", attempt+1, maxAttempts, strings.TrimSpace(statusOutput))
		}

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
				// Intentar DHCP de forma agresiva (limpiar y solicitar)
				executeCommand(fmt.Sprintf("sudo dhclient -r %s 2>/dev/null || true", interfaceName))
				executeCommand(fmt.Sprintf("sudo pkill -f 'dhclient.*%s|udhcpc.*%s' 2>/dev/null || true", interfaceName, interfaceName))
				executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>/dev/null || sudo udhcpc -i %s -q -n 2>/dev/null || true", interfaceName, interfaceName))
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
