package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"gopkg.in/yaml.v3"
)

//go:embed website/templates
var templatesFS embed.FS

//go:embed website/static
var staticFS embed.FS

// Configuración de la aplicación
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Security SecurityConfig `yaml:"security"`
}

type ServerConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Debug        bool   `yaml:"debug"`
	ReadTimeout  int    `yaml:"read_timeout"`
	WriteTimeout int    `yaml:"write_timeout"`
}

type DatabaseConfig struct {
	Type     string `yaml:"type"` // sqlite, postgres, mysql
	Path     string `yaml:"path"` // Para SQLite
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

type SecurityConfig struct {
	JWTSecret    string `yaml:"jwt_secret"`
	TokenExpiry  int    `yaml:"token_expiry"` // minutos
	BcryptCost   int    `yaml:"bcrypt_cost"`
	RateLimitRPS int    `yaml:"rate_limit_rps"`
}


var appConfig Config
func main() {
	// Cargar configuración
	if err := loadConfig(); err != nil {
		log.Fatalf("Error cargando configuración: %v", err)
	}


	// Inicializar i18n
	if err := InitI18n("locales"); err != nil {
		log.Printf("⚠️  Advertencia: Error inicializando i18n: %v", err)
	}

	// Inicializar base de datos
	if err := initDatabase(); err != nil {
		log.Fatalf("❌ Error inicializando base de datos: %v", err)
	}

	// Crear usuario admin por defecto si no existe
	log.Println("🔐 Verificando usuario admin por defecto...")
	createDefaultAdmin()

	// Crear aplicación Fiber
	app := createApp()

	// Configurar rutas
	setupRoutes(app)

	// Iniciar servidor
	addr := fmt.Sprintf("%s:%d", appConfig.Server.Host, appConfig.Server.Port)
	log.Printf("🚀 HostBerry iniciando en %s", addr)
	log.Printf("📋 Configuración: Debug=%v, Timeout=%ds/%ds",
		appConfig.Server.Debug,
		appConfig.Server.ReadTimeout,
		appConfig.Server.WriteTimeout)

	// Manejo graceful de shutdown
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint
		log.Println("🛑 Deteniendo servidor...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Printf("Error en shutdown: %v", err)
		}
		os.Exit(0)
	}()

	log.Println("✅ Servidor listo, escuchando en", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("❌ Error iniciando servidor: %v", err)
	}
}

func loadConfig() error {
	// Intentar cargar desde config.yaml, si no existe usar defaults
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		// Configuración por defecto
		appConfig = Config{
			Server: ServerConfig{
				Host:         DefaultServerHost,
				Port:         DefaultServerPort,
				Debug:        false,
				ReadTimeout:  30,
				WriteTimeout: 30,
			},
			Database: DatabaseConfig{
				Type: "sqlite",
				Path: "data/hostberry.db",
			},
			Security: SecurityConfig{
				JWTSecret:    generateRandomSecret(),
				TokenExpiry:  60, // 1 hora
				BcryptCost:   10,
				RateLimitRPS: 10,
			},
		}
		return nil
	}

	return yaml.Unmarshal(data, &appConfig)
}

func createApp() *fiber.App {
	// Configurar templates
	engine := createTemplateEngine()
	if engine == nil {
		log.Fatal("❌ Error crítico: No se pudo crear el engine de templates")
	}

	log.Printf("✅ Engine de templates creado, asignando a Fiber app...")

	app := fiber.New(fiber.Config{
		Views:        engine,
		ReadTimeout:  time.Duration(appConfig.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(appConfig.Server.WriteTimeout) * time.Second,
		ErrorHandler: errorHandler,
	})

	// Verificar que el engine se asignó correctamente
	if app.Config().Views == nil {
		log.Fatal("❌ Error crítico: Views no se asignó correctamente a la app")
	}
	log.Println("✅ Engine de templates asignado correctamente a Fiber app")

	// IMPORTANTE: Registrar archivos estáticos ANTES de aplicar middlewares globales
	// para evitar que cualquier middleware intercepte /static/*
	setupStaticFiles(app)

	// Middlewares globales
	// Configurar logger con formato más legible
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n",
		TimeFormat: "15:04:05",
		TimeZone:   "Local",
		Output:     os.Stdout,
		// Filtrar archivos estáticos para reducir ruido
		Next: func(c *fiber.Ctx) bool {
			path := c.Path()
			// Omitir logs de archivos estáticos comunes
			return strings.HasPrefix(path, "/static/") &&
				(strings.HasSuffix(path, ".css") ||
					strings.HasSuffix(path, ".js") ||
					strings.HasSuffix(path, ".png") ||
					strings.HasSuffix(path, ".jpg") ||
					strings.HasSuffix(path, ".jpeg") ||
					strings.HasSuffix(path, ".gif") ||
					strings.HasSuffix(path, ".ico") ||
					strings.HasSuffix(path, ".svg") ||
					strings.HasSuffix(path, ".woff") ||
					strings.HasSuffix(path, ".woff2") ||
					strings.HasSuffix(path, ".ttf") ||
					strings.HasSuffix(path, ".eot"))
		},
	}))
	app.Use(compress.New())
	// CORS más restrictivo: solo permitir orígenes específicos en producción
	corsOrigins := "*"
	if !appConfig.Server.Debug {
		// En producción, solo permitir localhost y la IP del servidor
		corsOrigins = "http://localhost:" + fmt.Sprintf("%d", appConfig.Server.Port) + ",http://127.0.0.1:" + fmt.Sprintf("%d", appConfig.Server.Port)
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowCredentials: true,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Content-Type,Authorization",
		MaxAge:           3600,
	}))

	// Middleware de seguridad se aplica a rutas específicas con requireAuth

	// Middleware de logging
	app.Use(loggingMiddleware)

	// Middleware de idioma (debe ir antes de las rutas)
	app.Use(LanguageMiddleware)

	// Middleware de Request ID para tracing
	app.Use(requestIDMiddleware)

	// Rate limiting (solo para APIs)
	app.Use("/api/", rateLimitMiddleware)

	return app
}

