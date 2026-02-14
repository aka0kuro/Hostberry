package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TorConfigOptions agrupa todas las opciones de configuración de Tor (incl. estilo Onion Pi).
type TorConfigOptions struct {
	User                  string
	EnableSocks           bool
	SocksPort             int
	EnableControlPort     bool
	ControlPort           int
	EnableHiddenService   bool
	EnableTransPort       bool   // Proxy transparente (TransPort), estilo Onion Pi
	TransPort             int    // Por defecto 9040
	EnableDNSPort         bool   // DNS a través de Tor
	DNSPort               int    // Por defecto 53
	ClientOnly            bool   // Solo cliente, no relay ni exit
	AutomapHostsOnResolve bool   // Resolver .onion y .exit a través de Tor
}

// Rutas típicas del binario tor (por si PATH no incluye /usr/bin en el proceso)
var torBinaryPaths = []string{"/usr/bin/tor", "/usr/sbin/tor"}

func isTorInstalled() bool {
	// Primero: command -v tor (PATH del shell)
	checkCmd := exec.Command("sh", "-c", "command -v tor 2>/dev/null")
	if checkCmd.Run() == nil {
		return true
	}
	// Segundo: comprobar rutas conocidas (útil cuando el proceso no tiene /usr/bin en PATH, p. ej. systemd)
	for _, p := range torBinaryPaths {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// Funciones para Tor
func getTorStatus() map[string]interface{} {
	result := make(map[string]interface{})

	// Verificar si está instalado (PATH + rutas conocidas)
	installed := isTorInstalled()
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

	// Estado de iptables (red hostapd torificada)
	iptStatus := getTorIptablesStatus()
	if active, ok := iptStatus["active"].(bool); ok {
		result["iptables_active"] = active
	}
	if iface, ok := iptStatus["interface"].(string); ok {
		result["iptables_interface"] = iface
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
	if isTorInstalled() {
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

func configureTor(opts TorConfigOptions) map[string]interface{} {
	result := make(map[string]interface{})

	if opts.User == "" {
		opts.User = "unknown"
	}

	LogTf("logs.tor_configuring", opts.User)

	// Verificar si está instalado
	if !isTorInstalled() {
		result["success"] = false
		result["error"] = "Tor no está instalado. Instálalo primero."
		return result
	}

	configDir := "/etc/tor"
	configPath := filepath.Join(configDir, "torrc")

	// Crear directorio si no existe
	executeCommand(fmt.Sprintf("sudo mkdir -p %s", configDir))

	// Valores por defecto
	if opts.SocksPort == 0 {
		opts.SocksPort = 9050
	}
	if opts.ControlPort == 0 {
		opts.ControlPort = 9051
	}
	if opts.TransPort == 0 {
		opts.TransPort = 9040
	}
	if opts.DNSPort == 0 {
		opts.DNSPort = 53
	}

	// Bloques opcionales para torrc
	socksBlock := ""
	if opts.EnableSocks {
		socksBlock = fmt.Sprintf("SocksPort %d\nSocksPolicy accept 127.0.0.1\nSocksPolicy reject *\n", opts.SocksPort)
	} else {
		socksBlock = fmt.Sprintf("# SocksPort %d (deshabilitado)\n", opts.SocksPort)
	}

	controlBlock := ""
	if opts.EnableControlPort {
		controlBlock = fmt.Sprintf("ControlPort %d\nCookieAuthentication 1\n", opts.ControlPort)
	} else {
		controlBlock = fmt.Sprintf("# ControlPort %d (deshabilitado)\n", opts.ControlPort)
	}

	// Estilo Onion Pi: proxy transparente, DNS y resolución .onion/.exit
	transBlock := ""
	if opts.EnableTransPort {
		transBlock = fmt.Sprintf("TransPort %d\n", opts.TransPort)
	}
	dnsBlock := ""
	if opts.EnableDNSPort {
		dnsBlock = fmt.Sprintf("DNSPort %d\n", opts.DNSPort)
	}
	clientOnlyLine := ""
	if opts.ClientOnly {
		clientOnlyLine = "ClientOnly 1\n"
	}
	automapLines := ""
	if opts.AutomapHostsOnResolve {
		automapLines = "AutomapHostsSuffixes .onion,.exit\nAutomapHostsOnResolve 1\n"
	}

	hiddenBlock := ""
	if opts.EnableHiddenService {
		hiddenBlock = `# Servicio oculto (ejemplo)
# HiddenServiceDir /var/lib/tor/hidden_service/
# HiddenServicePort 80 127.0.0.1:80
`
	}

	configContent := fmt.Sprintf(`# Configuración Tor para HostBerry (compatible con estilo Onion Pi)
# Generado automáticamente - https://github.com/breadtk/onion_pi

DataDirectory /var/lib/tor
Log notice file /var/log/tor/notices.log
RunAsDaemon 1

# SOCKS Proxy (puerto explícito para aplicaciones)
%s
# Control Port
%s
# Proxy transparente (para redirección por iptables, estilo Onion Pi)
%s
# DNS a través de Tor (DNSPort)
%s
# Solo cliente, no relay ni exit
%s
# Resolver .onion y .exit a través de Tor
%s
DisableDebuggerAttachment 1
NumEntryGuards 3
CircuitBuildTimeout 10
KeepalivePeriod 60
NewCircuitPeriod 30
SafeLogging 1
AvoidDiskWrites 0

%s
`, socksBlock, controlBlock, transBlock, dnsBlock, clientOnlyLine, automapLines, hiddenBlock)

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
	if !isTorInstalled() {
		result["success"] = false
		result["error"] = "Tor no está instalado. Instálalo primero."
		return result
	}

	// Verificar configuración
	configPath := "/etc/tor/torrc"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Configurar con valores por defecto (estilo Onion Pi: SOCKS, TransPort, DNSPort, ClientOnly, Automap)
		configResult := configureTor(TorConfigOptions{
			User:                  user,
			EnableSocks:           true,
			SocksPort:             9050,
			EnableControlPort:     true,
			ControlPort:           9051,
			EnableTransPort:       true,
			TransPort:             9040,
			EnableDNSPort:         true,
			DNSPort:               53,
			ClientOnly:            true,
			AutomapHostsOnResolve: true,
		})
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

const torIptablesComment = "HostBerry-Tor"

// getHostapdInterface devuelve la interfaz usada por hostapd (ej. ap0, wlan0) leyendo /etc/hostapd/hostapd.conf.
func getHostapdInterface() string {
	configPath := "/etc/hostapd/hostapd.conf"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "ap0"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				iface := strings.TrimSpace(parts[1])
				if iface != "" {
					return iface
				}
			}
			break
		}
	}
	return "ap0"
}

// getTorIptablesStatus indica si las reglas iptables de Tor para la red hostapd están activas.
func getTorIptablesStatus() map[string]interface{} {
	result := make(map[string]interface{})
	result["active"] = false
	result["interface"] = getHostapdInterface()
	result["success"] = true

	cmd := exec.Command("sh", "-c", "iptables -t nat -L PREROUTING -n -v 2>/dev/null | grep -c '"+torIptablesComment+"' || true")
	out, err := cmd.Output()
	if err != nil {
		return result
	}
	count := 0
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	result["active"] = count > 0
	return result
}

// enableTorIptables redirige todo el tráfico de la interfaz hostapd hacia Tor (TransPort y DNSPort). Estilo Onion Pi.
func enableTorIptables(user string) map[string]interface{} {
	result := make(map[string]interface{})
	if user == "" {
		user = "unknown"
	}

	// Comprobar que Tor está instalado y activo
	status := getTorStatus()
	if inst, _ := status["installed"].(bool); !inst {
		result["success"] = false
		result["error"] = "Tor no está instalado. Instálalo y habilítalo primero."
		return result
	}
	if active, _ := status["active"].(bool); !active {
		result["success"] = false
		result["error"] = "Tor no está activo. Habilita Tor primero."
		return result
	}

	iface := getHostapdInterface()
	transPort := 9040
	dnsPort := 53
	// Opcional: leer puertos del torrc
	if data, err := os.ReadFile("/etc/tor/torrc"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "TransPort ") {
				fmt.Sscanf(line, "TransPort %d", &transPort)
			}
			if strings.HasPrefix(line, "DNSPort ") {
				fmt.Sscanf(line, "DNSPort %d", &dnsPort)
			}
		}
	}

	// Quitar reglas previas para evitar duplicados
	disableTorIptables("")

	// Añadir reglas NAT: DNS y TCP hacia Tor
	// DNS (UDP 53 -> DNSPort)
	addDNS := fmt.Sprintf("sudo iptables -t nat -A PREROUTING -i %s -p udp --dport 53 -j REDIRECT --to-ports %d -m comment --comment %s 2>&1", iface, dnsPort, torIptablesComment)
	if out, err := executeCommand(addDNS); err != nil {
		result["success"] = false
		result["error"] = "Error añadiendo regla DNS: " + strings.TrimSpace(out)
		LogTf("logs.tor_iptables_error", out)
		return result
	}
	// TCP (SYN -> TransPort)
	addTCP := fmt.Sprintf("sudo iptables -t nat -A PREROUTING -i %s -p tcp --syn -j REDIRECT --to-ports %d -m comment --comment %s 2>&1", iface, transPort, torIptablesComment)
	if out, err := executeCommand(addTCP); err != nil {
		executeCommand(fmt.Sprintf("sudo iptables -t nat -D PREROUTING -i %s -p udp --dport 53 -j REDIRECT --to-ports %d -m comment --comment %s 2>/dev/null", iface, dnsPort, torIptablesComment))
		result["success"] = false
		result["error"] = "Error añadiendo regla TCP: " + strings.TrimSpace(out)
		return result
	}

	// Persistir reglas si netfilter-persistent está disponible
	executeCommand("sudo netfilter-persistent save 2>/dev/null || true")
	executeCommand("sudo iptables-save | sudo tee /etc/iptables/rules.v4 >/dev/null 2>&1 || true")

	result["success"] = true
	result["message"] = fmt.Sprintf("Tráfico de la red Hostberry (%s) redirigido a Tor. Los clientes WiFi usarán Tor.", iface)
	result["interface"] = iface
	LogTf("logs.tor_iptables_enabled", user)
	return result
}

// disableTorIptables elimina las reglas iptables que redirigen la interfaz hostapd a Tor.
func disableTorIptables(user string) map[string]interface{} {
	result := make(map[string]interface{})
	if user == "" {
		user = "unknown"
	}

	iface := getHostapdInterface()
	transPort := 9040
	dnsPort := 53
	if data, err := os.ReadFile("/etc/tor/torrc"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "TransPort ") {
				fmt.Sscanf(line, "TransPort %d", &transPort)
			}
			if strings.HasPrefix(line, "DNSPort ") {
				fmt.Sscanf(line, "DNSPort %d", &dnsPort)
			}
		}
	}

	// Eliminar reglas (pueden estar duplicadas si se activó dos veces)
	for i := 0; i < 5; i++ {
		delDNS := fmt.Sprintf("sudo iptables -t nat -D PREROUTING -i %s -p udp --dport 53 -j REDIRECT --to-ports %d -m comment --comment %s 2>&1", iface, dnsPort, torIptablesComment)
		if _, err := executeCommand(delDNS); err != nil {
			break
		}
	}
	for i := 0; i < 5; i++ {
		delTCP := fmt.Sprintf("sudo iptables -t nat -D PREROUTING -i %s -p tcp --syn -j REDIRECT --to-ports %d -m comment --comment %s 2>&1", iface, transPort, torIptablesComment)
		if _, err := executeCommand(delTCP); err != nil {
			break
		}
	}

	executeCommand("sudo netfilter-persistent save 2>/dev/null || true")
	executeCommand("sudo iptables-save | sudo tee /etc/iptables/rules.v4 >/dev/null 2>&1 || true")

	result["success"] = true
	result["message"] = "Redirección de la red Hostberry a Tor desactivada."
	LogTf("logs.tor_iptables_disabled", user)
	return result
}
