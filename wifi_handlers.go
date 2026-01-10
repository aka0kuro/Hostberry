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
	useSystemWpa := false

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

	// Evitar conflicto con wpa_supplicant gestionado por el sistema (modo -u / DBus)
	systemWpaOut, _ := exec.Command("sh", "-c", "ps aux | grep -E '[w]pa_supplicant.* -u ' 2>/dev/null").Output()
	if strings.TrimSpace(string(systemWpaOut)) != "" {
		log.Printf("wpa_supplicant está siendo gestionado por el sistema (-u)")
		// En sistemas tipo Arch (wpa_supplicant global con -O DIR=/run/wpa_supplicant),
		// lo correcto es usar el socket global y agregar la interfaz, no parar el servicio.
		useSystemWpa = true
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
	time.Sleep(1 * time.Second)
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null", interfaceName))
	time.Sleep(2 * time.Second)

	// Si NO estamos usando wpa_supplicant global del sistema, podemos controlar una instancia por interfaz.
	if !useSystemWpa {
		// Detener cualquier instancia de wpa_supplicant existente SOLO para esta interfaz
		wpaPid, _ := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
		if strings.TrimSpace(string(wpaPid)) != "" {
			log.Printf("Deteniendo wpa_supplicant existente para reiniciar limpiamente")
			executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
			time.Sleep(2 * time.Second)
		}
	}

	// Asegurar que wpa_supplicant esté corriendo
	// - Si usamos wpa_supplicant global del sistema (-u), agregamos la interfaz vía socket global.
	// - Si no, iniciamos una instancia por interfaz.
	wpaPid, _ := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
	if useSystemWpa {
		ctrlDir := "/run/wpa_supplicant"
		if _, err := os.Stat(ctrlDir); err != nil {
			ctrlDir = "/var/run/wpa_supplicant"
		}
		globalSock := fmt.Sprintf("%s/global", ctrlDir)
		log.Printf("Usando wpa_supplicant global: %s", globalSock)
		// Verificar que el socket global exista y responda
		pingGlobalCmd := fmt.Sprintf("sudo wpa_cli -g %s ping 2>&1", globalSock)
		if pingOut, _ := executeCommand(pingGlobalCmd); !strings.Contains(pingOut, "PONG") {
			result["success"] = false
			result["error"] = "wpa_supplicant global no responde (socket global no disponible)."
			return result
		}
		// Si el socket por interfaz no existe, agregar la interfaz
		ifaceSock := fmt.Sprintf("%s/%s", ctrlDir, interfaceName)
		ifaceCheck := exec.Command("sh", "-c", fmt.Sprintf("test -S %s && echo ok || echo missing", ifaceSock))
		if out, _ := ifaceCheck.Output(); strings.Contains(string(out), "missing") {
			log.Printf("Socket por interfaz no existe (%s). Agregando interfaz vía wpa_cli -g...", ifaceSock)
			// interface_add <ifname> <config> <driver> <ctrl_interface>
			addCmd := fmt.Sprintf("sudo wpa_cli -g %s interface_add %s '' nl80211 %s 2>&1", globalSock, interfaceName, ctrlDir)
			addOut, _ := executeCommand(addCmd)
			log.Printf("interface_add output: %s", strings.TrimSpace(addOut))
			// Esperar a que aparezca el socket
			time.Sleep(1 * time.Second)
			ifaceCheck2 := exec.Command("sh", "-c", fmt.Sprintf("test -S %s && echo ok || echo missing", ifaceSock))
			if out2, _ := ifaceCheck2.Output(); strings.Contains(string(out2), "missing") {
				result["success"] = false
				result["error"] = "No se pudo crear el socket de control de la interfaz en wpa_supplicant."
				return result
			}
		}
		log.Printf("Interfaz %s lista en wpa_supplicant global", interfaceName)
	} else if strings.TrimSpace(string(wpaPid)) == "" {
		log.Printf("Iniciando wpa_supplicant en interfaz %s", interfaceName)
		
		wpaConfig := fmt.Sprintf("/etc/wpa_supplicant/wpa_supplicant-%s.conf", interfaceName)
		if _, err := os.Stat(wpaConfig); os.IsNotExist(err) {
			wpaConfig = "/etc/wpa_supplicant/wpa_supplicant.conf"
			if _, err := os.Stat(wpaConfig); os.IsNotExist(err) {
				// Crear configuración con grupo netdev y permisos del socket
				defaultConfig := fmt.Sprintf("ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev\nctrl_interface_group=netdev\nupdate_config=1\ncountry=%s\n", country)
				os.WriteFile(wpaConfig, []byte(defaultConfig), 0600)
				log.Printf("Archivo de configuración wpa_supplicant creado con GROUP=netdev")
			} else {
				// Actualizar country y asegurar que GROUP=netdev esté presente
				configContent, _ := os.ReadFile(wpaConfig)
				configStr := string(configContent)
				
				// Asegurar que GROUP=netdev esté presente
				if !strings.Contains(configStr, "GROUP=netdev") && !strings.Contains(configStr, "GROUP=hostberry") {
					// Agregar GROUP=netdev si no está presente
					if strings.Contains(configStr, "ctrl_interface=") {
						// Reemplazar ctrl_interface existente para agregar GROUP
						re := regexp.MustCompile(`ctrl_interface=DIR=([^\n]+)`)
						configStr = re.ReplaceAllString(configStr, "ctrl_interface=DIR=$1 GROUP=netdev")
					} else {
						configStr = fmt.Sprintf("ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev\nctrl_interface_group=netdev\n%s", configStr)
					}
				}
				
				// Actualizar country
				if !strings.Contains(configStr, "country=") {
					configStr += fmt.Sprintf("\ncountry=%s\n", country)
				} else {
					re := regexp.MustCompile(`country=[A-Z][A-Z]`)
					configStr = re.ReplaceAllString(configStr, fmt.Sprintf("country=%s", country))
				}
				
				os.WriteFile(wpaConfig, []byte(configStr), 0600)
				log.Printf("Archivo de configuración wpa_supplicant actualizado con GROUP=netdev")
			}
		}

		// Asegurar que el directorio del socket existe con permisos y grupo correctos
		executeCommand("sudo mkdir -p /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo mkdir -p /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /run/wpa_supplicant 2>/dev/null || true")
		// Asegurar que el grupo sea netdev (necesario para wpa_cli)
		executeCommand("sudo chgrp netdev /var/run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant 2>/dev/null || true")
		
		// Limpiar wpa_supplicant y sockets SOLO de esta interfaz antes de reiniciar
		log.Printf("Limpiando procesos y sockets de wpa_supplicant...")
		executeCommand(fmt.Sprintf("sudo pkill -9 -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
		time.Sleep(1 * time.Second) // Dar tiempo para que los procesos terminen
		
		// Limpiar sockets solo de la interfaz
		executeCommand(fmt.Sprintf("sudo rm -rf /var/run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
		executeCommand(fmt.Sprintf("sudo rm -rf /run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
		
		// Asegurar que los directorios existan con permisos correctos ANTES de iniciar wpa_supplicant
		executeCommand("sudo mkdir -p /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo mkdir -p /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /var/run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chown root:netdev /var/run/wpa_supplicant 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chown root:netdev /run/wpa_supplicant 2>/dev/null || sudo chown root:hostberry /run/wpa_supplicant 2>/dev/null || true")

		// Iniciar wpa_supplicant especificando explícitamente el directorio de control (-C)
		// Nota: -g NO es un grupo; es el socket global. Pasar "netdev" aquí rompe el control socket.
		ctrlDir := "/run/wpa_supplicant"
		if _, err := os.Stat(ctrlDir); err != nil {
			ctrlDir = "/var/run/wpa_supplicant"
		}
		log.Printf("Iniciando wpa_supplicant con ctrlDir=%s...", ctrlDir)
		startCmd := fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext -C %s", interfaceName, wpaConfig, ctrlDir)
		startOut, _ := executeCommand(startCmd)
		log.Printf("wpa_supplicant start output: %s", strings.TrimSpace(startOut))
		
		// Si falla, intentar sin -C (usará ctrl_interface desde config)
		if strings.Contains(strings.ToLower(startOut), "error") || strings.Contains(strings.ToLower(startOut), "failed") {
			log.Printf("Intento con -C falló, intentando sin -C...")
			startCmd = fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext", interfaceName, wpaConfig)
			startOut, _ = executeCommand(startCmd)
			log.Printf("wpa_supplicant start output (sin -C): %s", strings.TrimSpace(startOut))
		}
		
		// Esperar un momento para que wpa_supplicant cree el socket
		time.Sleep(500 * time.Millisecond)
		
		// Esperar más tiempo y verificar que el socket se haya creado
		socketReady := false
		socketPath1 := fmt.Sprintf("/var/run/wpa_supplicant/%s", interfaceName)
		socketPath2 := fmt.Sprintf("/run/wpa_supplicant/%s", interfaceName)
		
		for waitAttempt := 0; waitAttempt < 15; waitAttempt++ {
			time.Sleep(500 * time.Millisecond)
			// Verificar que wpa_supplicant esté corriendo
			wpaPid, _ = exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
			if strings.TrimSpace(string(wpaPid)) != "" {
				// Verificar que el socket esté disponible y ajustar permisos inmediatamente
				socketCheck1 := exec.Command("sh", "-c", fmt.Sprintf("test -S %s && echo 'exists' || echo 'not'", socketPath1))
				socketCheck2 := exec.Command("sh", "-c", fmt.Sprintf("test -S %s && echo 'exists' || echo 'not'", socketPath2))
				
				if socketOut1, _ := socketCheck1.Output(); strings.Contains(string(socketOut1), "exists") {
					// Socket encontrado, ajustar permisos inmediatamente
					log.Printf("Socket encontrado en: %s, ajustando permisos...", socketPath1)
					executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath1))
					executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
					// Verificar que los permisos se aplicaron correctamente
					time.Sleep(300 * time.Millisecond)
					// Verificar con wpa_cli ping
					pingTest := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1 | grep -q PONG && echo 'ok' || echo 'fail'", interfaceName))
					if pingOut, _ := pingTest.Output(); strings.Contains(string(pingOut), "ok") {
						socketReady = true
						log.Printf("✅ Socket configurado correctamente y wpa_cli responde")
						break
					} else {
						log.Printf("⚠️  Socket existe pero wpa_cli no responde, reintentando ajuste de permisos...")
						// Reintentar ajuste de permisos con chown también
						executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath1))
						executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
						executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
						time.Sleep(200 * time.Millisecond)
						// Verificar nuevamente
						pingTest2 := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1 | grep -q PONG && echo 'ok' || echo 'fail'", interfaceName))
						if pingOut2, _ := pingTest2.Output(); strings.Contains(string(pingOut2), "ok") {
							socketReady = true
							log.Printf("✅ Socket configurado correctamente después del segundo intento")
							break
						}
					}
				} else if socketOut2, _ := socketCheck2.Output(); strings.Contains(string(socketOut2), "exists") {
					// Socket encontrado, ajustar permisos inmediatamente
					log.Printf("Socket encontrado en: %s, ajustando permisos...", socketPath2)
					executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath2))
					executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
					// Verificar que los permisos se aplicaron correctamente
					time.Sleep(300 * time.Millisecond)
					// Verificar con wpa_cli ping
					pingTest := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1 | grep -q PONG && echo 'ok' || echo 'fail'", interfaceName))
					if pingOut, _ := pingTest.Output(); strings.Contains(string(pingOut), "ok") {
						socketReady = true
						log.Printf("✅ Socket configurado correctamente y wpa_cli responde")
						break
					} else {
						log.Printf("⚠️  Socket existe pero wpa_cli no responde, reintentando ajuste de permisos...")
						// Reintentar ajuste de permisos con chown también
						executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath2))
						executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
						executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
						time.Sleep(200 * time.Millisecond)
						// Verificar nuevamente
						pingTest2 := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1 | grep -q PONG && echo 'ok' || echo 'fail'", interfaceName))
						if pingOut2, _ := pingTest2.Output(); strings.Contains(string(pingOut2), "ok") {
							socketReady = true
							log.Printf("✅ Socket configurado correctamente después del segundo intento")
							break
						}
					}
				} else {
					// Intentar verificar con wpa_cli ping (puede funcionar aunque el socket no sea visible)
					pingTest := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1 | grep -q PONG && echo 'ok' || echo 'fail'", interfaceName))
					if pingOut, _ := pingTest.Output(); strings.Contains(string(pingOut), "ok") {
						socketReady = true
						log.Printf("wpa_cli responde correctamente (socket puede estar en otra ubicación)")
						break
					}
				}
			}
		}
		
		// Intentar ajustar permisos de todos los sockets posibles como medida adicional
		if !socketReady {
			log.Printf("Intentando ajustar permisos de sockets como medida preventiva...")
			executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath1))
			executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath2))
			executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
			executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
			// También ajustar permisos de todos los sockets en el directorio
			executeCommand("sudo chmod 660 /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chmod 660 /run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chgrp netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chgrp netdev /run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chown root:netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chown root:netdev /run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /run/wpa_supplicant/* 2>/dev/null || true")
		}

		// Verificar que wpa_supplicant se inició
		wpaPid, _ = exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
		if strings.TrimSpace(string(wpaPid)) == "" {
			log.Printf("ERROR: wpa_supplicant no se inició correctamente")
			result["success"] = false
			result["error"] = "No se pudo iniciar wpa_supplicant"
			return result
		} else {
			log.Printf("wpa_supplicant iniciado correctamente con PID: %s", strings.TrimSpace(string(wpaPid)))
			if !socketReady {
				log.Printf("WARNING: Socket de control puede no estar listo, pero continuando...")
			}
		}
	} else {
		// wpa_supplicant ya está corriendo, verificar que el socket esté disponible
		log.Printf("wpa_supplicant ya está corriendo con PID: %s", strings.TrimSpace(string(wpaPid)))
		time.Sleep(1 * time.Second) // Dar tiempo para que el socket esté listo
		
		// Asegurar permisos del socket si existe (múltiples ubicaciones posibles)
		socketPath1 := fmt.Sprintf("/var/run/wpa_supplicant/%s", interfaceName)
		socketPath2 := fmt.Sprintf("/run/wpa_supplicant/%s", interfaceName)
		
		log.Printf("Ajustando permisos de sockets para %s...", interfaceName)
		// Ajustar permisos de sockets específicos
		executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath1))
		executeCommand(fmt.Sprintf("sudo chmod 660 %s 2>/dev/null || true", socketPath2))
		executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
		executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
		
		// Ajustar permisos de todos los sockets en el directorio (por si hay múltiples interfaces)
		executeCommand("sudo chmod 660 /var/run/wpa_supplicant/* 2>/dev/null || true")
		executeCommand("sudo chmod 660 /run/wpa_supplicant/* 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant/* 2>/dev/null || true")
		executeCommand("sudo chown root:netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
		executeCommand("sudo chown root:netdev /run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /run/wpa_supplicant/* 2>/dev/null || true")
		
		// Asegurar que el directorio también tenga permisos correctos
		executeCommand("sudo chmod 775 /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /var/run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant 2>/dev/null || true")
		
		// Verificar que wpa_cli puede comunicarse después de ajustar permisos
		pingTest := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1 | grep -q PONG && echo 'ok' || echo 'fail'", interfaceName))
		if pingOut, _ := pingTest.Output(); strings.Contains(string(pingOut), "ok") {
			log.Printf("✅ wpa_cli puede comunicarse después de ajustar permisos")
		} else {
			log.Printf("⚠️  wpa_cli aún no puede comunicarse, puede requerir reiniciar wpa_supplicant")
		}
	}

	// Usar wpa_cli con el directorio de control (NO el path del socket).
	// Esto es la forma estándar y evita fallos por rutas de socket mal interpretadas.
	ctrlDir := "/run/wpa_supplicant"
	if _, err := os.Stat(ctrlDir); err != nil {
		ctrlDir = "/var/run/wpa_supplicant"
	}
	// Construir comando wpa_cli estándar
	wpaCliCmd := fmt.Sprintf("sudo wpa_cli -i %s -p %s", interfaceName, ctrlDir)
	log.Printf("Usando wpa_cli con ctrlDir=%s e interfaz=%s", ctrlDir, interfaceName)

	// Verificar si la red ya existe
	listCmd := fmt.Sprintf("%s list_networks", wpaCliCmd)
	listOut, _ := executeCommand(listCmd)
	networkExists := false
	networkID := ""

	if strings.Contains(listOut, ssid) {
		// La red existe, extraer ID
		lines := strings.Split(listOut, "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) > 0 && strings.Contains(line, ssid) {
				networkID = fields[0]
				networkExists = true
				log.Printf("Red %s ya existe con ID: %s", ssid, networkID)
				break
			}
		}
	}

	if !networkExists {
		// Agregar nueva red
		log.Printf("Agregando nueva red: %s", ssid)
		addCmd := fmt.Sprintf("%s add_network", wpaCliCmd)
		addOut, _ := executeCommand(addCmd)
		networkID = strings.TrimSpace(addOut)
		log.Printf("Red agregada con ID: %s", networkID)

		// Configurar SSID
		setSsidCmd := fmt.Sprintf("%s set_network %s ssid '\"%s\"'", wpaCliCmd, networkID, ssid)
		ssidResult, _ := executeCommand(setSsidCmd)
		log.Printf("SSID set result: %s", strings.TrimSpace(ssidResult))

		// Configurar seguridad
		if password != "" {
			setPskCmd := fmt.Sprintf("%s set_network %s psk '\"%s\"'", wpaCliCmd, networkID, password)
			pskResult, _ := executeCommand(setPskCmd)
			log.Printf("PSK set result: %s", strings.TrimSpace(pskResult))
			setKeyMgmtCmd := fmt.Sprintf("%s set_network %s key_mgmt WPA-PSK", wpaCliCmd, networkID)
			keyMgmtResult, _ := executeCommand(setKeyMgmtCmd)
			log.Printf("Key management set result: %s", strings.TrimSpace(keyMgmtResult))
		} else {
			setKeyMgmtCmd := fmt.Sprintf("%s set_network %s key_mgmt NONE", wpaCliCmd, networkID)
			keyMgmtResult, _ := executeCommand(setKeyMgmtCmd)
			log.Printf("Key management (NONE) set result: %s", strings.TrimSpace(keyMgmtResult))
		}

		// Habilitar la red
		enableCmd := fmt.Sprintf("%s enable_network %s", wpaCliCmd, networkID)
		executeCommand(enableCmd)

		// Guardar configuración
		saveCmd := fmt.Sprintf("%s save_config", wpaCliCmd)
		executeCommand(saveCmd)
		log.Printf("Configuración guardada en wpa_supplicant")
	} else {
		// La red ya existe, habilitarla
		log.Printf("Habilitando red existente: %s", ssid)
		enableCmd := fmt.Sprintf("%s enable_network %s", wpaCliCmd, networkID)
		executeCommand(enableCmd)

		// Si hay una contraseña nueva, actualizarla
		if password != "" {
			setPskCmd := fmt.Sprintf("%s set_network %s psk '\"%s\"'", wpaCliCmd, networkID, password)
			executeCommand(setPskCmd)
			saveCmd := fmt.Sprintf("%s save_config", wpaCliCmd)
			executeCommand(saveCmd)
		}
	}

	// Deshabilitar todas las redes primero
	executeCommand(fmt.Sprintf("%s disable_network all", wpaCliCmd))

	// Intentar conectar
	selectCmd := fmt.Sprintf("%s select_network %s", wpaCliCmd, networkID)
	selectResult, _ := executeCommand(selectCmd)
	log.Printf("select_network result: %s", strings.TrimSpace(selectResult))

	// Habilitar la red específica
	enableCmd := fmt.Sprintf("%s enable_network %s", wpaCliCmd, networkID)
	enableResult, _ := executeCommand(enableCmd)
	log.Printf("enable_network result: %s", strings.TrimSpace(enableResult))

	// Verificar que wpa_cli esté respondiendo (con múltiples intentos y métodos alternativos)
	pingSuccess := false
	
	for pingAttempt := 0; pingAttempt < 8; pingAttempt++ {
		// Intentar con el comando configurado
		pingCmd := fmt.Sprintf("%s ping", wpaCliCmd)
		pingOut, _ := executeCommand(pingCmd)
		if strings.Contains(pingOut, "PONG") {
			pingSuccess = true
			log.Printf("✅ wpa_cli responde correctamente (intento %d)", pingAttempt+1)
			break
		} else {
			log.Printf("⚠️  wpa_cli no responde (intento %d/8): %s", pingAttempt+1, strings.TrimSpace(pingOut))
			
			// Si falla, intentar método alternativo solo cambiando a -i (sin -p)
			if pingAttempt == 4 {
				// Intentar con -i interfaceName como último recurso
				log.Printf("Intentando método alternativo: wpa_cli -i %s...", interfaceName)
				altCmd := fmt.Sprintf("sudo wpa_cli -i %s ping", interfaceName)
				if altOut, _ := executeCommand(altCmd); strings.Contains(altOut, "PONG") {
					pingSuccess = true
					wpaCliCmd = fmt.Sprintf("sudo wpa_cli -i %s", interfaceName)
					log.Printf("✅ Método alternativo con -i funcionó")
					break
				}
			}
			
			// No ajustar permisos globales aquí para no afectar otras interfaces/servicios.
			
			if pingAttempt < 7 {
				time.Sleep(1 * time.Second)
			}
		}
	}
	
	if !pingSuccess {
		log.Printf("❌ ERROR: wpa_cli no está respondiendo después de 8 intentos")
		// Intentar diagnosticar el problema
		wpaPidCheck, _ := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
		if strings.TrimSpace(string(wpaPidCheck)) == "" {
			result["success"] = false
			result["error"] = "wpa_supplicant no está corriendo. Por favor, reinicia el servicio WiFi."
			return result
		} else {
			// wpa_supplicant está corriendo pero wpa_cli no responde
			// Diagnosticar el problema más a fondo
			log.Printf("wpa_supplicant está corriendo (PID: %s) pero wpa_cli no responde", strings.TrimSpace(string(wpaPidCheck)))
			
			// Verificar permisos del socket y del directorio
			socketPath1 := fmt.Sprintf("/var/run/wpa_supplicant/%s", interfaceName)
			socketPath2 := fmt.Sprintf("/run/wpa_supplicant/%s", interfaceName)
			
			// Verificar qué socket existe y sus permisos
			checkPerms1 := exec.Command("sh", "-c", fmt.Sprintf("ls -la %s 2>/dev/null || echo 'not found'", socketPath1))
			checkPerms2 := exec.Command("sh", "-c", fmt.Sprintf("ls -la %s 2>/dev/null || echo 'not found'", socketPath2))
			if permsOut1, _ := checkPerms1.Output(); len(permsOut1) > 0 {
				log.Printf("Permisos del socket %s: %s", socketPath1, strings.TrimSpace(string(permsOut1)))
			}
			if permsOut2, _ := checkPerms2.Output(); len(permsOut2) > 0 {
				log.Printf("Permisos del socket %s: %s", socketPath2, strings.TrimSpace(string(permsOut2)))
			}
			
			// Verificar grupos del usuario
			groupsCmd := exec.Command("sh", "-c", "groups")
			if groupsOut, _ := groupsCmd.Output(); len(groupsOut) > 0 {
				log.Printf("Grupos del usuario actual: %s", strings.TrimSpace(string(groupsOut)))
			}
			
			// Intentar ajustar permisos una vez más antes de reiniciar
			log.Printf("Intentando ajustar permisos del socket como último recurso...")
			executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath1))
			executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath2))
			executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
			executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
			executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
			executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
			executeCommand("sudo chmod 666 /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chmod 666 /run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chgrp netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chown root:netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
			
			time.Sleep(500 * time.Millisecond)
			
			// Intentar ping una vez más después de ajustar permisos
			finalPingCmd := fmt.Sprintf("sudo wpa_cli -i %s ping 2>&1", interfaceName)
			if finalPingOut, _ := executeCommand(finalPingCmd); strings.Contains(finalPingOut, "PONG") {
				log.Printf("✅ wpa_cli responde después de ajustar permisos, continuando...")
				// Continuar con el proceso de conexión
				pingSuccess = true
			} else {
				log.Printf("❌ wpa_cli aún no responde después de ajustar permisos")
				log.Printf("Reiniciando wpa_supplicant automáticamente...")
				// Reiniciar wpa_supplicant automáticamente
				executeCommand(fmt.Sprintf("sudo pkill -9 -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
				executeCommand("sudo pkill -9 wpa_supplicant 2>/dev/null || true")
				time.Sleep(2 * time.Second)
				// Limpiar sockets
				executeCommand(fmt.Sprintf("sudo rm -rf /var/run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
				executeCommand(fmt.Sprintf("sudo rm -rf /run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
				// No borrar sockets globales; puede afectar otras interfaces/servicios.
				
				// Reiniciar wpa_supplicant con configuración limpia
				log.Printf("Reiniciando wpa_supplicant con configuración limpia...")
				wpaConfig := fmt.Sprintf("/etc/wpa_supplicant/wpa_supplicant-%s.conf", interfaceName)
				if _, err := os.Stat(wpaConfig); os.IsNotExist(err) {
					wpaConfig = "/etc/wpa_supplicant/wpa_supplicant.conf"
				}
				
				// Preparar directorios con permisos correctos
				executeCommand("sudo mkdir -p /var/run/wpa_supplicant 2>/dev/null || true")
				executeCommand("sudo mkdir -p /run/wpa_supplicant 2>/dev/null || true")
				executeCommand("sudo chmod 775 /var/run/wpa_supplicant 2>/dev/null || true")
				executeCommand("sudo chmod 775 /run/wpa_supplicant 2>/dev/null || true")
				executeCommand("sudo chgrp netdev /var/run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant 2>/dev/null || true")
				executeCommand("sudo chgrp netdev /run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant 2>/dev/null || true")
				
				// Iniciar wpa_supplicant con directorio de control explícito (-C)
				ctrlDir := "/run/wpa_supplicant"
				if _, err := os.Stat(ctrlDir); err != nil {
					ctrlDir = "/var/run/wpa_supplicant"
				}
				startCmd := fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext -C %s", interfaceName, wpaConfig, ctrlDir)
				startOut, _ := executeCommand(startCmd)
				log.Printf("wpa_supplicant reiniciado: %s", strings.TrimSpace(startOut))
				
				// Esperar a que el socket esté listo
				time.Sleep(3 * time.Second)
				
				// Ajustar permisos del socket
				socketPath1 := fmt.Sprintf("/var/run/wpa_supplicant/%s", interfaceName)
				socketPath2 := fmt.Sprintf("/run/wpa_supplicant/%s", interfaceName)
				executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath1))
				executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath2))
				executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath1, socketPath1))
				executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath2, socketPath2))
				executeCommand("sudo chmod 666 /var/run/wpa_supplicant/* 2>/dev/null || true")
				executeCommand("sudo chmod 666 /run/wpa_supplicant/* 2>/dev/null || true")
				executeCommand("sudo chgrp netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
				
				time.Sleep(1 * time.Second)
				
				// Verificar que wpa_cli responde ahora usando el directorio de control estándar
				ctrlDir = "/run/wpa_supplicant"
				if _, err := os.Stat(ctrlDir); err != nil {
					ctrlDir = "/var/run/wpa_supplicant"
				}
				finalTestCmd := fmt.Sprintf("sudo wpa_cli -i %s -p %s ping 2>&1", interfaceName, ctrlDir)
				if finalTestOut, _ := executeCommand(finalTestCmd); strings.Contains(finalTestOut, "PONG") {
					log.Printf("✅ wpa_supplicant reiniciado exitosamente y wpa_cli responde")
					pingSuccess = true
					// Actualizar wpaCliCmd para usar el método que funciona
					wpaCliCmd = fmt.Sprintf("sudo wpa_cli -i %s -p %s", interfaceName, ctrlDir)
				} else {
					log.Printf("❌ ERROR CRÍTICO: wpa_supplicant reiniciado pero wpa_cli aún no responde")
					result["success"] = false
					result["error"] = "No se pudo establecer comunicación con wpa_supplicant después de reiniciar. Verifica la configuración del sistema."
					return result
				}
			}
		}
	}

	// Reconectar explícitamente
	reconnectOut, _ := executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
	log.Printf("reconnect result: %s", strings.TrimSpace(reconnectOut))
	time.Sleep(2 * time.Second)

	// Verificar el estado de la conexión (con múltiples intentos mejorados)
	connected := false
	statusOutput := ""
	maxAttempts := 15 // Aumentado de 8 a 15
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
					executeCommand(fmt.Sprintf("%s disable_network %s", wpaCliCmd, networkID))
					time.Sleep(1 * time.Second)
					executeCommand(fmt.Sprintf("%s enable_network %s", wpaCliCmd, networkID))
					executeCommand(fmt.Sprintf("%s reconnect", wpaCliCmd))
				}
			}
		}

		// Si no hay progreso después de varios intentos, intentar reconectar
		if attempt > 5 && !connected && (currentState == "DISCONNECTED" || currentState == "INACTIVE" || currentState == "") {
			log.Printf("Sin progreso después de %d intentos, forzando reconexión...", attempt+1)
			executeCommand(fmt.Sprintf("%s disable_network all", wpaCliCmd))
			time.Sleep(1 * time.Second)
			executeCommand(fmt.Sprintf("%s enable_network %s", wpaCliCmd, networkID))
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