// setupStaticFiles registra los archivos estáticos ANTES de cualquier middleware
// para asegurar que /static/* siempre se sirva correctamente sin interceptación
func setupStaticFiles(app *fiber.App) {
	// Archivos estáticos: preferir filesystem (para poder actualizar JS/CSS sin recompilar),
	// fallback a embebidos si no existe ./website/static.
	if _, err := os.Stat("./website/static"); err == nil {
		app.Static("/static", "./website/static", fiber.Static{
			Compress:  true,
			ByteRange: true,
		})
		log.Println("✅ Archivos estáticos cargados desde sistema de archivos")
	} else {
		// Fallback: estáticos embebidos
		staticSubFS, err := fs.Sub(staticFS, "website/static")
		if err != nil {
			log.Printf("⚠️  Error preparando archivos estáticos embebidos: %v", err)
			log.Printf("⚠️  No se encontraron archivos estáticos ni en filesystem ni embebidos")
		} else {
			// Usar handler personalizado para archivos embebidos
			app.Get("/static/*", func(c *fiber.Ctx) error {
				path := c.Params("*")
				file, err := staticSubFS.Open(path)
				if err != nil {
					return c.Status(404).SendString("Not found")
				}
				defer file.Close()

				stat, err := file.Stat()
				if err != nil {
					return c.Status(500).SendString("Error reading file")
				}

				c.Type(filepath.Ext(path))
				return c.SendStream(file, int(stat.Size()))
			})
			log.Println("✅ Archivos estáticos cargados desde archivos embebidos")
		}
	}
}

