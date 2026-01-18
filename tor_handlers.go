package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Funciones para Tor
func getTorStatus() map[string]interface{} {
	result := make(map[string]interface{})

	// Verificar si está instalado
	checkCmd := exec.Command("sh", "-c", "command -v tor 2>/dev/null")
	installed := checkCmd.Run() == nil
	result["installed"] = installed

	if !installed {
		result["active"] = false
		result["success"] = true
		return result
	}

	// Verificar estado del servicio
	statusCmd := exec.Command("sh", "-c", "systemctl is-active tor 2>/dev/null || echo inactive")
	statusOut, _ := statusCmd.Output()
	status := strings.TrimSpace(string(statusOut))
	result["active"] = status == "active"
	result["status"] = status

	// Verificar si está habilitado para iniciar al arranque
	enabledCmd := exec.Command("sh", "-c", "systemctl is-enabled tor 2>/dev/null || echo disabled")
	enabledOut, _ := enabledCmd.Output()
	enabled := strings.TrimSpace(string(enabledOut))
	result["enabled"] = enabled == "enabled"

	// Leer configuración si existe
	configPath := "/etc/tor/torrc"
	if _, err := os.Stat(configPath); err == nil {
		result["config_exists"] = true
		result["config_path"] = configPath
	} else {
		result["config_exists"] = false
	}

	// Verificar puerto SOCKS si está activo
	if result["active"] == true {
		// Intentar conectar al puerto SOCKS por defecto (9050)
		socksCheckCmd := exec.Command("sh", "-c", "netstat -tuln 2>/dev/null | grep ':9050' || ss -tuln 2>/dev/null | grep ':9050'")
		if socksOut, err := socksCheckCmd.Output(); err == nil {
			socksLine := strings.TrimSpace(string(socksOut))
			if socksLine != "" {
				result["socks_port"] = "9050"
				result["socks_listening"] = true
			} else {
				result["socks_listening"] = false
			}
		}
	}

	// Verificar IP actual a través de Tor (si está activo)
	if result["active"] == true && result["socks_listening"] == true {
		// Intentar obtener IP a través de Tor usando curl
		ipCheckCmd := exec.Command("sh", "-c", "curl -s --socks5-hostname 127.0.0.1:9050 https://api.ipify.org 2>/dev/null || echo ''")
		if ipOut, err := ipCheckCmd.Output(); err == nil {
			ip := strings.TrimSpace(string(ipOut))
			if ip != "" && !strings.Contains(ip, "error") {
				result["tor_ip"] = ip
			}
		}
	}

	result["success"] = true
	return result
}

func installTor(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.tor_installing", user)

	// Verificar si ya está instalado
	checkCmd := exec.Command("sh", "-c", "command -v tor 2>/dev/null")
	if checkCmd.Run() == nil {
		result["success"] = true
		result["message"] = "Tor ya está instalado"
		result["already_installed"] = true
		return result
	}

	// Intentar instalar Tor
	// Primero intentar con apt (Debian/Ubuntu)
	installCmd := "sudo apt-get update && sudo apt-get install -y tor"
	if out, err := executeCommand(installCmd); err != nil {
		// Si falla, intentar con otros métodos
		LogTf("logs.tor_install_error", err)
		result["success"] = false
		result["error"] = fmt.Sprintf("Error instalando Tor: %v", err)
		if out != "" {
			result["error"] = strings.TrimSpace(out)
		}
		return result
	}

	result["success"] = true
	result["message"] = "Tor instalado correctamente"
	LogT("logs.tor_installed")
	return result
}

