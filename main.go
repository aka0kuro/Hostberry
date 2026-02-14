package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
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

var templatesFS embed.FS

var staticFS embed.FS

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
	JWTSecret      string `yaml:"jwt_secret"`
	TokenExpiry    int    `yaml:"token_expiry"`    // minutos
	BcryptCost     int    `yaml:"bcrypt_cost"`
	RateLimitRPS   int    `yaml:"rate_limit_rps"`
	LockoutMinutes int    `yaml:"lockout_minutes"` // duración del bloqueo por intentos fallidos (0 = indefinido)
}

var appConfig Config

func generateRandomSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	}
	return hex.EncodeToString(bytes)
}

func main() {
	if err := loadConfig(); err != nil {
		LogTfatal("logs.config_load_error", err)
	}

	if err := InitI18n("locales"); err != nil {
		LogTf("logs.i18n_init_warning", err)
	}

	if err := initDatabase(); err != nil {
		LogTfatal("logs.db_init_error", err)
	}

	// Establecer idioma de logs desde configuración del sistema (después de inicializar BD)
	if configs, err := GetAllConfigs(); err == nil {
		if lang, ok := configs["log_language"]; ok && lang != "" {
			SetLogLanguage(lang)
		}
	}

	LogTln("logs.checking_admin")
	createDefaultAdmin()

	// Iniciar autoconexión WiFi en segundo plano
	go func() {
		// Esperar menos tiempo (5 segundos es suficiente)
		LogTf("logs.wifi_auto_wait")
		time.Sleep(5 * time.Second)

		// Intentar detectar interfaz (menos intentos, más rápido)
		var interfaceName string
		for attempt := 0; attempt < 3; attempt++ {
			interfaceName = detectWiFiInterface()
			if interfaceName != "" {
				// Verificar que la interfaz realmente existe
				cmd := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null", interfaceName))
				if err := cmd.Run(); err == nil {
					LogTf("logs.wifi_interface_detected", interfaceName)
					break
				}
			}
			if attempt < 2 {
				LogTf("logs.wifi_interface_wait", attempt+1)
				time.Sleep(2 * time.Second)
			}
		}

		if interfaceName != "" {
			LogTf("logs.wifi_auto_start", interfaceName)
			autoConnectToLastNetwork(interfaceName)
		} else {
			LogT("logs.wifi_interface_not_found")
		}
	}()

	app := createApp()

	setupRoutes(app)

	addr := fmt.Sprintf("%s:%d", appConfig.Server.Host, appConfig.Server.Port)
	LogTf("logs.server_starting", addr)
	LogTf("logs.server_config",
		appConfig.Server.Debug,
		appConfig.Server.ReadTimeout,
		appConfig.Server.WriteTimeout)

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint
		LogTln("logs.server_stopping")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			LogTf("logs.server_shutdown_error", err)
		}
		os.Exit(0)
	}()

	LogTf("logs.server_ready", addr)
	if err := app.Listen(addr); err != nil {
		LogTfatal("logs.server_start_error", err)
	}
}

func loadConfig() error {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
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
				JWTSecret:      generateRandomSecret(),
				TokenExpiry:    60,  // 1 hora
				BcryptCost:     10,
				RateLimitRPS:   10,
				LockoutMinutes: 15, // bloqueo por intentos fallidos
			},
		}
		return nil
	}

	return yaml.Unmarshal(data, &appConfig)
}

func createApp() *fiber.App {
	engine := createTemplateEngine()
	if engine == nil {
		LogTfatal("logs.template_engine_error")
	}

	LogT("logs.template_engine_created")

	app := fiber.New(fiber.Config{
		Views:        engine,
		ReadTimeout:  time.Duration(appConfig.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(appConfig.Server.WriteTimeout) * time.Second,
		ErrorHandler: errorHandler,
	})

	if app.Config().Views == nil {
		LogTfatal("logs.template_views_error")
	}
	LogTln("logs.template_views_ok")

	setupStaticFiles(app)

	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n",
		TimeFormat: "15:04:05",
		TimeZone:   "Local",
		Output:     os.Stdout,
		Next: func(c *fiber.Ctx) bool {
			path := c.Path()
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
	corsOrigins := "*"
	if !appConfig.Server.Debug {
		corsOrigins = "http://localhost:" + fmt.Sprintf("%d", appConfig.Server.Port) + ",http://127.0.0.1:" + fmt.Sprintf("%d", appConfig.Server.Port)
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowCredentials: true,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Content-Type,Authorization",
		MaxAge:           3600,
	}))

	app.Use(loggingMiddleware)

	app.Use(LanguageMiddleware)

	app.Use(requestIDMiddleware)

	app.Use("/api/", rateLimitMiddleware)

	return app
}