func setupRoutes(app *fiber.App) {
	// Health check endpoints (sin autenticación)
	app.Get("/health", healthCheckHandler)
	app.Get("/health/ready", readinessCheckHandler)
	app.Get("/health/live", livenessCheckHandler)

	// Rutas web
	web := app.Group("/")
	{
		web.Get("/login", loginHandler)
		web.Get("/first-login", firstLoginPageHandler)
		web.Get("/", indexHandler)

		// Páginas protegidas (requieren token por cookie o query ?token=)
		protected := web.Group("/", requireAuth)
		protected.Get("/dashboard", dashboardHandler)
		protected.Get("/settings", settingsHandler)
		protected.Get("/network", networkPageHandler)
		protected.Get("/wifi", wifiPageHandler)
		protected.Get("/wifi-scan", wifiScanPageHandler)
		protected.Get("/vpn", vpnPageHandler)
		protected.Get("/wireguard", wireguardPageHandler)
		protected.Get("/adblock", adblockPageHandler)
		protected.Get("/hostapd", hostapdPageHandler)
		protected.Get("/profile", profilePageHandler)
		protected.Get("/system", systemPageHandler)
		protected.Get("/monitoring", monitoringPageHandler)
		protected.Get("/update", updatePageHandler)
	}

	// API v1
	api := app.Group("/api/v1")
	{
		// Autenticación
		auth := api.Group("/auth")
		{
			auth.Post("/login", loginAPIHandler)
			auth.Post("/logout", requireAuth, logoutAPIHandler)
			auth.Get("/me", requireAuth, meHandler)
			auth.Post("/change-password", requireAuth, changePasswordAPIHandler)
			auth.Post("/first-login/change", firstLoginChangeAPIHandler)
			auth.Post("/profile", requireAuth, updateProfileAPIHandler)
			auth.Post("/preferences", requireAuth, updatePreferencesAPIHandler)
		}

		// Sistema
		system := api.Group("/system", requireAuth)
		{
			system.Get("/stats", systemStatsHandler)
			system.Get("/info", systemInfoHandler)
			system.Get("/logs", systemLogsHandler)
			system.Get("/activity", systemActivityHandler)
			system.Get("/network", systemNetworkHandler)
			system.Get("/updates", systemUpdatesHandler)
			system.Get("/services", systemServicesHandler)
			system.Post("/backup", systemBackupHandler)
			system.Post("/config", systemConfigHandler)
			system.Post("/restart", systemRestartHandler)
			system.Post("/shutdown", systemShutdownHandler)
		}

		// Red
		network := api.Group("/network", requireAuth)
		{
			network.Get("/status", networkStatusHandler)
			network.Get("/interfaces", networkInterfacesHandler)
			network.Get("/routing", networkRoutingHandler)
			network.Post("/firewall/toggle", networkFirewallToggleHandler)
			network.Get("/config", networkConfigHandler)
			network.Post("/config", networkConfigHandler)
		}

		// WiFi
		wifi := api.Group("/wifi", requireAuth)
		{
			wifi.Get("/status", wifiStatusHandler)
			wifi.Get("/scan", wifiScanHandler)
			wifi.Post("/scan", wifiScanHandler)
			wifi.Get("/interfaces", wifiInterfacesHandler)
			wifi.Post("/connect", wifiConnectHandler)
			wifi.Get("/networks", wifiNetworksHandler)
			wifi.Get("/clients", wifiClientsHandler)
			wifi.Post("/toggle", wifiToggleHandler)
			wifi.Post("/unblock", wifiUnblockHandler)
			wifi.Post("/software-switch", wifiSoftwareSwitchHandler)
			wifi.Post("/config", wifiConfigHandler)
		}

		// VPN
		vpn := api.Group("/vpn", requireAuth)
		{
			vpn.Get("/status", vpnStatusHandler)
			vpn.Post("/connect", vpnConnectHandler)
			vpn.Get("/connections", vpnConnectionsHandler)
			vpn.Get("/servers", vpnServersHandler)
			vpn.Get("/clients", vpnClientsHandler)
			vpn.Post("/toggle", vpnToggleHandler)
			vpn.Post("/config", vpnConfigHandler)
			vpn.Post("/connections/:name/toggle", vpnConnectionToggleHandler)
			vpn.Post("/certificates/generate", vpnCertificatesGenerateHandler)
		}

		// HostAPD
		hostapd := api.Group("/hostapd", requireAuth)
		{
		hostapd.Get("/access-points", hostapdAccessPointsHandler)
		hostapd.Get("/clients", hostapdClientsHandler)
		hostapd.Get("/config", hostapdGetConfigHandler)
		hostapd.Get("/diagnostics", hostapdDiagnosticsHandler)
		hostapd.Post("/toggle", hostapdToggleHandler)
		hostapd.Post("/restart", hostapdRestartHandler)
		hostapd.Post("/config", hostapdConfigHandler)
		}

		// Help
		help := api.Group("/help", requireAuth)
		{
			help.Post("/contact", helpContactHandler)
		}

		// Translations (para carga dinámica)
		api.Get("/translations/:lang", translationsHandler)

		// WireGuard
		wireguard := api.Group("/wireguard", requireAuth)
		{
			wireguard.Get("/status", wireguardStatusHandler)
			wireguard.Get("/interfaces", wireguardInterfacesHandler)
			wireguard.Get("/peers", wireguardPeersHandler)
			wireguard.Get("/config", wireguardGetConfigHandler)
			wireguard.Post("/config", wireguardConfigHandler)
			wireguard.Post("/toggle", wireguardToggleHandler)
			wireguard.Post("/restart", wireguardRestartHandler)
		}

		// AdBlock
		adblock := api.Group("/adblock", requireAuth)
		{
			adblock.Get("/status", adblockStatusHandler)
			adblock.Post("/enable", adblockEnableHandler)
			adblock.Post("/disable", adblockDisableHandler)
		}
	}

	// Compat legacy: /api/wifi/* usado por wifi_scan.js
	// IMPORTANTE: NO usar app.Group("/api", ...) porque intercepta /api/v1/*
	wifiLegacy := app.Group("/api/wifi", requireAuth)
	wifiLegacy.Get("/status", wifiLegacyStatusHandler)
	wifiLegacy.Get("/stored_networks", wifiLegacyStoredNetworksHandler)
	wifiLegacy.Get("/autoconnect", wifiLegacyAutoconnectHandler)
	wifiLegacy.Get("/scan", wifiLegacyScanHandler)
	wifiLegacy.Post("/disconnect", wifiLegacyDisconnectHandler)
}

