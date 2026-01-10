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

	// Detener hostapd si está corriendo
	hostapdRunning, _ := exec.Command("sh", "-c", "pgrep hostapd 2>/dev/null").Output()
	if strings.TrimSpace(string(hostapdRunning)) != "" {
		log.Printf("Deteniendo hostapd para liberar la interfaz WiFi")
		executeCommand("sudo systemctl stop hostapd 2>/dev/null || true")
		executeCommand("sudo pkill hostapd 2>/dev/null || true")
		time.Sleep(2 * time.Second)
	}

	// Asegurar que la interfaz esté en modo managed
	iwInfoCmd := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s info 2>/dev/null", interfaceName))
	if iwInfoOut, err := iwInfoCmd.Output(); err == nil {
		if strings.Contains(string(iwInfoOut), "type AP") {
			log.Printf("Interfaz está en modo AP, cambiando a modo managed para conexión STA")
			executeCommand(fmt.Sprintf("sudo iw dev %s set type managed 2>/dev/null", interfaceName))
			time.Sleep(2 * time.Second)
		}
	}

	// Asegurar que la interfaz esté activa y no bloqueada
	executeCommand("sudo rfkill unblock wifi 2>/dev/null || true")
	executeCommand(fmt.Sprintf("sudo ip link set %s down 2>/dev/null", interfaceName))
	time.Sleep(1 * time.Second)
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null", interfaceName))
	time.Sleep(2 * time.Second)

	// Detener cualquier instancia de wpa_supplicant existente
	wpaPid, _ := exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
	if strings.TrimSpace(string(wpaPid)) != "" {
		log.Printf("Deteniendo wpa_supplicant existente para reiniciar limpiamente")
		executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
		time.Sleep(2 * time.Second)
	}

	// Asegurar que wpa_supplicant esté corriendo
	wpaPid, _ = exec.Command("sh", "-c", fmt.Sprintf("pgrep -f 'wpa_supplicant.*%s'", interfaceName)).Output()
	if strings.TrimSpace(string(wpaPid)) == "" {
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
		
		// Limpiar completamente wpa_supplicant y todos sus sockets antes de reiniciar
		log.Printf("Limpiando procesos y sockets de wpa_supplicant...")
		executeCommand(fmt.Sprintf("sudo pkill -9 -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
		executeCommand("sudo pkill -9 wpa_supplicant 2>/dev/null || true")
		time.Sleep(1 * time.Second) // Dar tiempo para que los procesos terminen
		
		// Limpiar todos los sockets posibles
		executeCommand(fmt.Sprintf("sudo rm -rf /var/run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
		executeCommand(fmt.Sprintf("sudo rm -rf /run/wpa_supplicant/%s 2>/dev/null || true", interfaceName))
		executeCommand("sudo rm -rf /var/run/wpa_supplicant/* 2>/dev/null || true")
		executeCommand("sudo rm -rf /run/wpa_supplicant/* 2>/dev/null || true")
		
		// Asegurar que los directorios existan con permisos correctos ANTES de iniciar wpa_supplicant
		executeCommand("sudo mkdir -p /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo mkdir -p /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chmod 775 /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /var/run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chgrp netdev /run/wpa_supplicant 2>/dev/null || sudo chgrp hostberry /run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chown root:netdev /var/run/wpa_supplicant 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant 2>/dev/null || true")
		executeCommand("sudo chown root:netdev /run/wpa_supplicant 2>/dev/null || sudo chown root:hostberry /run/wpa_supplicant 2>/dev/null || true")

		// Iniciar wpa_supplicant con el grupo correcto para que el socket tenga permisos adecuados
		// Usar -g para especificar el grupo del socket (netdev o hostberry)
		// También usar -u para especificar el usuario (root) y -g para el grupo (netdev)
		log.Printf("Iniciando wpa_supplicant con configuración de permisos...")
		startCmd := fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext -g netdev", interfaceName, wpaConfig)
		startOut, _ := executeCommand(startCmd)
		log.Printf("wpa_supplicant start output: %s", strings.TrimSpace(startOut))
		
		// Si falla con -g netdev, intentar sin especificar grupo (usará el del archivo de configuración)
		if strings.Contains(strings.ToLower(startOut), "error") || strings.Contains(strings.ToLower(startOut), "failed") {
			log.Printf("Intento con -g netdev falló, intentando sin especificar grupo...")
			startCmd = fmt.Sprintf("sudo wpa_supplicant -B -i %s -c %s -D nl80211,wext", interfaceName, wpaConfig)
			startOut, _ = executeCommand(startCmd)
			log.Printf("wpa_supplicant start output (sin -g): %s", strings.TrimSpace(startOut))
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

	// Buscar el socket real de wpa_supplicant y usar la ruta directa
	// Esto es más confiable que usar -i interfaceName
	socketPath := ""
	socketPath1 := fmt.Sprintf("/var/run/wpa_supplicant/%s", interfaceName)
	socketPath2 := fmt.Sprintf("/run/wpa_supplicant/%s", interfaceName)
	
	// Verificar qué socket existe realmente
	socketCheck1 := exec.Command("sh", "-c", fmt.Sprintf("test -S %s && echo 'exists' || echo 'not'", socketPath1))
	socketCheck2 := exec.Command("sh", "-c", fmt.Sprintf("test -S %s && echo 'exists' || echo 'not'", socketPath2))
	if socketOut1, _ := socketCheck1.Output(); strings.Contains(string(socketOut1), "exists") {
		socketPath = socketPath1
		log.Printf("Socket encontrado en: %s", socketPath)
		// Ajustar permisos inmediatamente
		executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath))
		executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath, socketPath))
		executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath, socketPath))
	} else if socketOut2, _ := socketCheck2.Output(); strings.Contains(string(socketOut2), "exists") {
		socketPath = socketPath2
		log.Printf("Socket encontrado en: %s", socketPath)
		// Ajustar permisos inmediatamente
		executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath))
		executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath, socketPath))
		executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath, socketPath))
	} else {
		// Si no encontramos el socket específico, buscar cualquier socket de wpa_supplicant
		findSocketCmd := exec.Command("sh", "-c", "find /var/run/wpa_supplicant /run/wpa_supplicant -type s -name '*' 2>/dev/null | head -1")
		if findOut, _ := findSocketCmd.Output(); len(findOut) > 0 {
			socketPath = strings.TrimSpace(string(findOut))
			log.Printf("Socket encontrado mediante búsqueda: %s", socketPath)
			// Ajustar permisos inmediatamente
			executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPath))
			executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPath, socketPath))
			executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPath, socketPath))
		} else {
			// Fallback: usar -i interfaceName (puede funcionar si wpa_cli encuentra el socket automáticamente)
			log.Printf("No se encontró socket específico, usando -i %s", interfaceName)
			socketPath = "" // Vacío significa usar -i
			// Ajustar permisos de todos los sockets posibles como medida preventiva
			executeCommand("sudo chmod 666 /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chmod 666 /run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chgrp netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
			executeCommand("sudo chown root:netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
		}
	}
	
	// Construir comando wpa_cli según si tenemos la ruta del socket o no
	var wpaCliCmd string
	if socketPath != "" {
		// Usar socket directamente con -p
		wpaCliCmd = fmt.Sprintf("sudo wpa_cli -p %s", socketPath)
		log.Printf("Usando wpa_cli con socket directo: %s", socketPath)
	} else {
		// Usar -i interfaceName como fallback
		wpaCliCmd = fmt.Sprintf("sudo wpa_cli -i %s", interfaceName)
		log.Printf("Usando wpa_cli con -i %s", interfaceName)
	}

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
	socketPathForPing := socketPath // Usar la ruta del socket encontrada anteriormente
	
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
			
			// Si falla, intentar métodos alternativos
			if pingAttempt == 2 && socketPathForPing != "" {
				// Intentar con socket directo usando -p
				log.Printf("Intentando método alternativo: wpa_cli -p %s...", socketPathForPing)
				altCmd := fmt.Sprintf("sudo wpa_cli -p %s ping", socketPathForPing)
				if altOut, _ := executeCommand(altCmd); strings.Contains(altOut, "PONG") {
					pingSuccess = true
					wpaCliCmd = fmt.Sprintf("sudo wpa_cli -p %s", socketPathForPing)
					log.Printf("✅ Método alternativo con -p funcionó")
					break
				}
			}
			
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
			
			// Ajustar permisos del socket nuevamente antes del siguiente intento
			if socketPathForPing != "" {
				executeCommand(fmt.Sprintf("sudo chmod 666 %s 2>/dev/null || true", socketPathForPing))
				executeCommand(fmt.Sprintf("sudo chgrp netdev %s 2>/dev/null || sudo chgrp hostberry %s 2>/dev/null || true", socketPathForPing, socketPathForPing))
				executeCommand(fmt.Sprintf("sudo chown root:netdev %s 2>/dev/null || sudo chown root:hostberry %s 2>/dev/null || true", socketPathForPing, socketPathForPing))
			} else {
				// Ajustar todos los sockets posibles
				executeCommand("sudo chmod 666 /var/run/wpa_supplicant/* 2>/dev/null || true")
				executeCommand("sudo chmod 666 /run/wpa_supplicant/* 2>/dev/null || true")
				executeCommand("sudo chgrp netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chgrp hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
				executeCommand("sudo chown root:netdev /var/run/wpa_supplicant/* 2>/dev/null || sudo chown root:hostberry /var/run/wpa_supplicant/* 2>/dev/null || true")
			}
			
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
			// Intentar reiniciar wpa_supplicant como último recurso
			log.Printf("wpa_supplicant está corriendo (PID: %s) pero wpa_cli no responde", strings.TrimSpace(string(wpaPidCheck)))
			log.Printf("Intentando reiniciar wpa_supplicant para corregir el socket...")
			executeCommand(fmt.Sprintf("sudo pkill -f 'wpa_supplicant.*%s' 2>/dev/null || true", interfaceName))
			time.Sleep(2 * time.Second)
			// El código continuará y reiniciará wpa_supplicant en la siguiente iteración
			result["success"] = false
			result["error"] = "wpa_cli no puede comunicarse con wpa_supplicant. Se intentará reiniciar wpa_supplicant. Por favor, intenta nuevamente."
			return result
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