func setupStaticFiles(app *fiber.App) {
	if _, err := os.Stat("./website/static"); err == nil {
		app.Static("/static", "./website/static", fiber.Static{
			Compress:  true,
			ByteRange: true,
		})
		LogTln("logs.static_files_loaded")
	} else {
		staticSubFS, err := fs.Sub(staticFS, "website/static")
		if err != nil {
			LogTf("logs.static_files_embed_error", err)
			LogT("logs.static_files_not_found")
		} else {
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
			LogTln("logs.static_files_embedded")
		}
	}
}

func setupRoutes(app *fiber.App) {
	app.Get("/health", healthCheckHandler)
	app.Get("/health/ready", readinessCheckHandler)
	app.Get("/health/live", livenessCheckHandler)

	web := app.Group("/")
	{
		web.Get("/login", loginHandler)
		web.Get("/first-login", firstLoginPageHandler)
		web.Get("/", indexHandler)

		protected := web.Group("/", requireAuth)
		protected.Get("/dashboard", dashboardHandler)
		protected.Get("/settings", settingsHandler)
		protected.Get("/network", networkPageHandler)
		protected.Get("/wifi", wifiPageHandler)
		protected.Get("/wifi-scan", wifiScanPageHandler)
		protected.Get("/vpn", vpnPageHandler)
		protected.Get("/wireguard", wireguardPageHandler)
		protected.Get("/adblock", adblockPageHandler)
		protected.Get("/tor", torPageHandler)
		protected.Get("/hostapd", hostapdPageHandler)
		protected.Get("/setup-wizard", setupWizardPageHandler)
		protected.Get("/setup-wizard/vpn", setupWizardVpnPageHandler)
		protected.Get("/setup-wizard/wireguard", setupWizardWireguardPageHandler)
		protected.Get("/setup-wizard/tor", setupWizardTorPageHandler)
		protected.Get("/profile", profilePageHandler)
		protected.Get("/system", systemPageHandler)
		protected.Get("/monitoring", monitoringPageHandler)
		protected.Get("/update", updatePageHandler)
	}

	api := app.Group("/api/v1")
	{
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
			system.Post("/updates/execute", systemUpdatesExecuteHandler)
			system.Post("/updates/project", systemUpdatesProjectHandler)
			system.Post("/notifications/test-email", systemNotificationsTestEmailHandler)
			system.Post("/restart", systemRestartHandler)
			system.Post("/shutdown", systemShutdownHandler)
		}

		network := api.Group("/network", requireAuth)
		{
			network.Get("/status", networkStatusHandler)
			network.Get("/interfaces", networkInterfacesHandler)
			network.Get("/routing", networkRoutingHandler)
			network.Post("/firewall/toggle", networkFirewallToggleHandler)
			network.Post("/speedtest", networkSpeedtestHandler)
			network.Get("/config", networkConfigHandler)
			network.Post("/config", networkConfigHandler)
		}

		wifi := api.Group("/wifi", requireAuth)
		{
			wifi.Get("/status", wifiStatusHandler)
			wifi.Get("/scan", wifiScanHandler)
			wifi.Post("/scan", wifiScanHandler)
			wifi.Get("/interfaces", wifiInterfacesHandler)
			wifi.Post("/connect", wifiConnectHandler)
			wifi.Post("/disconnect", wifiLegacyDisconnectHandler)
			wifi.Get("/networks", wifiNetworksHandler)
			wifi.Get("/clients", wifiClientsHandler)
			wifi.Post("/toggle", wifiToggleHandler)
			wifi.Post("/unblock", wifiUnblockHandler)
			wifi.Post("/software-switch", wifiSoftwareSwitchHandler)
			wifi.Post("/config", wifiConfigHandler)
		}

		vpn := api.Group("/vpn", requireAuth)
		{
			vpn.Get("/status", vpnStatusHandler)
			vpn.Get("/config", vpnGetConfigHandler)
			vpn.Post("/connect", vpnConnectHandler)
			vpn.Get("/connections", vpnConnectionsHandler)
			vpn.Get("/servers", vpnServersHandler)
			vpn.Get("/clients", vpnClientsHandler)
			vpn.Post("/toggle", vpnToggleHandler)
			vpn.Post("/config", vpnConfigHandler)
			vpn.Post("/connections/:name/toggle", vpnConnectionToggleHandler)
			vpn.Post("/certificates/generate", vpnCertificatesGenerateHandler)
		}

		hostapd := api.Group("/hostapd", requireAuth)
		{
			hostapd.Get("/access-points", hostapdAccessPointsHandler)
			hostapd.Get("/clients", hostapdClientsHandler)
			hostapd.Get("/config", hostapdGetConfigHandler)
			hostapd.Get("/diagnostics", hostapdDiagnosticsHandler)
			hostapd.Post("/create-ap0", hostapdCreateAp0Handler)
			hostapd.Post("/toggle", hostapdToggleHandler)
			hostapd.Post("/restart", hostapdRestartHandler)
			hostapd.Post("/config", hostapdConfigHandler)
		}

		help := api.Group("/help", requireAuth)
		{
			help.Post("/contact", helpContactHandler)
		}

		api.Get("/translations/:lang", translationsHandler)

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

		adblock := api.Group("/adblock", requireAuth)
		{
			adblock.Get("/status", adblockStatusHandler)
			adblock.Get("/lists", adblockListsHandler)
			adblock.Get("/domains", adblockDomainsHandler)
			adblock.Post("/enable", adblockEnableHandler)
			adblock.Post("/disable", adblockDisableHandler)
			adblock.Post("/update", adblockUpdateHandler)
			adblock.Post("/lists/:name/toggle", adblockToggleListHandler)
			adblock.Post("/domains/:name/toggle", adblockToggleDomainHandler)
			adblock.Post("/config", adblockConfigHandler)

			// DNSCrypt (sub-sección de AdBlock)
			dnscrypt := adblock.Group("/dnscrypt")
			{
				dnscrypt.Get("/status", dnscryptStatusHandler)
				dnscrypt.Post("/install", dnscryptInstallHandler)
				dnscrypt.Post("/configure", dnscryptConfigureHandler)
				dnscrypt.Post("/enable", dnscryptEnableHandler)
				dnscrypt.Post("/disable", dnscryptDisableHandler)
			}

			// Blocky (proxy DNS y ad-blocker, configuración desde la web)
			adblock.Get("/blocky/status", blockyStatusHandler)
			adblock.Get("/blocky/config", blockyConfigHandler)
			adblock.Post("/blocky/install", blockyInstallHandler)
			adblock.Post("/blocky/configure", blockyConfigureHandler)
			adblock.Post("/blocky/enable", blockyEnableHandler)
			adblock.Post("/blocky/disable", blockyDisableHandler)
			adblock.Get("/blocky/api/*", blockyAPIProxyHandler)
			adblock.Post("/blocky/api/*", blockyAPIProxyHandler)
		}

		tor := api.Group("/tor", requireAuth)
		{
			tor.Get("/status", torStatusHandler)
			tor.Post("/install", torInstallHandler)
			tor.Post("/configure", torConfigureHandler)
			tor.Post("/enable", torEnableHandler)
			tor.Post("/disable", torDisableHandler)
			tor.Get("/circuit", torCircuitHandler)
			tor.Post("/iptables-enable", torIptablesEnableHandler)
			tor.Post("/iptables-disable", torIptablesDisableHandler)
		}
	}

	wifiLegacy := app.Group("/api/wifi", requireAuth)
	wifiLegacy.Get("/status", wifiLegacyStatusHandler)
	wifiLegacy.Get("/stored_networks", wifiLegacyStoredNetworksHandler)
	wifiLegacy.Get("/autoconnect", wifiLegacyAutoconnectHandler)
	wifiLegacy.Get("/scan", wifiLegacyScanHandler)
	wifiLegacy.Post("/disconnect", wifiLegacyDisconnectHandler)
}