// Handlers básicos
func indexHandler(c *fiber.Ctx) error {
	// Intentar obtener token de cookie o query
	token := c.Cookies("access_token")
	if token == "" {
		token = c.Query("token")
	}

	// Si hay token, validarlo
	if token != "" {
		claims, err := ValidateToken(token)
		if err == nil {
			// Token válido, verificar usuario
			var user User
			if err := db.First(&user, claims.UserID).Error; err == nil && user.IsActive {
				// Usuario válido y activo, redirigir a dashboard
				return c.Redirect("/dashboard")
			}
		}
	}

	// No hay token válido o usuario no válido, redirigir a login
	return c.Redirect("/login")
}

func dashboardHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "dashboard", fiber.Map{
		"Title": T(c, "dashboard.title", "Dashboard"),
	})
}

func loginHandler(c *fiber.Ctx) error {
	return renderTemplate(c, "login", fiber.Map{
		"Title": T(c, "auth.login", "Login"),
	})
}

func settingsHandler(c *fiber.Ctx) error {
	configs, _ := GetAllConfigs()

	// Asegurar valores por defecto si no existen
	if _, exists := configs["max_login_attempts"]; !exists || configs["max_login_attempts"] == "" {
		configs["max_login_attempts"] = "3"
	}
	if _, exists := configs["session_timeout"]; !exists || configs["session_timeout"] == "" {
		configs["session_timeout"] = "60"
	}
	if _, exists := configs["cache_enabled"]; !exists || configs["cache_enabled"] == "" {
		configs["cache_enabled"] = "true"
	}
	if _, exists := configs["cache_size"]; !exists || configs["cache_size"] == "" {
		configs["cache_size"] = "75"
	}

	configsJSON, _ := json.Marshal(configs)

	return renderTemplate(c, "settings", fiber.Map{
		"Title":         T(c, "navigation.settings", "Settings"),
		"settings":      configs,
		"settings_json": string(configsJSON),
	})
}

// Handlers de API
func systemStatsHandler(c *fiber.Ctx) error {
	stats := getSystemStats()
	return c.JSON(stats)
}

func systemRestartHandler(c *fiber.Ctx) error {
	user := c.Locals("user").(*User)
	userID := user.ID
	
	result := systemRestart(user.Username)
	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("Sistema reiniciado por usuario %s", user.Username), "system", &userID)
		return c.JSON(result)
	}
	
	if err, ok := result["error"].(string); ok {
		InsertLog("ERROR", fmt.Sprintf("Error reiniciando sistema: %s (usuario: %s)", err, user.Username), "system", &userID)
		return c.Status(500).JSON(fiber.Map{"error": err})
	}
	
	return c.JSON(result)
}

// detectWiFiInterface detecta automáticamente la interfaz WiFi
func detectWiFiInterface() string {
	// Intentar con ip primero (sin sudo, solo lectura)
	cmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl' | head -1")
	out, err := cmd.Output()
	if err == nil {
		iface := strings.TrimSpace(string(out))
		if iface != "" {
			return iface
		}
	}

	// Último fallback: usar interfaz por defecto
	return DefaultWiFiInterface
}

// wifiInterfacesHandler devuelve las interfaces WiFi disponibles
// Usa solo ip/iw, sin nmcli
func wifiInterfacesHandler(c *fiber.Ctx) error {
	var interfaces []fiber.Map

	// Buscar interfaces wlan* usando ip
	cmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl'")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, ifaceName := range lines {
			ifaceName = strings.TrimSpace(ifaceName)
			if ifaceName != "" {
				// Verificar estado usando /sys/class/net
				stateCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/operstate 2>/dev/null", ifaceName))
				stateOut, _ := stateCmd.Output()
				state := strings.TrimSpace(string(stateOut))
				if state == "" {
					state = "unknown"
				}

				interfaces = append(interfaces, fiber.Map{
					"name":  ifaceName,
					"type":  "wifi",
					"state": state,
				})
			}
		}
	}

	// Si no hay interfaces, agregar interfaz por defecto como opción
	if len(interfaces) == 0 {
		interfaces = append(interfaces, fiber.Map{
			"name":  DefaultWiFiInterface,
			"type":  "wifi",
			"state": "unknown",
		})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"interfaces": interfaces,
	})
}

