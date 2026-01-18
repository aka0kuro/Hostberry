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

const WpaSupplicantConfigDir = "/etc/wpa_supplicant"
const WpaSupplicantAltConfigDir = "/var/lib/hostberry/wpa_supplicant"

var activeRunDir string

func getRunDir() string {
	if activeRunDir != "" {
		return activeRunDir
	}
	candidates := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			testFile := fmt.Sprintf("%s/.test_write", dir)
			if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
				os.Remove(testFile)
				activeRunDir = dir
				LogTf("logs.socket_dir_selected", activeRunDir)
				return activeRunDir
			} else {
				LogTf("logs.socket_dir_not_writable", dir, err)
			}
		} else {
			if err := os.MkdirAll(dir, 0755); err == nil {
				testFile := fmt.Sprintf("%s/.test_write", dir)
				if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
					os.Remove(testFile)
					activeRunDir = dir
					LogTf("logs.socket_dir_created", activeRunDir)
					return activeRunDir
				}
			}
		}
	}
	activeRunDir = "/tmp/wpa_supplicant"
	os.MkdirAll(activeRunDir, 0755)
	LogTf("logs.socket_dir_default", activeRunDir)
	return activeRunDir
}

func ensureWpaSupplicantDirs() error {
	if _, err := os.Stat(WpaSupplicantConfigDir); os.IsNotExist(err) {
		LogTf("logs.wpa_config_dir_creating", WpaSupplicantConfigDir)
		cmd := exec.Command("sudo", "mkdir", "-p", WpaSupplicantConfigDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			LogTf("logs.wpa_config_dir_error", WpaSupplicantConfigDir, err, string(out))
		}
	}
	exec.Command("sudo", "chmod", "755", WpaSupplicantConfigDir).Run()
	exec.Command("sudo", "chown", "root:netdev", WpaSupplicantConfigDir).Run()

	runDirCandidates := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	var createdDir string

	for _, dir := range runDirCandidates {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			LogTf("logs.socket_dir_exists", dir)
			createdDir = dir
			break
		}

		LogTf("logs.socket_dir_creating", dir)
		cmd := exec.Command("sudo", "mkdir", "-p", dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			LogTf("logs.socket_dir_create_error", dir, err, string(out))
			continue
		}

		if _, err := os.Stat(dir); err == nil {
			LogTf("logs.socket_dir_created_ok", dir)
			createdDir = dir
			break
		}
	}

	if createdDir == "" {
		createdDir = "/tmp/wpa_supplicant"
		os.MkdirAll(createdDir, 0775)
		LogTf("logs.socket_dir_temp", createdDir)
	}

	exec.Command("sudo", "chmod", "775", createdDir).Run()
	exec.Command("sudo", "chown", "root:netdev", createdDir).Run()

	activeRunDir = createdDir
	LogTf("logs.socket_dir_active", activeRunDir)

	return nil
}

func stopWpaSupplicant(interfaceName string) {
	LogTf("logs.wpa_stopping", interfaceName)

	executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*-i.*%s' 2>/dev/null || true", interfaceName))
	executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
	
	executeCommand("sudo killall wpa_supplicant 2>/dev/null || true")

	for i := 0; i < 5; i++ {
		checkCmd := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName))
		if out, _ := checkCmd.Output(); strings.TrimSpace(string(out)) == "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i == 4 {
			LogT("logs.wpa_force_kill")
			executeCommand(fmt.Sprintf("sudo pkill -9 -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
			executeCommand("sudo killall -9 wpa_supplicant 2>/dev/null || true")
		}
	}

	for _, dir := range []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"} {
		executeCommand(fmt.Sprintf("sudo rm -f %s/%s 2>/dev/null || true", dir, interfaceName))
	}
}

func startWpaSupplicant(interfaceName, configPath, runDir string) error {
	if runDir == "" {
		runDir = "/run/wpa_supplicant"
	}
	LogTf("logs.wpa_starting", interfaceName, configPath, runDir)

	executeCommand(fmt.Sprintf("sudo mkdir -p %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chmod 775 %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo rm -f %s/%s 2>/dev/null || true", runDir, interfaceName))

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("archivo de configuración no existe: %s", configPath)
	}

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
	
	if wpaSupplicantPath == "" {
		whichCmd := exec.Command("sh", "-c", "which wpa_supplicant 2>/dev/null")
		if whichOut, err := whichCmd.Output(); err == nil {
			wpaSupplicantPath = strings.TrimSpace(string(whichOut))
		}
	}
	
	if wpaSupplicantPath == "" {
		return fmt.Errorf("wpa_supplicant no se encontró en el sistema. Instala el paquete wpa_supplicant")
	}
	
	LogTf("logs.wpa_path", wpaSupplicantPath)
	
	if fi, err := os.Stat(wpaSupplicantPath); err != nil || fi.Mode()&0111 == 0 {
		return fmt.Errorf("wpa_supplicant no es ejecutable en %s", wpaSupplicantPath)
	}
	
	args := []string{wpaSupplicantPath, "-B", "-i", interfaceName, "-c", configPath, "-D", "nl80211,wext"}
	if runDir != "" {
		args = append(args, "-C", runDir)
	}
	startCmd := exec.Command("sudo", args...)
	startCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	startOut, startErr := startCmd.CombinedOutput()
	if startErr != nil {
		outStr := string(startOut)
		LogTf("logs.wpa_start_error", startErr, outStr)
		if strings.Contains(outStr, "not found") || strings.Contains(outStr, "No such file") {
			return fmt.Errorf("wpa_supplicant no se encontró en %s. Verifica la instalación", wpaSupplicantPath)
		}
		if strings.Contains(outStr, "ctrl_iface exists") || strings.Contains(outStr, "cannot override it") {
			LogT("logs.wpa_socket_in_use")
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

	LogTf("logs.wpa_command_executed", strings.TrimSpace(string(startOut)))

	time.Sleep(2 * time.Second)

	pidFound := false
	var pid string
	
	pidCmd := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName))
	if pidOut, err := pidCmd.Output(); err == nil {
		pid = strings.TrimSpace(string(pidOut))
		if pid != "" {
			pidFound = true
			LogTf("logs.wpa_found_pgrep", pid)
		}
	}
	
	if !pidFound {
		pidCmd2 := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f '%s.*%s'", wpaSupplicantPath, interfaceName))
		if pidOut2, err2 := pidCmd2.Output(); err2 == nil {
			pid = strings.TrimSpace(string(pidOut2))
			if pid != "" {
				pidFound = true
				LogTf("logs.wpa_found_pgrep2", pid)
			}
		}
	}
	
	if !pidFound {
		psCmd := exec.Command("sh", "-c", fmt.Sprintf("ps aux | grep '[w]pa_supplicant.*%s' | awk '{print $2}' | head -1", interfaceName))
		if psOut, err := psCmd.Output(); err == nil {
			pid = strings.TrimSpace(string(psOut))
			if pid != "" {
				pidFound = true
				LogTf("logs.wpa_found_ps", pid)
			}
		}
	}
	
	if !pidFound {
		LogT("logs.wpa_not_running")
		LogT("logs.wpa_checking_logs")
		dmesgCmd := exec.Command("sh", "-c", "dmesg | tail -20 | grep -i wpa || echo 'No hay mensajes de wpa en dmesg'")
		if dmesgOut, err := dmesgCmd.Output(); err == nil {
			LogTf("logs.wpa_dmesg", string(dmesgOut))
		}
		return fmt.Errorf("wpa_supplicant no se inició correctamente o se detuvo inmediatamente")
	}

	LogTf("logs.wpa_running", pid)
	return nil
}