func configureTor(enableSocks bool, socksPort int, enableControlPort bool, controlPort int, enableHiddenService bool, user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.tor_configuring", user)

	// Verificar si está instalado
	checkCmd := exec.Command("sh", "-c", "command -v tor 2>/dev/null")
	if checkCmd.Run() != nil {
		result["success"] = false
		result["error"] = "Tor no está instalado. Instálalo primero."
		return result
	}

	configDir := "/etc/tor"
	configPath := filepath.Join(configDir, "torrc")

	// Crear directorio si no existe
	executeCommand(fmt.Sprintf("sudo mkdir -p %s", configDir))

	// Valores por defecto
	if socksPort == 0 {
		socksPort = 9050
	}
	if controlPort == 0 {
		controlPort = 9051
	}

	// Configuración básica de torrc
	configContent := fmt.Sprintf(`# Configuración Tor para HostBerry
# Generado automáticamente

# Directorio de datos
DataDirectory /var/lib/tor

# Logs
Log notice file /var/log/tor/tor.log

# SOCKS Proxy
%sSocksPort %d
SocksPolicy accept 127.0.0.1
SocksPolicy reject *

# Control Port (para control remoto)
%sControlPort %d
CookieAuthentication 1

# Evitar que Tor use ciertos puertos
DisableDebuggerAttachment 1

# Configuración de rendimiento
NumEntryGuards 3
NumEntryGuards 3
CircuitBuildTimeout 10
KeepalivePeriod 60
NewCircuitPeriod 30

# Configuración de seguridad
SafeLogging 1
AvoidDiskWrites 0

# Servicios ocultos (opcional)
%s
`, func() string {
		if enableSocks {
			return ""
		}
		return "# "
	}(), socksPort, func() string {
		if enableControlPort {
			return ""
		}
		return "# "
	}(), controlPort, func() string {
		if enableHiddenService {
			return `# Servicio oculto (ejemplo)
# HiddenServiceDir /var/lib/tor/hidden_service/
# HiddenServicePort 80 127.0.0.1:80
`
		}
		return ""
	}())

	// Escribir configuración
	writeCmd := fmt.Sprintf("sudo tee %s > /dev/null", configPath)
	cmd := exec.Command("sh", "-c", writeCmd)
	cmd.Stdin = strings.NewReader(configContent)
	if err := cmd.Run(); err != nil {
		result["success"] = false
		result["error"] = fmt.Sprintf("Error escribiendo configuración: %v", err)
		LogTf("logs.tor_config_error", err)
		return result
	}

	// Crear directorio de datos y logs
	executeCommand("sudo mkdir -p /var/lib/tor")
	executeCommand("sudo mkdir -p /var/log/tor")
	executeCommand("sudo chown debian-tor:debian-tor /var/lib/tor /var/log/tor 2>/dev/null || sudo chown tor:tor /var/lib/tor /var/log/tor 2>/dev/null || true")

	result["success"] = true
	result["message"] = "Tor configurado correctamente"
	LogT("logs.tor_configured")
	return result
}

func enableTor(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.tor_enabling", user)

	// Verificar si está instalado
	checkCmd := exec.Command("sh", "-c", "command -v tor 2>/dev/null")
	if checkCmd.Run() != nil {
		result["success"] = false
		result["error"] = "Tor no está instalado. Instálalo primero."
		return result
	}

	// Verificar configuración
	configPath := "/etc/tor/torrc"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Configurar con valores por defecto
		configResult := configureTor(true, 9050, true, 9051, false, user)
		if success, ok := configResult["success"].(bool); !ok || !success {
			result["success"] = false
			result["error"] = "Error configurando Tor antes de iniciarlo"
			if errMsg, ok := configResult["error"].(string); ok {
				result["error"] = errMsg
			}
			return result
		}
	}

	// Iniciar servicio
	startCmd := "sudo systemctl start tor"
	if out, err := executeCommand(startCmd); err != nil {
		result["success"] = false
		result["error"] = fmt.Sprintf("Error iniciando Tor: %v", err)
		if out != "" {
			result["error"] = strings.TrimSpace(out)
		}
		LogTf("logs.tor_start_error", err)
		return result
	}

	// Habilitar para iniciar al arranque
	executeCommand("sudo systemctl enable tor")

	// Esperar un poco para que Tor se inicie
	time.Sleep(2 * time.Second)

	result["success"] = true
	result["message"] = "Tor habilitado correctamente"
	LogT("logs.tor_enabled")
	return result
}

func disableTor(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.tor_disabling", user)

	// Detener servicio
	executeCommand("sudo systemctl stop tor")
	executeCommand("sudo systemctl disable tor")

	result["success"] = true
	result["message"] = "Tor deshabilitado correctamente"
	LogT("logs.tor_disabled")
	return result
}

func getTorCircuitInfo() map[string]interface{} {
	result := make(map[string]interface{})

	// Verificar si Tor está activo
	statusCmd := exec.Command("sh", "-c", "systemctl is-active tor 2>/dev/null || echo inactive")
	statusOut, _ := statusCmd.Output()
	status := strings.TrimSpace(string(statusOut))
	
	if status != "active" {
		result["active"] = false
		result["success"] = true
		return result
	}

	// Intentar obtener información del circuito usando control port
	// Necesitamos usar tor control para obtener información del circuito
	controlCmd := exec.Command("sh", "-c", "echo 'GETINFO circuit-status' | nc 127.0.0.1 9051 2>/dev/null || echo ''")
	if controlOut, err := controlCmd.Output(); err == nil {
		controlOutput := strings.TrimSpace(string(controlOut))
		if controlOutput != "" {
			result["circuit_info"] = controlOutput
		}
	}

	// Intentar obtener IP a través de Tor
	ipCheckCmd := exec.Command("sh", "-c", "curl -s --socks5-hostname 127.0.0.1:9050 https://api.ipify.org 2>/dev/null || echo ''")
	if ipOut, err := ipCheckCmd.Output(); err == nil {
		ip := strings.TrimSpace(string(ipOut))
		if ip != "" && !strings.Contains(ip, "error") {
			result["tor_ip"] = ip
		}
	}

	result["active"] = true
	result["success"] = true
	return result
}