func wifiScanHandler(c *fiber.Ctx) error {
	// Verificar que el usuario esté autenticado
	userInterface := c.Locals("user")
	if userInterface == nil {
		log.Printf("ERROR: Usuario no encontrado en wifiScanHandler")
		return c.Status(401).JSON(fiber.Map{
			"error": "No autenticado. Por favor, inicia sesión nuevamente.",
		})
	}
	
	user, ok := userInterface.(*User)
	if !ok || user == nil {
		log.Printf("ERROR: Usuario inválido en wifiScanHandler")
		return c.Status(401).JSON(fiber.Map{
			"error": "Usuario no encontrado. Por favor, inicia sesión nuevamente.",
		})
	}
	
	// Obtener interfaz del query o body
	interfaceName := c.Query("interface", "")
	if interfaceName == "" {
		var req struct {
			Interface string `json:"interface"`
		}
		if err := c.BodyParser(&req); err == nil {
			interfaceName = req.Interface
		}
	}

	// Si no se especifica interfaz, detectar automáticamente
	if interfaceName == "" {
		interfaceName = detectWiFiInterface()
	}
	if interfaceName == "" {
		interfaceName = DefaultWiFiInterface
	}

	result := scanWiFiNetworks(interfaceName)
	if networks, ok := result["networks"]; ok {
		return c.JSON(networks)
	}
	return c.JSON([]fiber.Map{})
}

	var networks []fiber.Map

	// Si no se especifica interfaz, detectar automáticamente
	if interfaceName == "" {
		interfaceName = detectWiFiInterface()
	}

	// Verificar que WiFi esté habilitado (usar rfkill en lugar de solo nmcli)
	wifiEnabled := false
	
	// Método 1: Verificar con rfkill
	rfkillCheck := execCommand("rfkill list wifi 2>/dev/null")
	rfkillOut, _ := rfkillCheck.Output()
	rfkillStr := strings.ToLower(string(filterSudoErrors(rfkillOut)))
	if !strings.Contains(rfkillStr, "hard blocked: yes") && !strings.Contains(rfkillStr, "soft blocked: yes") {
		wifiEnabled = true
	}
	
	// Método 2: Si rfkill no funcionó, verificar con nmcli (solo si NetworkManager está corriendo)
	if !wifiEnabled {
		if out, _ := exec.Command("pgrep", "NetworkManager").Output(); len(out) > 0 {
			wifiCheck := execCommand("nmcli -t -f WIFI g 2>/dev/null")
			wifiOut, _ := wifiCheck.Output()
			wifiState := strings.ToLower(strings.TrimSpace(filterSudoErrors(wifiOut)))
			if strings.Contains(wifiState, "enabled") || strings.Contains(wifiState, "on") {
				wifiEnabled = true
			}
		}
	}
	
	// Método 3: Verificar que la interfaz esté activa
	if !wifiEnabled {
		ipCheck := execCommand(fmt.Sprintf("ip link show %s 2>/dev/null | grep -i 'state UP'", interfaceName))
		ipOut, _ := ipCheck.Output()
		if len(ipOut) > 0 {
			wifiEnabled = true
		}
	}
	
	if !wifiEnabled {
		log.Printf("⚠️  WiFi no está habilitado")
		return c.JSON(fiber.Map{
			"success":  false,
			"error":    "WiFi no está habilitado. Por favor, habilita WiFi primero.",
			"networks": []fiber.Map{},
		})
	}
	
	// Asegurar que la interfaz esté en modo managed (no AP) para poder escanear
	iwInfoCmd := execCommand(fmt.Sprintf("iw dev %s info 2>/dev/null", interfaceName))
	iwInfoOut, _ := iwInfoCmd.Output()
	iwInfoStr := string(iwInfoOut)
	if strings.Contains(iwInfoStr, "type AP") {
		log.Printf("⚠️  Interfaz está en modo AP, cambiando a modo managed para escanear")
		executeCommand(fmt.Sprintf("sudo iw dev %s set type managed 2>/dev/null", interfaceName))
		time.Sleep(1 * time.Second)
	}
	
	// Asegurar que la interfaz esté activa
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null", interfaceName))
	time.Sleep(500 * time.Millisecond)

	// Método 1: Intentar con nmcli (solo si NetworkManager está corriendo)
	// Verificar si NetworkManager está corriendo
	nmCheck := exec.Command("pgrep", "NetworkManager")
	if nmOut, _ := nmCheck.Output(); len(nmOut) > 0 {
		cmd := execCommand("nmcli -t -f SSID,SIGNAL,SECURITY,CHAN dev wifi list 2>&1")
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))

		if err == nil && len(output) > 0 && !strings.Contains(output, "Error") && !strings.Contains(output, "permission") {
			log.Printf("📡 Escaneando con nmcli...")
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || line == "--" || strings.HasPrefix(line, "Error") {
					continue
				}
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					ssid := parts[0]
					signalStr := parts[1]
					security := "Open"
					channel := ""
					if len(parts) >= 3 {
						security = parts[2]
					}
					if len(parts) >= 4 {
						channel = parts[3]
					}
					if ssid != "" && ssid != "--" {
						signal := 0
						if s, err := strconv.Atoi(signalStr); err == nil {
							signal = s
						}
						networks = append(networks, fiber.Map{
							"ssid":     ssid,
							"signal":   signal,
							"security": security,
							"channel":  channel,
						})
					}
				}
			}
			if len(networks) > 0 {
				log.Printf("✅ Encontradas %d redes con nmcli", len(networks))
				return c.JSON(fiber.Map{
					"success":  true,
					"networks": networks,
				})
			}
		}
	}

	// Método 2: Usar iw directamente (más confiable cuando NetworkManager no está disponible)
	log.Printf("📡 Escaneando con iw en interfaz %s...", interfaceName)
	
	// Asegurar que la interfaz esté activa antes de escanear
	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null", interfaceName))
	time.Sleep(1 * time.Second)
	
	iwCmd := execCommand(fmt.Sprintf("iw dev %s scan 2>&1", interfaceName))
	iwOut, iwErr := iwCmd.CombinedOutput()
		if iwErr == nil && len(iwOut) > 0 {
		lines := strings.Split(string(iwOut), "\n")
		currentNetwork := make(map[string]interface{})
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Detectar inicio de nuevo BSS (nueva red)
			if strings.HasPrefix(line, "BSS ") {
				// Guardar red anterior si existe
				if ssid, ok := currentNetwork["ssid"].(string); ok && ssid != "" {
					networks = append(networks, fiber.Map{
						"ssid":     currentNetwork["ssid"],
						"signal":   currentNetwork["signal"],
						"security": currentNetwork["security"],
						"channel":  currentNetwork["channel"],
					})
				}
				// Iniciar nueva red
				currentNetwork = make(map[string]interface{})
				currentNetwork["security"] = "Open"  // Por defecto Open, se actualizará si se detecta seguridad
				currentNetwork["signal"] = 0
				currentNetwork["channel"] = ""
			} else if strings.HasPrefix(line, "SSID:") {
				// Si encontramos SSID sin BSS previo, guardar red anterior
				if ssid, ok := currentNetwork["ssid"].(string); ok && ssid != "" {
					networks = append(networks, fiber.Map{
						"ssid":     currentNetwork["ssid"],
						"signal":   currentNetwork["signal"],
						"security": currentNetwork["security"],
						"channel":  currentNetwork["channel"],
					})
				}
				// Iniciar nueva red
				currentNetwork = make(map[string]interface{})
				ssid := strings.TrimSpace(strings.TrimPrefix(line, "SSID:"))
				if ssid != "" {
					currentNetwork["ssid"] = ssid
					currentNetwork["security"] = "Open"  // Por defecto Open, se actualizará si se detecta seguridad
					currentNetwork["signal"] = 0
					currentNetwork["channel"] = ""
				}
			} else if strings.Contains(line, "signal:") {
				// Formato de iw scan: "signal: -45.00 dBm" o "signal: -45 dBm"
				parts := strings.Fields(line)
				for i, part := range parts {
					if part == "signal:" && i+1 < len(parts) {
						signalStr := strings.TrimSpace(parts[i+1])
						// Remover "dBm" si está presente
						signalStr = strings.TrimSuffix(signalStr, "dBm")
						signalStr = strings.TrimSpace(signalStr)
						// Parsear como float primero para manejar decimales
						if signalFloat, err := strconv.ParseFloat(signalStr, 64); err == nil {
							// Convertir a entero (redondear)
							signalInt := int(signalFloat)
							currentNetwork["signal"] = signalInt
							log.Printf("Parsed signal: %d dBm from line: %s", signalInt, line)
						} else {
							log.Printf("Warning: Could not parse signal from line: %s, error: %v", line, err)
						}
						break
					}
				}
			} else if strings.Contains(line, "freq:") {
				// Formato: "freq: 2412" o "freq: 2412 [MHz]"
				parts := strings.Fields(line)
				for i, part := range parts {
					if part == "freq:" && i+1 < len(parts) {
						freqStr := strings.TrimSpace(parts[i+1])
						// Remover "[MHz]" si está presente
						freqStr = strings.TrimSuffix(freqStr, "[MHz]")
						freqStr = strings.TrimSpace(freqStr)
						if f, err := strconv.Atoi(freqStr); err == nil {
							// Convertir frecuencia a canal
							if f >= 2412 && f <= 2484 {
								channel := (f-2412)/5 + 1
								currentNetwork["channel"] = strconv.Itoa(channel)
								log.Printf("Parsed channel: %d from freq: %d", channel, f)
							} else if f >= 5000 && f <= 5825 {
								channel := (f - 5000) / 5
								currentNetwork["channel"] = strconv.Itoa(channel)
								log.Printf("Parsed channel: %d from freq: %d", channel, f)
							} else {
								log.Printf("Warning: Frecuencia fuera de rango: %d", f)
							}
						} else {
							log.Printf("Warning: Could not parse frequency from line: %s, error: %v", line, err)
						}
						break
					}
				}
			} else if strings.Contains(line, "RSN:") {
				// RSN (Robust Security Network) indica WPA2 o WPA3
				if strings.Contains(line, "WPA3") || strings.Contains(line, "SAE") || strings.Contains(line, "suite-B") {
					currentNetwork["security"] = "WPA3"
				} else {
					currentNetwork["security"] = "WPA2"
				}
				log.Printf("Detected security: %s from RSN line: %s", currentNetwork["security"], line)
			} else if strings.Contains(line, "WPA:") {
				// WPA indica WPA2 (WPA1 es raro)
				currentNetwork["security"] = "WPA2"
				log.Printf("Detected security: WPA2 from WPA line: %s", line)
			} else if strings.Contains(line, "capability:") {
				// Detectar si tiene Privacy (WEP o protegida)
				if strings.Contains(line, "Privacy") {
					// Solo establecer WEP si no se ha detectado otra seguridad
					if sec, ok := currentNetwork["security"].(string); !ok || sec == "Open" || sec == "" {
						currentNetwork["security"] = "WEP"
						log.Printf("Detected security: WEP from capability line")
					}
				}
			} else if strings.Contains(line, "WPS:") {
				// Si tiene WPS, probablemente es WPA2
				if sec, ok := currentNetwork["security"].(string); !ok || sec == "Open" || sec == "" {
					currentNetwork["security"] = "WPA2"
					log.Printf("Detected security: WPA2 from WPS line")
				}
			}
		}
		// Guardar última red si existe
		if ssid, ok := currentNetwork["ssid"].(string); ok && ssid != "" {
			networks = append(networks, fiber.Map{
				"ssid":     currentNetwork["ssid"],
				"signal":   currentNetwork["signal"],
				"security": currentNetwork["security"],
				"channel":  currentNetwork["channel"],
			})
		}
		
		// Eliminar duplicados basándose en SSID (mantener el que tiene mejor señal)
		seen := make(map[string]fiber.Map)
		for _, net := range networks {
			ssid, ok := net["ssid"].(string)
			if ok && ssid != "" {
				existing, exists := seen[ssid]
				if !exists {
					seen[ssid] = net
				} else {
					// Si ya existe, mantener el que tiene mejor señal
					existingSignal := 0
					currentSignal := 0
					if s, ok := existing["signal"].(int); ok {
						existingSignal = s
					}
					if s, ok := net["signal"].(int); ok {
						currentSignal = s
					}
					// Si la señal actual es mejor (más alta, menos negativa), reemplazar
					if currentSignal > existingSignal {
						seen[ssid] = net
					}
				}
			}
		}
		// Convertir map a slice
		uniqueNetworks := []fiber.Map{}
		for _, net := range seen {
			uniqueNetworks = append(uniqueNetworks, net)
		}
		networks = uniqueNetworks
		
		if len(networks) > 0 {
			log.Printf("✅ Encontradas %d redes únicas con iw", len(networks))
			return c.JSON(fiber.Map{
				"success":  true,
				"networks": networks,
			})
		}
	}

	// Si no hay redes encontradas, retornar array vacío con mensaje
	log.Printf("⚠️  No se encontraron redes WiFi")
	return c.JSON(fiber.Map{
		"success":  true,
		"error":    "No se encontraron redes WiFi. Verifica que WiFi esté habilitado y que haya redes disponibles en el área.",
		"networks": []fiber.Map{},
	})
}