func waitForWpaCliConnection(interfaceName string, maxAttempts int) (string, error) {
	LogTf("logs.wpa_cli_waiting", interfaceName)

	socketDirs := []string{}
	if activeRunDir != "" {
		socketDirs = append(socketDirs, activeRunDir)
	}
	socketDirs = append(socketDirs, "/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant")
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
				LogTf("logs.wpa_socket_found", socketPath)
				workingSocketDir = dir
				executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath))
				executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", socketPath))
			}

			pingOut, pingErr := runWpaCli("wpa_cli", "-i", interfaceName, "-p", dir, "ping")
			lastPingOutput = pingOut
			lastPingErr = pingErr
			if lastPingOutput != "" {
				LogTf("logs.wpa_cli_ping", dir, lastPingOutput)
			}
			if pingErr != nil && lastPingOutput != "" {
				LogTf("logs.wpa_cli_ping_error", dir, pingErr)
			}
			if strings.Contains(lastPingOutput, "PONG") {
				LogTf("logs.wpa_cli_responded", dir)
				return dir, nil
			}

			statusOut, statusErr := runWpaCli("wpa_cli", "-i", interfaceName, "-p", dir, "status")
			lastStatusOutput = statusOut
			lastStatusErr = statusErr
			if lastStatusOutput != "" {
				LogTf("logs.wpa_cli_status", dir, lastStatusOutput)
			}
			if statusErr != nil && lastStatusOutput != "" {
				LogTf("logs.wpa_cli_status_error", dir, statusErr)
			}
			if strings.Contains(lastStatusOutput, "wpa_state=") {
				LogTf("logs.wpa_cli_status_valid", dir)
				return dir, nil
			}

			globalSocket := fmt.Sprintf("%s/global", dir)
			if _, err := os.Stat(globalSocket); err == nil {
				globalPingOut, globalPingErr := runWpaCli("wpa_cli", "-g", dir, "-i", interfaceName, "ping")
				if strings.TrimSpace(globalPingOut) != "" {
					LogTf("logs.wpa_cli_global_ping", dir, strings.TrimSpace(globalPingOut))
				}
				if globalPingErr == nil && strings.Contains(globalPingOut, "PONG") {
					LogTf("logs.wpa_cli_global_responded", dir)
					return dir, nil
				}
				globalStatusOut, globalStatusErr := runWpaCli("wpa_cli", "-g", dir, "-i", interfaceName, "status")
				if strings.TrimSpace(globalStatusOut) != "" {
					LogTf("logs.wpa_cli_global_status", dir, strings.TrimSpace(globalStatusOut))
				}
				if globalStatusErr == nil && strings.Contains(globalStatusOut, "wpa_state=") {
					LogTf("logs.wpa_cli_global_status_valid", dir)
					return dir, nil
				}
			}
		}

		if workingSocketDir != "" {
			LogTf("logs.wpa_cli_attempt", attempt+1, maxAttempts, workingSocketDir)
		} else {
			LogTf("logs.wpa_cli_socket_not_found", attempt+1, maxAttempts)
		}

		time.Sleep(1 * time.Second)
	}

	if lastPingOutput != "" || lastStatusOutput != "" {
		return "", fmt.Errorf("wpa_cli no pudo comunicarse con wpa_supplicant después de %d intentos (último ping: %s, error: %v; último status: %s, error: %v)", maxAttempts, lastPingOutput, lastPingErr, lastStatusOutput, lastStatusErr)
	}
	return "", fmt.Errorf("wpa_cli no pudo comunicarse con wpa_supplicant después de %d intentos", maxAttempts)
}

