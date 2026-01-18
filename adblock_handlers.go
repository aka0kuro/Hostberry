package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getAdBlockStatus() map[string]interface{} {
	result := make(map[string]interface{})

	dnsmasqCmd := exec.Command("sh", "-c", "systemctl is-active dnsmasq 2>/dev/null || echo inactive")
	dnsmasqOut, _ := dnsmasqCmd.Output()
	dnsmasqStatus := strings.TrimSpace(string(dnsmasqOut))
	if dnsmasqStatus == "" {
		dnsmasqStatus = "inactive"
	}

	piholeCmd := exec.Command("sh", "-c", "systemctl is-active pihole-FTL 2>/dev/null || echo inactive")
	piholeOut, _ := piholeCmd.Output()
	piholeStatus := strings.TrimSpace(string(piholeOut))
	if piholeStatus == "" {
		piholeStatus = "inactive"
	}

	// Verificar DNSCrypt
	dnscryptCmd := exec.Command("sh", "-c", "systemctl is-active dnscrypt-proxy 2>/dev/null || echo inactive")
	dnscryptOut, _ := dnscryptCmd.Output()
	dnscryptStatus := strings.TrimSpace(string(dnscryptOut))
	if dnscryptStatus == "" {
		dnscryptStatus = "inactive"
	}

	// Verificar si dnscrypt-proxy está instalado
	dnscryptInstalled := false
	if checkCmd := exec.Command("sh", "-c", "command -v dnscrypt-proxy 2>/dev/null"); checkCmd.Run() == nil {
		dnscryptInstalled = true
	}

	result["active"] = dnsmasqStatus == "active" || piholeStatus == "active" || dnscryptStatus == "active"
	result["type"] = "none"

	if dnscryptStatus == "active" {
		result["type"] = "dnscrypt"
	} else if dnsmasqStatus == "active" {
		result["type"] = "dnsmasq"
	} else if piholeStatus == "active" {
		result["type"] = "pihole"
	}

	result["dnscrypt_installed"] = dnscryptInstalled
	result["dnscrypt_active"] = dnscryptStatus == "active"

	if result["active"] == true {
		if hostsContent, err := os.ReadFile("/etc/hosts"); err == nil {
			blockedCount := strings.Count(string(hostsContent), "0.0.0.0")
			result["blocked_domains"] = blockedCount
		} else {
			result["blocked_domains"] = 0
		}
	} else {
		result["blocked_domains"] = 0
	}

	result["success"] = true
	return result
}

func enableAdBlock(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.adblock_enabling", user)

	dnsmasqCmd := "sudo systemctl start dnsmasq"
	if _, err := executeCommand(dnsmasqCmd); err != nil {
		piholeCmd := "sudo systemctl start pihole-FTL"
		if out2, err2 := executeCommand(piholeCmd); err2 != nil {
			result["success"] = false
			result["error"] = err2.Error()
			if out2 != "" {
				result["error"] = strings.TrimSpace(out2)
			}
			result["message"] = "Error iniciando servicio AdBlock"
			LogTf("logs.adblock_enable_error", err2)
			return result
		}
	}

	result["success"] = true
	result["message"] = "AdBlock habilitado"
	LogT("logs.adblock_enabled")
	return result
}

func disableAdBlock(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.adblock_disabling", user)

	executeCommand("sudo systemctl stop dnsmasq")
	executeCommand("sudo systemctl stop pihole-FTL")
	executeCommand("sudo systemctl stop dnscrypt-proxy")

	result["success"] = true
	result["message"] = "AdBlock deshabilitado"
	LogT("logs.adblock_disabled")
	return result
}

// Funciones para DNSCrypt
func getDNSCryptStatus() map[string]interface{} {
	result := make(map[string]interface{})

	// Verificar si está instalado
	checkCmd := exec.Command("sh", "-c", "command -v dnscrypt-proxy 2>/dev/null")
	installed := checkCmd.Run() == nil
	result["installed"] = installed

	if !installed {
		result["active"] = false
		result["success"] = true
		return result
	}

	// Verificar estado del servicio
	statusCmd := exec.Command("sh", "-c", "systemctl is-active dnscrypt-proxy 2>/dev/null || echo inactive")
	statusOut, _ := statusCmd.Output()
	status := strings.TrimSpace(string(statusOut))
	result["active"] = status == "active"
	result["status"] = status

	// Verificar si está habilitado para iniciar al arranque
	enabledCmd := exec.Command("sh", "-c", "systemctl is-enabled dnscrypt-proxy 2>/dev/null || echo disabled")
	enabledOut, _ := enabledCmd.Output()
	enabled := strings.TrimSpace(string(enabledOut))
	result["enabled"] = enabled == "enabled"

	// Leer configuración si existe
	configPath := "/etc/dnscrypt-proxy/dnscrypt-proxy.toml"
	if _, err := os.Stat(configPath); err == nil {
		result["config_exists"] = true
		result["config_path"] = configPath
	} else {
		result["config_exists"] = false
	}

	// Verificar qué servidor está usando
	if result["active"] == true {
		logCmd := exec.Command("sh", "-c", "journalctl -u dnscrypt-proxy -n 10 --no-pager 2>/dev/null | grep -i 'server' | tail -1")
		if logOut, err := logCmd.Output(); err == nil {
			logLine := strings.TrimSpace(string(logOut))
			if logLine != "" {
				result["current_server"] = logLine
			}
		}
	}

	result["success"] = true
	return result
}