func indexHandler(c *fiber.Ctx) error {
	token := c.Cookies("access_token")

	if token != "" {
		claims, err := ValidateToken(token)
		if err == nil {
			var user User
			if err := db.First(&user, claims.UserID).Error; err == nil && user.IsActive {
				return c.Redirect("/dashboard")
			}
		}
	}

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

func systemStatsHandler(c *fiber.Ctx) error {
	stats := getSystemStats()
	return c.JSON(stats)
}

func systemRestartHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
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

func detectWiFiInterface() string {
	cmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl' | head -1")
	out, err := cmd.Output()
	if err == nil {
		iface := strings.TrimSpace(string(out))
		if iface != "" {
			return iface
		}
	}

	return DefaultWiFiInterface
}

func wifiInterfacesHandler(c *fiber.Ctx) error {
	var interfaces []fiber.Map

	cmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl'")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, ifaceName := range lines {
			ifaceName = strings.TrimSpace(ifaceName)
			if ifaceName != "" {
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

	if len(interfaces) == 0 {
		interfaces = append(interfaces, fiber.Map{
			"name":  DefaultWiFiInterface,
			"type":  "wifi",
			"state": "unknown",
		})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"interfaces": interfaces,
	})
}

func wifiScanHandler(c *fiber.Ctx) error {
	userInterface := c.Locals("user")
	if userInterface == nil {
		LogT("logs.user_not_found")
		return c.Status(401).JSON(fiber.Map{
			"error": "No autenticado. Por favor, inicia sesión nuevamente.",
		})
	}

	user, ok := userInterface.(*User)
	if !ok || user == nil {
		LogT("logs.user_invalid")
		return c.Status(401).JSON(fiber.Map{
			"error": "Usuario no encontrado. Por favor, inicia sesión nuevamente.",
		})
	}

	interfaceName := c.Query("interface", "")
	if interfaceName == "" {
		var req struct {
			Interface string `json:"interface"`
		}
		if err := c.BodyParser(&req); err == nil {
			interfaceName = req.Interface
		}
	}

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