func scanWiFiNetworks(interfaceName string) map[string]interface{} {
	result := make(map[string]interface{})
	networks := []map[string]interface{}{}

	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second)

	scanCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo iw dev %s scan 2>/dev/null", interfaceName))
	scanOut, err := scanCmd.Output()
	if err != nil {
		LogTf("logs.wifi_scan_error", err)
		result["success"] = false
		result["error"] = fmt.Sprintf("Error escaneando redes: %v", err)
		result["networks"] = networks
		return result
	}

	lines := strings.Split(string(scanOut), "\n")
	currentNetwork := make(map[string]interface{})
	seenNetworks := make(map[string]bool) // Para evitar duplicados

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "BSS ") {
			if len(currentNetwork) > 0 {
				if ssid, ok := currentNetwork["ssid"].(string); ok && ssid != "" {
					if !seenNetworks[ssid] {
						seenNetworks[ssid] = true
						networks = append(networks, currentNetwork)
					} else {
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
								if currentSignal > existingSignal {
									networks[i] = currentNetwork
								}
								break
							}
						}
					}
				}
			}
			currentNetwork = make(map[string]interface{})
			currentNetwork["security"] = "Open" // Por defecto
			currentNetwork["signal"] = 0
		} else if strings.HasPrefix(line, "SSID:") {
			ssid := strings.TrimPrefix(line, "SSID:")
			ssid = strings.TrimSpace(ssid)
			if ssid != "" {
				currentNetwork["ssid"] = ssid
			}
		} else if strings.Contains(line, "signal:") {
			re := regexp.MustCompile(`signal:\s*(-?\d+\.?\d*)\s*dBm?`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				if signalNum, err := strconv.ParseFloat(matches[1], 64); err == nil {
					if signalNum > 0 {
						signalNum = -signalNum
					}
					if signalNum >= -100 && signalNum <= -30 {
						currentNetwork["signal"] = int(signalNum)
					} else {
						LogTf("logs.wifi_signal_out_of_range", signalNum)
					}
				}
			} else {
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
			re := regexp.MustCompile(`freq:\s*(\d+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				if freq, err := strconv.Atoi(matches[1]); err == nil {
					var channel int
					if freq >= 2412 && freq <= 2484 {
						channel = (freq-2412)/5 + 1
					} else if freq >= 5000 && freq <= 5825 {
						channel = (freq - 5000) / 5
					} else if freq >= 5955 && freq <= 7115 {
						channel = (freq - 5955) / 5
					}
					if channel > 0 {
						currentNetwork["channel"] = channel
					}
				}
			}
		} else if strings.Contains(line, "RSN:") {
			if strings.Contains(line, "WPA3") || strings.Contains(line, "SAE") || strings.Contains(line, "suite-B") {
				currentNetwork["security"] = "WPA3"
			} else {
				currentNetwork["security"] = "WPA2"
			}
		} else if strings.Contains(line, "WPA:") {
			currentNetwork["security"] = "WPA2"
		} else if strings.Contains(line, "capability:") {
			if strings.Contains(line, "Privacy") {
				if sec, ok := currentNetwork["security"].(string); !ok || sec == "Open" || sec == "" {
					currentNetwork["security"] = "WEP"
				}
			}
		}
	}

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

	if appConfig.Server.Debug { log.Printf("========================================") }
	LogTf("logs.wifi_connecting_user", ssid, user)
	LogTf("logs.wifi_interface_country_info", interfaceName, country)
	if appConfig.Server.Debug { log.Printf("========================================") }

	if appConfig.Server.Debug { log.Printf("Paso 1: Verificando directorios...") }
	if err := ensureWpaSupplicantDirs(); err != nil {
		LogTf("logs.wifi_dirs_create_error", err)
		result["error"] = fmt.Sprintf("Error preparando sistema: %v", err)
		return result
	}

	if appConfig.Server.Debug { log.Printf("Paso 2: Verificando conflictos...") }

	hostapdRunning, _ := exec.Command("sh", "-c", "pgrep hostapd 2>/dev/null").Output()
	if strings.TrimSpace(string(hostapdRunning)) != "" {
		LogT("logs.wifi_hostapd_running")
		LogT("logs.wifi_hostapd_apsta_mode")
	}

	LogT("logs.wifi_checking_systemd")
	executeCommand("sudo systemctl stop wpa_supplicant 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo systemctl stop wpa_supplicant@%s 2>/dev/null || true", interfaceName))
	executeCommand("sudo systemctl disable wpa_supplicant 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo systemctl disable wpa_supplicant@%s 2>/dev/null || true", interfaceName))

	iwInfoCmd := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s info 2>/dev/null", interfaceName))
	if iwInfoOut, err := iwInfoCmd.Output(); err == nil {
		if strings.Contains(string(iwInfoOut), "type AP") {
			LogTf("logs.wifi_interface_ap_mode", interfaceName)
			executeCommand(fmt.Sprintf("sudo iw dev %s set type managed 2>/dev/null || true", interfaceName))
			time.Sleep(1 * time.Second)
			LogTf("logs.wifi_interface_managed", interfaceName)
		}
	}

	nmActiveCmd := exec.Command("sh", "-c", "nmcli -t -f STATE general status 2>/dev/null | head -1")
	nmActiveOut, _ := nmActiveCmd.Output()
	nmState := strings.TrimSpace(string(nmActiveOut))
	if nmState == "connected" || nmState == "connecting" {
		LogTf("logs.wifi_networkmanager_active", interfaceName)
		executeCommand(fmt.Sprintf("sudo nmcli dev disconnect %s 2>/dev/null || true", interfaceName))
		executeCommand(fmt.Sprintf("sudo nmcli dev set %s managed no 2>/dev/null || true", interfaceName))
	} else {
		LogT("logs.wifi_networkmanager_stopping")
		executeCommand("sudo systemctl stop NetworkManager 2>/dev/null || true")
	}

	LogTf("logs.wifi_preparing_interface", interfaceName)
	executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo ip link set %s down 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second)
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second)

	if appConfig.Server.Debug { log.Printf("Paso 4: Deteniendo wpa_supplicant existente...") }
	stopWpaSupplicant(interfaceName)
	
	socketDirs := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	for _, socketDir := range socketDirs {
		socketFile := fmt.Sprintf("%s/%s", socketDir, interfaceName)
		executeCommand(fmt.Sprintf("sudo rm -f %s 2>/dev/null || true", socketFile))
		executeCommand(fmt.Sprintf("sudo rm -f %s/* 2>/dev/null || true", socketDir))
	}
	
	activeRunDir = ""

	if appConfig.Server.Debug { log.Printf("Paso 5: Creando archivo de configuración...") }

	safeSSID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(ssid, "_")
	wpaConfigPath := fmt.Sprintf("%s/wpa_supplicant-%s.conf", WpaSupplicantConfigDir, safeSSID)

	var networkBlock string
	if password != "" {
		checkCmd := exec.Command("sh", "-c", "which wpa_passphrase 2>/dev/null")
		checkOut, _ := checkCmd.Output()
		if strings.TrimSpace(string(checkOut)) == "" {
			result["error"] = "wpa_passphrase no está disponible. Instala el paquete wpa_supplicant"
			return result
		}

		cmd := exec.Command("wpa_passphrase", ssid, password)
		passphraseOut, err := cmd.Output()
		if err != nil || !strings.Contains(string(passphraseOut), "network=") {
			LogTf("logs.wifi_wpa_passphrase_error", err)
			result["error"] = "Error al generar la clave PSK. Verifica el SSID y la contraseña."
			return result
		}
		networkBlock = strings.TrimSpace(string(passphraseOut))
	} else {
		networkBlock = fmt.Sprintf("network={\n\tssid=\"%s\"\n\tkey_mgmt=NONE\n}", ssid)
	}

	activeRunDir = ""
	runDir := getRunDir()
	LogTf("logs.wifi_socket_dir_writable", runDir)
	
	if err := os.MkdirAll(runDir, 0755); err != nil {
		LogTf("logs.wifi_socket_dir_create_warning", runDir, err)
		runDir = "/tmp/wpa_supplicant"
		if err := os.MkdirAll(runDir, 0755); err != nil {
			LogTf("logs.wifi_socket_dir_tmp_error", err)
			result["error"] = "No se pudo crear directorio de socket para wpa_supplicant"
			return result
		}
		activeRunDir = runDir
		LogTf("logs.wifi_socket_dir_alt", runDir)
	}
	
	testFile := fmt.Sprintf("%s/.test_write", runDir)
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		LogTf("logs.wifi_socket_dir_not_writable", runDir, err)
		runDir = "/tmp/wpa_supplicant"
		os.MkdirAll(runDir, 0755)
		activeRunDir = runDir
		LogTf("logs.wifi_socket_dir_switching", runDir)
	} else {
		os.Remove(testFile)
		LogTf("logs.wifi_socket_dir_verified", runDir)
	}

	executeCommand(fmt.Sprintf("sudo mkdir -p %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chmod 775 %s 2>/dev/null || true", runDir))
	executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || true", runDir))
	
	configContent := fmt.Sprintf(`ctrl_interface=DIR=%s GROUP=netdev
ctrl_interface_group=netdev
update_config=1
country=%s

%s
`, runDir, country, networkBlock)

	LogTf("logs.wifi_config_content", configContent)

	executeCommand(fmt.Sprintf("sudo rm -f %s", wpaConfigPath))

	tmpConfigFile := fmt.Sprintf("/tmp/wpa_supplicant_%s_%d.conf", safeSSID, time.Now().Unix())
	if err := os.WriteFile(tmpConfigFile, []byte(configContent), 0644); err != nil {
		LogTf("logs.wifi_temp_file_error", err)
		result["error"] = fmt.Sprintf("Error al guardar configuración: %v", err)
		return result
	}
	
	if _, err := os.Stat(WpaSupplicantConfigDir); os.IsNotExist(err) {
		LogTf("logs.wifi_config_dir_creating_alt", WpaSupplicantConfigDir)
		mkdirCmd := exec.Command("sudo", "mkdir", "-p", WpaSupplicantConfigDir)
		mkdirCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
		if out, err := mkdirCmd.CombinedOutput(); err != nil {
			LogTf("logs.wifi_config_dir_error_alt", WpaSupplicantConfigDir, err, string(out))
		}
		exec.Command("sudo", "chmod", "755", WpaSupplicantConfigDir).Run()
		exec.Command("sudo", "chown", "root:netdev", WpaSupplicantConfigDir).Run()
	}
	
	cpPath := "/bin/cp"
	if _, err := os.Stat(cpPath); os.IsNotExist(err) {
		cpPath = "/usr/bin/cp"
	}
	cpCmd := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
	cpCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	cpOut, cpErr := cpCmd.CombinedOutput()
	cpOutStr := string(cpOut)
	
	if cpErr != nil {
		cpOutLower := strings.ToLower(cpOutStr)
		if strings.Contains(cpOutLower, "read-only") || strings.Contains(cpOutLower, "readonly") {
			LogTf("logs.wifi_readonly_detected", cpOutStr)
			LogT("logs.wifi_readonly_remounting")
			remountCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/")
			remountCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
			remountOut, remountErr := remountCmd.CombinedOutput()
			if remountErr != nil {
				LogTf("logs.wifi_remount_failed", remountErr, string(remountOut))
				LogTf("logs.wifi_using_alt_dir", WpaSupplicantAltConfigDir)
				parentDir := "/var/lib/hostberry"
				mkdirParentCmd := exec.Command("sudo", "mkdir", "-p", parentDir)
				mkdirParentCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				if mkdirParentOut, mkdirParentErr := mkdirParentCmd.CombinedOutput(); mkdirParentErr != nil {
					LogTf("logs.wifi_parent_dir_error", parentDir, mkdirParentErr, string(mkdirParentOut))
					remountVarCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/var")
					remountVarCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
					remountVarCmd.Run() // Intentar remontar
					if mkdirParentOut2, mkdirParentErr2 := mkdirParentCmd.CombinedOutput(); mkdirParentErr2 != nil {
						LogTf("logs.wifi_parent_dir_error2", mkdirParentErr2, string(mkdirParentOut2))
						os.Remove(tmpConfigFile)
						result["error"] = fmt.Sprintf("Error al guardar configuración: no se pudo crear directorio alternativo")
						return result
					}
				}
				mkdirAltCmd := exec.Command("sudo", "mkdir", "-p", WpaSupplicantAltConfigDir)
				mkdirAltCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				if mkdirOut, mkdirErr := mkdirAltCmd.CombinedOutput(); mkdirErr != nil {
					LogTf("logs.wifi_alt_dir_error", mkdirErr, string(mkdirOut))
					os.Remove(tmpConfigFile)
					result["error"] = fmt.Sprintf("Error al guardar configuración: no se pudo crear directorio alternativo")
					return result
				}
				exec.Command("sudo", "chmod", "755", WpaSupplicantAltConfigDir).Run()
				exec.Command("sudo", "chown", "root:netdev", WpaSupplicantAltConfigDir).Run()
				
				wpaConfigPath = fmt.Sprintf("%s/wpa_supplicant-%s.conf", WpaSupplicantAltConfigDir, safeSSID)
				
				cpCmd2 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
				cpCmd2.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				cpOut2, cpErr2 := cpCmd2.CombinedOutput()
				if cpErr2 != nil {
					if strings.Contains(string(cpOut2), "Read-only file system") {
						LogT("logs.wifi_var_readonly")
						remountVarCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/var")
						remountVarCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
						if remountVarOut, remountVarErr := remountVarCmd.CombinedOutput(); remountVarErr == nil {
							LogT("logs.wifi_var_remounted")
							cpCmd3 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
							cpCmd3.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
							if cpOut3, cpErr3 := cpCmd3.CombinedOutput(); cpErr3 != nil {
								LogTf("logs.wifi_write_failed_var", cpErr3, string(cpOut3))
								os.Remove(tmpConfigFile)
								result["error"] = fmt.Sprintf("Error al guardar configuración: sistema de archivos de solo lectura (incluso /var)")
								return result
							}
							LogTf("logs.wifi_file_saved_var", wpaConfigPath)
						} else {
							LogTf("logs.wifi_var_remount_failed", remountVarErr, string(remountVarOut))
							LogTf("logs.wifi_write_failed_alt", cpErr2, string(cpOut2))
							os.Remove(tmpConfigFile)
							result["error"] = fmt.Sprintf("Error al guardar configuración: sistema de archivos de solo lectura")
							return result
						}
					} else {
						LogTf("logs.wifi_write_failed_alt", cpErr2, string(cpOut2))
						os.Remove(tmpConfigFile)
						result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr2)
						return result
					}
				} else {
					LogTf("logs.wifi_file_saved_alt", wpaConfigPath)
				}
			} else {
				LogT("logs.wifi_remounted_retry")
				cpCmd2 := exec.Command("sudo", cpPath, tmpConfigFile, wpaConfigPath)
				cpCmd2.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				if cpOut2, cpErr2 := cpCmd2.CombinedOutput(); cpErr2 != nil {
					LogTf("logs.wifi_write_failed_remount", cpErr2, string(cpOut2))
					os.Remove(tmpConfigFile)
					result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr2)
					return result
				}
				LogT("logs.wifi_file_saved_remount")
			}
		} else {
			LogTf("logs.wifi_config_write_error", cpErr, string(cpOut))
			os.Remove(tmpConfigFile)
			result["error"] = fmt.Sprintf("Error al guardar configuración: %v", cpErr)
			return result
		}
	}
	
	os.Remove(tmpConfigFile)

	executeCommand(fmt.Sprintf("sudo chmod 600 %s", wpaConfigPath))
	executeCommand(fmt.Sprintf("sudo chown root:root %s", wpaConfigPath))

	LogTf("logs.wifi_config_created", wpaConfigPath)

	if appConfig.Server.Debug { log.Printf("Paso 6: Iniciando wpa_supplicant...") }
	if err := startWpaSupplicant(interfaceName, wpaConfigPath, runDir); err != nil {
		LogTf("logs.wifi_wpa_start_error2", err)
		result["error"] = "No se pudo iniciar wpa_supplicant. Verifica la instalación."
		return result
	}

	existsOut, _ := executeCommand(fmt.Sprintf("sudo ls -l %s/%s 2>/dev/null || true", runDir, interfaceName))
	if strings.TrimSpace(existsOut) == "" {
		LogTf("logs.wifi_socket_not_found", runDir, interfaceName)
	}

	if appConfig.Server.Debug { log.Printf("Paso 7: Estableciendo comunicación con wpa_cli...") }
	socketDir, err := waitForWpaCliConnection(interfaceName, 10)
	if err != nil {
		LogTf("logs.wifi_wpa_cli_error", err)
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

	if appConfig.Server.Debug { log.Printf("Paso 8: Conectando a la red...") }

	listOut, listErr := runWpaCli("list_networks")
	if listErr != nil {
		LogTf("logs.wifi_list_networks_error", listErr, listOut)
	} else {
		LogTf("logs.wifi_networks_configured", strings.TrimSpace(listOut))
	}

	lines := []string{}
	if listOut != "" {
		lines = strings.Split(listOut, "\n")
	}
	needsAdd := len(lines) <= 1
	if needsAdd {
		LogT("logs.wifi_no_networks_found")
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
		runWpaCli("select_network", "0")
		runWpaCli("enable_network", "0")
	}

	runWpaCli("reconnect")

	if appConfig.Server.Debug { log.Printf("Paso 9: Esperando conexión...") }
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

		stateRe := regexp.MustCompile(`wpa_state=([^\r\n]+)`)
		stateMatches := stateRe.FindStringSubmatch(statusOutput)
		currentState := ""
		if len(stateMatches) > 1 {
			currentState = strings.TrimSpace(stateMatches[1])
		}

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

		if strings.Contains(statusOutput, "wpa_state=COMPLETED") {
			connected = true
			// WiFi conectado exitosamente - no traducir, es debug interno
			break
		}

		if currentState != "" && currentState != lastState {
			log.Printf("Estado cambiado: %s -> %s", lastState, currentState)
			lastState = currentState
		}

		if currentState == "DISCONNECTED" || currentState == "INACTIVE" {
			if attempt > 3 && attempt%3 == 0 {
				log.Printf("Reintentando conexión...")
				executeCommand(fmt.Sprintf("%s disconnect", wpaCliCmd))
				time.Sleep(1 * time.Second)
				executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
			}
		}
	}

	if connected {
		if appConfig.Server.Debug { log.Printf("Paso 10: Obteniendo dirección IP...") }
		ipObtained := false
		var ip string

		log.Printf("Solicitando IP con DHCP...")
		executeCommand(fmt.Sprintf("sudo pkill -f 'dhclient.*%s|udhcpc.*%s' 2>/dev/null || true", interfaceName, interfaceName))
		time.Sleep(500 * time.Millisecond)
		dhcpOut, dhcpErr := executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>&1 || sudo udhcpc -i %s -q -n 2>&1 || true", interfaceName, interfaceName))
		if dhcpErr == nil && dhcpOut != "" {
			log.Printf("DHCP output: %s", dhcpOut)
		}

		for ipAttempt := 0; ipAttempt < 15 && !ipObtained; ipAttempt++ {
			time.Sleep(2 * time.Second)

			ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", interfaceName))
			ipOut, _ := ipCmd.Output()
			ip = strings.TrimSpace(string(ipOut))

			if ip != "" && ip != "N/A" && !strings.HasPrefix(ip, "169.254") {
				ipObtained = true
				log.Printf("IP obtenida: %s", ip)
			} else {
				log.Printf("Esperando IP... (intento %d/15)", ipAttempt+1)
				if ipAttempt > 2 && ipAttempt%3 == 0 {
					log.Printf("Reintentando DHCP...")
					executeCommand(fmt.Sprintf("sudo dhclient -r %s 2>/dev/null || true", interfaceName))
					time.Sleep(500 * time.Millisecond)
					executeCommand(fmt.Sprintf("sudo pkill -f 'dhclient.*%s|udhcpc.*%s' 2>/dev/null || true", interfaceName, interfaceName))
					time.Sleep(500 * time.Millisecond)
					dhcpOut2, _ := executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>&1 || sudo udhcpc -i %s -q -n 2>&1 || true", interfaceName, interfaceName))
					if dhcpOut2 != "" {
						log.Printf("DHCP retry output: %s", dhcpOut2)
					}
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

func toggleWiFi(interfaceName string, enable bool) map[string]interface{} {
	result := make(map[string]interface{})

	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	if enable {
		executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
		executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
		result["success"] = true
		result["message"] = "WiFi habilitado"
		result["enabled"] = true
	} else {
		executeCommand("sudo rfkill block wifi 2>/dev/null || true")
		executeCommand(fmt.Sprintf("sudo ip link set %s down 2>/dev/null || true", interfaceName))
		result["success"] = true
		result["message"] = "WiFi deshabilitado"
		result["enabled"] = false
	}

	return result
}

// getLastConnectedNetwork obtiene la última red WiFi conectada desde los archivos de configuración
func getLastConnectedNetwork(interfaceName string) (string, string, error) {
	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	// Primero intentar obtener desde wpa_cli si wpa_supplicant está corriendo
	socketDirs := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	for _, dir := range socketDirs {
		// Intentar socket de interfaz
		socketPath := fmt.Sprintf("%s/%s", dir, interfaceName)
		if _, err := os.Stat(socketPath); err == nil {
			runWpaCli := func(args ...string) (string, error) {
				base := []string{"wpa_cli", "-i", interfaceName, "-p", dir}
				cmd := exec.Command("sudo", append(base, args...)...)
				out, err := cmd.CombinedOutput()
				return strings.TrimSpace(string(out)), err
			}
			
			listOut, err := runWpaCli("list_networks")
			if err == nil && listOut != "" {
				lines := strings.Split(listOut, "\n")
				if len(lines) > 1 {
					// Buscar la red activa o la primera habilitada
					for i, line := range lines {
						if i == 0 {
							continue // Saltar encabezado
						}
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							ssid := strings.Trim(fields[1], "\"")
							if ssid != "" && ssid != "--" {
								// Verificar si está habilitada o es CURRENT
								if len(fields) >= 4 && (fields[3] == "[CURRENT]" || fields[2] == "[ENABLED]") {
									log.Printf("Red encontrada en wpa_cli: %s", ssid)
									return ssid, "", nil
								}
							}
						}
					}
					// Si no hay red activa, tomar la primera
					if len(lines) > 1 {
						firstLine := lines[1]
						fields := strings.Fields(firstLine)
						if len(fields) >= 2 {
							ssid := strings.Trim(fields[1], "\"")
							if ssid != "" && ssid != "--" {
								LogTf("logs.wifi_first_network_cli", ssid)
								return ssid, "", nil
							}
						}
					}
				}
			}
		}
		
		// Intentar socket global
		globalSocket := fmt.Sprintf("%s/global", dir)
		if _, err := os.Stat(globalSocket); err == nil {
			runWpaCli := func(args ...string) (string, error) {
				base := []string{"wpa_cli", "-g", dir, "-i", interfaceName}
				cmd := exec.Command("sudo", append(base, args...)...)
				out, err := cmd.CombinedOutput()
				return strings.TrimSpace(string(out)), err
			}
			
			listOut, err := runWpaCli("list_networks")
			if err == nil && listOut != "" {
				lines := strings.Split(listOut, "\n")
				if len(lines) > 1 {
					// Buscar la red activa o la primera habilitada
					for i, line := range lines {
						if i == 0 {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							ssid := strings.Trim(fields[1], "\"")
							if ssid != "" && ssid != "--" {
								if len(fields) >= 4 && (fields[3] == "[CURRENT]" || fields[2] == "[ENABLED]") {
									LogTf("logs.wifi_network_found_global", ssid)
									return ssid, "", nil
								}
							}
						}
					}
					// Si no hay red activa, tomar la primera
					if len(lines) > 1 {
						firstLine := lines[1]
						fields := strings.Fields(firstLine)
						if len(fields) >= 2 {
							ssid := strings.Trim(fields[1], "\"")
							if ssid != "" && ssid != "--" {
								LogTf("logs.wifi_first_network_global", ssid)
								return ssid, "", nil
							}
						}
					}
				}
			}
		}
	}

	// Si no se encontró en wpa_cli, buscar en archivos de configuración
	// Buscar en ambos directorios
	configDirs := []string{WpaSupplicantConfigDir, WpaSupplicantAltConfigDir}
	var lastConfigFile os.DirEntry
	var lastModTime time.Time
	var foundConfigDir string

	for _, configDir := range configDirs {
		configFiles, err := os.ReadDir(configDir)
		if err != nil {
			continue
		}

		// Buscar el archivo de configuración más reciente
		for _, file := range configFiles {
			if strings.HasPrefix(file.Name(), "wpa_supplicant-") && strings.HasSuffix(file.Name(), ".conf") {
				info, err := file.Info()
				if err != nil {
					continue
				}
				if info.ModTime().After(lastModTime) {
					lastModTime = info.ModTime()
					lastConfigFile = file
					foundConfigDir = configDir
				}
			}
		}
	}

	if lastConfigFile == nil {
		return "", "", fmt.Errorf("no se encontró ninguna red guardada")
	}

	configPath := fmt.Sprintf("%s/%s", foundConfigDir, lastConfigFile.Name())
	
	// Leer archivo usando sudo cat (los archivos tienen permisos 600 y pertenecen a root)
	cmd := exec.Command("sudo", "cat", configPath)
	cmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
	configContentBytes, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error leyendo archivo de configuración: %v", err)
	}
	configContent := string(configContentBytes)

	// Extraer SSID del archivo de configuración
	ssid := ""
	lines := strings.Split(string(configContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ssid=") {
			ssid = strings.Trim(strings.TrimPrefix(line, "ssid="), "\"")
			break
		}
	}

	if ssid == "" {
		return "", "", fmt.Errorf("no se pudo extraer SSID del archivo de configuración")
	}

	LogTf("logs.wifi_ssid_found_config", ssid)
	// No retornamos password porque no podemos obtenerla del archivo (está hasheada)
	// La contraseña ya está en el archivo de configuración, solo necesitamos el SSID
	return ssid, "", nil
}

// autoConnectToLastNetwork intenta conectarse automáticamente a la última red WiFi conectada
func autoConnectToLastNetwork(interfaceName string) {
	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	LogTf("logs.wifi_auto_connect_start", interfaceName)

	// Verificar que la interfaz existe antes de continuar
	cmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null", interfaceName))
	if err := cmd.Run(); err != nil {
		LogTf("logs.wifi_interface_not_exists", interfaceName)
		return
	}

	// Paso 1: Activar software switch y WiFi en paralelo (más rápido)
	LogT("logs.wifi_activating")
	executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", interfaceName))
	time.Sleep(1 * time.Second) // Reducido de 3 a 1 segundo

	// Paso 3: Intentar obtener la última red desde wpa_cli (más confiable)
	LogT("logs.wifi_searching_network")
	
	// Primero intentar con wpa_cli si wpa_supplicant está corriendo
	socketDirs := []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant", "/tmp/wpa_supplicant"}
	var workingSocketDir string
	var useGlobalSocket bool
	
	// Buscar socket de interfaz específica
	for _, dir := range socketDirs {
		socketPath := fmt.Sprintf("%s/%s", dir, interfaceName)
		if _, err := os.Stat(socketPath); err == nil {
			workingSocketDir = dir
			LogTf("logs.wifi_socket_interface_found", socketPath)
			break
		}
	}
	
	// Si no hay socket de interfaz, intentar con socket global
	if workingSocketDir == "" {
		for _, dir := range socketDirs {
			globalSocket := fmt.Sprintf("%s/global", dir)
			if _, err := os.Stat(globalSocket); err == nil {
				workingSocketDir = dir
				useGlobalSocket = true
				LogTf("logs.wifi_socket_global_found", globalSocket)
				break
			}
		}
	}

	if workingSocketDir != "" {
		// wpa_supplicant está corriendo, intentar reconectar a la red activa (método más rápido)
		LogT("logs.wifi_wpa_running")
		runWpaCli := func(args ...string) (string, error) {
			var base []string
			if useGlobalSocket {
				base = []string{"wpa_cli", "-g", workingSocketDir, "-i", interfaceName}
			} else {
				base = []string{"wpa_cli", "-i", interfaceName, "-p", workingSocketDir}
			}
			cmd := exec.Command("sudo", append(base, args...)...)
			out, err := cmd.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}

		// Obtener estado actual (verificación rápida)
		statusOut, err := runWpaCli("status")
		if err == nil {
			// Verificar si ya está conectado (verificación inmediata)
			if strings.Contains(statusOut, "wpa_state=COMPLETED") {
				LogT("logs.wifi_already_connected")
				// Iniciar DHCP en segundo plano si no tiene IP
				go func() {
					ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", interfaceName))
					if ipOut, _ := ipCmd.Output(); ipOut == nil || strings.TrimSpace(string(ipOut)) == "" {
						executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>&1 || sudo udhcpc -i %s -q -n 2>&1 || true", interfaceName, interfaceName))
					}
				}()
				return
			}

			// Reconexión rápida: habilitar y reconectar directamente
			listOut, _ := runWpaCli("list_networks")
			if listOut != "" {
				lines := strings.Split(listOut, "\n")
				if len(lines) > 1 {
					// Buscar red CURRENT o primera habilitada
					var netID string
					for i, line := range lines {
						if i == 0 {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) >= 1 {
							if len(fields) >= 4 && fields[3] == "[CURRENT]" {
								netID = fields[0]
								break
							} else if netID == "" && len(fields) >= 2 {
								netID = fields[0]
							}
						}
					}
					
					if netID != "" {
						LogTf("logs.wifi_reconnecting", netID)
						// Hacer todo en secuencia rápida
						runWpaCli("enable_network", netID)
						runWpaCli("select_network", netID)
						runWpaCli("reconnect")
						
						// Verificar conexión más rápido (menos espera)
						for attempt := 0; attempt < 5; attempt++ {
							time.Sleep(1 * time.Second) // Reducido de 2 a 1 segundo
							statusOut2, _ := runWpaCli("status")
							if strings.Contains(statusOut2, "wpa_state=COMPLETED") {
								LogT("logs.wifi_reconnected")
								// Iniciar DHCP en segundo plano
								go func() {
									executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>&1 || sudo udhcpc -i %s -q -n 2>&1 || true", interfaceName, interfaceName))
								}()
								return
							}
						}
						LogT("logs.wifi_reconnect_started")
					}
				}
			}
		} else {
			// Error obteniendo estado - no traducir, es debug interno
		}
	} else {
		LogT("logs.wifi_wpa_not_running")
	}

	// Paso 4: Si no hay wpa_supplicant corriendo o no se pudo reconectar,
	// buscar el último archivo de configuración y conectarse usando connectWiFi
	LogT("logs.wifi_searching_config")
	ssid, _, err := getLastConnectedNetwork(interfaceName)
	if err != nil {
		LogTf("logs.wifi_config_not_found", err)
		LogT("logs.wifi_trying_other_way")
		
		// Último intento: buscar cualquier archivo de configuración reciente
		configDirs := []string{WpaSupplicantConfigDir, WpaSupplicantAltConfigDir}
		for _, configDir := range configDirs {
			configFiles, err := os.ReadDir(configDir)
			if err != nil {
				continue
			}
			var lastFile os.DirEntry
			var lastTime time.Time
			for _, file := range configFiles {
				if strings.HasPrefix(file.Name(), "wpa_supplicant-") && strings.HasSuffix(file.Name(), ".conf") {
					info, err := file.Info()
					if err == nil && info.ModTime().After(lastTime) {
						lastTime = info.ModTime()
						lastFile = file
					}
				}
			}
			if lastFile != nil {
				configPath := fmt.Sprintf("%s/%s", configDir, lastFile.Name())
				// Leer archivo usando sudo cat
				cmd := exec.Command("sudo", "cat", configPath)
				cmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
				contentBytes, err := cmd.CombinedOutput()
				if err != nil {
					continue
				}
				lines := strings.Split(string(contentBytes), "\n")
				for _, line := range lines {
					if strings.HasPrefix(strings.TrimSpace(line), "ssid=") {
						ssid = strings.Trim(strings.TrimPrefix(strings.TrimSpace(line), "ssid="), "\"")
						if ssid != "" {
							LogTf("logs.wifi_network_found_file", lastFile.Name(), ssid)
							break
						}
					}
				}
				if ssid != "" {
					break
				}
			}
		}
		
		if ssid == "" {
			LogT("logs.wifi_no_network_found")
			return
		}
	} else {
		// Última red encontrada - no traducir, es debug interno
	}

	// Paso 5: Usar el archivo de configuración existente para iniciar wpa_supplicant
	// Paso 5: Conectándose usando archivo de configuración existente...
	
	// Buscar el archivo de configuración para esta red
	safeSSID := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(ssid, "_")
	wpaConfigPath := fmt.Sprintf("%s/wpa_supplicant-%s.conf", WpaSupplicantConfigDir, safeSSID)
	
	if _, err := os.Stat(wpaConfigPath); os.IsNotExist(err) {
		// Intentar directorio alternativo
		wpaConfigPath = fmt.Sprintf("%s/wpa_supplicant-%s.conf", WpaSupplicantAltConfigDir, safeSSID)
		if _, err := os.Stat(wpaConfigPath); os.IsNotExist(err) {
			// Buscar cualquier archivo que contenga este SSID
			configDirs := []string{WpaSupplicantConfigDir, WpaSupplicantAltConfigDir}
			found := false
			for _, configDir := range configDirs {
				configFiles, err := os.ReadDir(configDir)
				if err != nil {
					continue
				}
				for _, file := range configFiles {
					if strings.Contains(file.Name(), safeSSID) && strings.HasSuffix(file.Name(), ".conf") {
						wpaConfigPath = fmt.Sprintf("%s/%s", configDir, file.Name())
						// Verificar que el archivo existe y es legible
						cmd := exec.Command("sudo", "test", "-r", wpaConfigPath)
						cmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
						if err := cmd.Run(); err == nil {
							found = true
							LogTf("logs.wifi_config_file_found", wpaConfigPath)
							break
						}
					}
				}
				if found {
					break
				}
			}
			if !found {
				// No se encontró archivo de configuración - no traducir, es debug interno
				// Último recurso: usar connectWiFi
				country := DefaultCountryCode
				result := connectWiFi(ssid, "", interfaceName, country, "system")
				if success, ok := result["success"].(bool); ok && success {
					LogTf("logs.wifi_auto_success", ssid)
					return
				} else {
					errorMsg := "Error desconocido"
					if err, ok := result["error"].(string); ok && err != "" {
						errorMsg = err
					}
					LogTf("logs.wifi_auto_error", errorMsg)
					return
				}
			}
		}
	}
	
	// Usando archivo de configuración - no traducir, es debug interno
	
	// Detener wpa_supplicant existente si está corriendo (más rápido)
	stopWpaSupplicant(interfaceName)
	time.Sleep(1 * time.Second) // Reducido de 2 a 1 segundo
	
	// Iniciar wpa_supplicant con el archivo de configuración existente
	runDir := getRunDir()
	LogT("logs.wifi_starting_wpa")
	if err := startWpaSupplicant(interfaceName, wpaConfigPath, runDir); err != nil {
		LogTf("logs.wifi_wpa_start_error", err)
		LogT("logs.wifi_trying_connect")
		country := DefaultCountryCode
		result := connectWiFi(ssid, "", interfaceName, country, "system")
		if success, ok := result["success"].(bool); ok && success {
			LogTf("logs.wifi_auto_success", ssid)
		} else {
			LogTf("logs.wifi_auto_error", result["error"])
		}
		return
	}
	
	// Esperar menos tiempo para que wpa_supplicant se inicie
	time.Sleep(2 * time.Second) // Reducido de 3 a 2 segundos
	
	// Verificar conexión usando wpa_cli (menos intentos, más rápido)
	socketDir, err := waitForWpaCliConnection(interfaceName, 5) // Reducido de 10 a 5 intentos
	if err != nil {
		// No se pudo conectar a wpa_cli - no traducir, es debug interno
		return
	}
	
	runWpaCli := func(args ...string) (string, error) {
		base := []string{"wpa_cli", "-i", interfaceName, "-p", socketDir}
		cmd := exec.Command("sudo", append(base, args...)...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	
	// Habilitar y seleccionar la red (todo en secuencia rápida)
	LogT("logs.wifi_enabling_network")
	runWpaCli("enable_network", "0")
	runWpaCli("select_network", "0")
	runWpaCli("reconnect")
	
	// Verificar conexión más rápido (menos espera, menos intentos)
	LogT("logs.wifi_waiting_auth")
	connected := false
	for attempt := 0; attempt < 8; attempt++ { // Reducido de 15 a 8 intentos
		time.Sleep(1 * time.Second) // Reducido de 2 a 1 segundo
		statusOut, _ := runWpaCli("status")
		if strings.Contains(statusOut, "wpa_state=COMPLETED") {
			connected = true
			LogTf("logs.wifi_authenticated", ssid)
			break
		}
		if attempt%3 == 0 && attempt > 0 {
			LogTf("logs.wifi_status_attempt3", statusOut, attempt+1)
		}
	}
	
	if !connected {
		log.Printf("⚠️  No se pudo autenticar después de 8 segundos. Estado final:")
		statusOut, _ := runWpaCli("status")
		log.Printf("Estado: %s", statusOut)
		// Continuar de todas formas, puede que se conecte después
	}
	
	// Solicitar IP con DHCP en segundo plano (no bloquea)
	log.Printf("📡 Solicitando IP con DHCP...")
	go func() {
		executeCommand(fmt.Sprintf("sudo pkill -f 'dhclient.*%s|udhcpc.*%s' 2>/dev/null || true", interfaceName, interfaceName))
		time.Sleep(300 * time.Millisecond)
		executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>&1 || sudo udhcpc -i %s -q -n 2>&1 || true", interfaceName, interfaceName))
	}()
	
	// Verificar IP rápidamente (solo unos pocos intentos)
	var ip string
	for ipAttempt := 0; ipAttempt < 5; ipAttempt++ { // Reducido de 15 a 5 intentos
		time.Sleep(1 * time.Second) // Reducido de 2 a 1 segundo
		
		ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", interfaceName))
		ipOut, _ := ipCmd.Output()
		ip = strings.TrimSpace(string(ipOut))
		
		if ip != "" && ip != "N/A" && !strings.HasPrefix(ip, "169.254") {
			log.Printf("✅✅ Autoconexión completa: %s (IP: %s)", ssid, ip)
			return
		}
	}
	
	if connected {
		log.Printf("✅ Autoconexión exitosa a %s (IP se asignará en segundo plano)", ssid)
	} else {
		log.Printf("⚠️  Autoconexión iniciada, puede tardar unos segundos más en completarse")
	}
}