// Middlewares
func securityMiddleware(c *fiber.Ctx) error {
	// Headers de seguridad
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "DENY")
	c.Set("X-XSS-Protection", "1; mode=block")
	c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	return c.Next()
}

	stats := fiber.Map{
		"cpu_usage":      0.0,
		"memory_usage":   0.0,
		"disk_usage":     0.0,
		"uptime":         0,
		"cpu_cores":      1,
		"hostname":       "unknown",
		"kernel_version": "unknown",
		"architecture":   "unknown",
		"os_version":     "Linux",
	}

	// Intentar obtener datos reales del sistema
	// CPU usage - usar /proc/stat (más confiable)
	cpuOut, cpuErr := executeCommand("grep 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$3+$4+$5)} END {print usage}'")
	if cpuErr == nil && strings.TrimSpace(cpuOut) != "" {
		// Normalizar: reemplazar coma por punto para locales que usan coma
		cpuOut = strings.ReplaceAll(strings.TrimSpace(cpuOut), ",", ".")
		if cpu, err := strconv.ParseFloat(cpuOut, 64); err == nil && cpu >= 0 && cpu <= 100 {
			stats["cpu_usage"] = cpu
			log.Printf("✅ CPU usage obtenido: %.2f%%", cpu)
		} else {
			log.Printf("⚠️  Error parseando CPU: %v, output: %s", err, cpuOut)
		}
	} else {
		log.Printf("⚠️  Error ejecutando comando CPU: %v", cpuErr)
	}

	// Fallback alternativo para CPU usando top
	if stats["cpu_usage"] == 0.0 {
		cpuOut2, cpuErr2 := executeCommand("top -bn1 | grep 'Cpu(s)' | awk -F'id,' '{split($1,a,\"%\"); for(i in a){if(a[i] ~ /^[0-9]/){print 100-a[i];break}}}'")
		if cpuErr2 == nil && strings.TrimSpace(cpuOut2) != "" {
			cpuOut2 = strings.ReplaceAll(strings.TrimSpace(cpuOut2), ",", ".")
			if cpu, err := strconv.ParseFloat(cpuOut2, 64); err == nil && cpu >= 0 && cpu <= 100 {
				stats["cpu_usage"] = cpu
				log.Printf("✅ CPU usage obtenido (fallback): %.2f%%", cpu)
			}
		}
	}

	// Memory usage
	memOut, memErr := executeCommand("free | grep Mem | awk '{printf \"%.2f\", $3/$2 * 100.0}'")
	if memErr == nil && strings.TrimSpace(memOut) != "" {
		// Normalizar: reemplazar coma por punto
		memOut = strings.ReplaceAll(strings.TrimSpace(memOut), ",", ".")
		if mem, err := strconv.ParseFloat(memOut, 64); err == nil && mem >= 0 && mem <= 100 {
			stats["memory_usage"] = mem
			log.Printf("✅ Memory usage obtenido: %.2f%%", mem)
		} else {
			log.Printf("⚠️  Error parseando Memory: %v, output: %s", err, memOut)
		}
	} else {
		log.Printf("⚠️  Error ejecutando comando Memory: %v", memErr)
	}

	// Disk usage - usar método más simple
	diskOut, diskErr := executeCommand("df / | tail -1 | awk '{print $5}' | sed 's/%//'")
	if diskErr == nil && strings.TrimSpace(diskOut) != "" {
		if disk, err := strconv.ParseFloat(strings.TrimSpace(diskOut), 64); err == nil && disk >= 0 && disk <= 100 {
			stats["disk_usage"] = disk
			log.Printf("✅ Disk usage obtenido: %.2f%%", disk)
		} else {
			log.Printf("⚠️  Error parseando Disk: %v, output: %s", err, diskOut)
		}
	} else {
		log.Printf("⚠️  Error ejecutando comando Disk: %v", diskErr)
	}

	// Uptime
	if uptimeOut, err := executeCommand("cat /proc/uptime | awk '{print int($1)}'"); err == nil {
		if uptime, err := strconv.Atoi(strings.TrimSpace(uptimeOut)); err == nil {
			stats["uptime"] = uptime
		}
	}

	// CPU cores
	if coresOut, err := executeCommand("nproc"); err == nil {
		if cores, err := strconv.Atoi(strings.TrimSpace(coresOut)); err == nil && cores > 0 {
			stats["cpu_cores"] = cores
		}
	}

	// Hostname
	if hostname, err := executeCommand("hostname"); err == nil && hostname != "" {
		stats["hostname"] = hostname
	}

	// Kernel version
	if kernel, err := executeCommand("uname -r"); err == nil && kernel != "" {
		stats["kernel_version"] = kernel
	}

	// Architecture
	if arch, err := executeCommand("uname -m"); err == nil && arch != "" {
		stats["architecture"] = arch
	}

	// OS version
	if osRelease, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(osRelease), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osVersion := strings.TrimPrefix(line, "PRETTY_NAME=")
				osVersion = strings.Trim(osVersion, "\"")
				if osVersion != "" {
					stats["os_version"] = osVersion
				}
				break
			}
		}
	}