func installDNSCrypt(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.dnscrypt_installing", user)

	// Verificar si ya está instalado
	checkCmd := exec.Command("sh", "-c", "command -v dnscrypt-proxy 2>/dev/null")
	if checkCmd.Run() == nil {
		result["success"] = true
		result["message"] = "DNSCrypt ya está instalado"
		result["already_installed"] = true
		return result
	}

	// Intentar instalar dnscrypt-proxy
	// Primero intentar con apt (Debian/Ubuntu)
	installCmd := "sudo apt-get update && sudo apt-get install -y dnscrypt-proxy"
	if out, err := executeCommand(installCmd); err != nil {
		// Si falla, intentar compilar desde fuente o usar otro método
		LogTf("logs.dnscrypt_install_error", err)
		result["success"] = false
		result["error"] = fmt.Sprintf("Error instalando DNSCrypt: %v", err)
		if out != "" {
			result["error"] = strings.TrimSpace(out)
		}
		return result
	}

	result["success"] = true
	result["message"] = "DNSCrypt instalado correctamente"
	LogT("logs.dnscrypt_installed")
	return result
}

func configureDNSCrypt(serverName string, blockAds bool, user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.dnscrypt_configuring", user)

	// Verificar si está instalado
	checkCmd := exec.Command("sh", "-c", "command -v dnscrypt-proxy 2>/dev/null")
	if checkCmd.Run() != nil {
		result["success"] = false
		result["error"] = "DNSCrypt no está instalado. Instálalo primero."
		return result
	}

	configDir := "/etc/dnscrypt-proxy"
	configPath := filepath.Join(configDir, "dnscrypt-proxy.toml")

	// Crear directorio si no existe
	executeCommand(fmt.Sprintf("sudo mkdir -p %s", configDir))

	// Servidores DNSCrypt recomendados que filtran publicidad
	servers := map[string]string{
		"adguard-dns":     "sdns://AQMAAAAAAAAAFDE3Ni4xMDMuMTMwLjEzMDo1NDQzINErR_JS3PLCu_iZEIbq95zkSV2LFsigxDIuUso_OQhzIjIuZG5zY3J5cHQuZGVmYXVsdC5uczEuYWRndWFyZC5jb20",
		"adguard-dns-ipv6": "sdns://AQMAAAAAAAAAGVsyYTAwOjVhODA6MjIwMDo6XTo1NDQzINErR_JS3PLCu_iZEIbq95zkSV2LFsigxDIuUso_OQhzIjIuZG5zY3J5cHQuZGVmYXVsdC5uczEuYWRndWFyZC5jb20",
		"quad9-dnscrypt":  "sdns://AQMAAAAAAAAADTkuOS45Ljk6ODQ0MyBnyEe4yHWM0SAkVUO-dWdG3zTfHYTAC4xHA2jfgh2GPhkyLmRuc2NyeXB0LXByb3h5LnF1YWQ5Lm5ldA",
		"cloudflare":      "sdns://AgcAAAAAAAAABzEuMC4wLjEAEmRucy5jbG91ZGZsYXJlLmNvbQovZG5zLXF1ZXJ5",
	}

	serverSDNS := servers["adguard-dns"] // Por defecto AdGuard que filtra publicidad
	if serverName != "" && servers[serverName] != "" {
		serverSDNS = servers[serverName]
	}

	// Configuración básica de dnscrypt-proxy.toml
	configContent := fmt.Sprintf(`# Configuración DNSCrypt para HostBerry
# Generado automáticamente

listen_addresses = ['127.0.0.1:53', '[::1]:53']

# Servidor DNSCrypt
server_names = ['%s']

# Si no se especifica servidor, usar lista pública
[static]
  [static.'%s']
    stamp = '%s'

# Bloquear dominios de publicidad si está habilitado
%s

# Logs
log_file = '/var/log/dnscrypt-proxy/dnscrypt-proxy.log'
log_level = 2

# Cache
cache = true
cache_size = 4096
cache_min_ttl = 2400
cache_max_ttl = 86400

# Consultas simultáneas
max_clients = 250

# Timeouts
timeout = 5000
`, serverName, serverName, serverSDNS, func() string {
		if blockAds {
			return `# Lista de bloqueo de publicidad
[blocked_names]
  blocked_names_file = '/etc/dnscrypt-proxy/blocklist.txt'
  log_file = '/var/log/dnscrypt-proxy/blocked.log'
  log_format = 'tsv'
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
		LogTf("logs.dnscrypt_config_error", err)
		return result
	}

	// Si blockAds está habilitado, crear lista de bloqueo básica
	if blockAds {
		blocklistPath := "/etc/dnscrypt-proxy/blocklist.txt"
		blocklistContent := `# Lista de bloqueo de publicidad y rastreadores
# Dominios comunes de publicidad
ads.*
advertising.*
tracking.*
analytics.*
doubleclick.*
googleadservices.*
googlesyndication.*
`

		writeBlocklistCmd := fmt.Sprintf("sudo tee %s > /dev/null", blocklistPath)
		blocklistCmd := exec.Command("sh", "-c", writeBlocklistCmd)
		blocklistCmd.Stdin = strings.NewReader(blocklistContent)
		blocklistCmd.Run() // Ignorar errores
	}

	// Crear directorio de logs
	executeCommand("sudo mkdir -p /var/log/dnscrypt-proxy")

	result["success"] = true
	result["message"] = "DNSCrypt configurado correctamente"
	LogT("logs.dnscrypt_configured")
	return result
}

func enableDNSCrypt(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.dnscrypt_enabling", user)

	// Verificar si está instalado
	checkCmd := exec.Command("sh", "-c", "command -v dnscrypt-proxy 2>/dev/null")
	if checkCmd.Run() != nil {
		result["success"] = false
		result["error"] = "DNSCrypt no está instalado. Instálalo primero."
		return result
	}

	// Verificar configuración
	configPath := "/etc/dnscrypt-proxy/dnscrypt-proxy.toml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Configurar con valores por defecto
		configResult := configureDNSCrypt("adguard-dns", true, user)
		if success, ok := configResult["success"].(bool); !ok || !success {
			result["success"] = false
			result["error"] = "Error configurando DNSCrypt antes de iniciarlo"
			if errMsg, ok := configResult["error"].(string); ok {
				result["error"] = errMsg
			}
			return result
		}
	}

	// Iniciar servicio
	startCmd := "sudo systemctl start dnscrypt-proxy"
	if out, err := executeCommand(startCmd); err != nil {
		result["success"] = false
		result["error"] = fmt.Sprintf("Error iniciando DNSCrypt: %v", err)
		if out != "" {
			result["error"] = strings.TrimSpace(out)
		}
		LogTf("logs.dnscrypt_start_error", err)
		return result
	}

	// Habilitar para iniciar al arranque
	executeCommand("sudo systemctl enable dnscrypt-proxy")

	// Configurar sistema para usar DNSCrypt como DNS local
	// Cambiar /etc/resolv.conf para apuntar a 127.0.0.1
	resolvConf := "/etc/resolv.conf"
	backupCmd := fmt.Sprintf("sudo cp %s %s.backup 2>/dev/null || true", resolvConf, resolvConf)
	executeCommand(backupCmd)

	// Leer resolv.conf actual
	content, err := os.ReadFile(resolvConf)
	newLines := []string{}
	if err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "nameserver") {
				newLines = append(newLines, line)
			}
		}
	}

	// Agregar nameserver local
	newLines = append(newLines, "nameserver 127.0.0.1")
	newLines = append(newLines, "nameserver ::1")

	newContent := strings.Join(newLines, "\n")
	writeCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", resolvConf))
	writeCmd.Stdin = strings.NewReader(newContent)
	if err := writeCmd.Run(); err != nil {
		LogTf("logs.dnscrypt_resolv_error", err)
		// No es crítico, continuar
	}

	// Reiniciar systemd-resolved si existe
	executeCommand("sudo systemctl restart systemd-resolved 2>/dev/null || true")

	result["success"] = true
	result["message"] = "DNSCrypt habilitado correctamente"
	LogT("logs.dnscrypt_enabled")
	return result
}

func disableDNSCrypt(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.dnscrypt_disabling", user)

	// Detener servicio
	executeCommand("sudo systemctl stop dnscrypt-proxy")
	executeCommand("sudo systemctl disable dnscrypt-proxy")

	// Restaurar /etc/resolv.conf si existe backup
	resolvConf := "/etc/resolv.conf"
	backupPath := resolvConf + ".backup"
	if _, err := os.Stat(backupPath); err == nil {
		restoreCmd := fmt.Sprintf("sudo cp %s %s", backupPath, resolvConf)
		executeCommand(restoreCmd)
	} else {
		// Si no hay backup, usar DNS públicos por defecto
		content := "nameserver 8.8.8.8\nnameserver 8.8.4.4\n"
		writeCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", resolvConf))
		writeCmd.Stdin = strings.NewReader(content)
		writeCmd.Run()
	}

	// Reiniciar systemd-resolved si existe
	executeCommand("sudo systemctl restart systemd-resolved 2>/dev/null || true")

	result["success"] = true
	result["message"] = "DNSCrypt deshabilitado correctamente"
	LogT("logs.dnscrypt_disabled")
	return result
}
