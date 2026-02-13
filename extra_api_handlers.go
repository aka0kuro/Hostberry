package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

var (
	adblockDefaultLists = []struct {
		Name    string
		Domains int
	}{
		{Name: "StevenBlack", Domains: 50000},
		{Name: "OISD", Domains: 250000},
		{Name: "EasyList", Domains: 40000},
	}
	adblockDomainRegex = regexp.MustCompile(`^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

func currentUserInfo(c *fiber.Ctx) (string, *int) {
	user, ok := GetUser(c)
	if !ok || user == nil {
		return "unknown", nil
	}
	id := user.ID
	return user.Username, &id
}

func systemUpdatesExecuteHandler(c *fiber.Ctx) error {
	username, userID := currentUserInfo(c)
	_ = InsertLog("INFO", fmt.Sprintf("Actualización de sistema iniciada por %s", username), "system", userID)

	go func(user string, uid *int) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "sh", "-c", "sudo DEBIAN_FRONTEND=noninteractive apt-get update -y && sudo DEBIAN_FRONTEND=noninteractive apt-get upgrade -y")
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(filterSudoErrors(out))

		if ctx.Err() == context.DeadlineExceeded {
			_ = InsertLog("ERROR", "Actualización del sistema cancelada por timeout", "system", uid)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("Error en actualización del sistema (%s): %v", user, err)
			if output != "" {
				msg = msg + " | " + output
			}
			_ = InsertLog("ERROR", msg, "system", uid)
			return
		}

		_ = SetConfig("system_last_update", time.Now().Format(time.RFC3339))
		_ = InsertLog("INFO", fmt.Sprintf("Actualización del sistema completada por %s", user), "system", uid)
	}(username, userID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Actualización del sistema iniciada en segundo plano",
	})
}

func systemUpdatesProjectHandler(c *fiber.Ctx) error {
	username, userID := currentUserInfo(c)
	repoPath, err := os.Getwd()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "No se pudo determinar el directorio del proyecto"})
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Este despliegue no está en un repositorio git"})
	}

	_ = InsertLog("INFO", fmt.Sprintf("Actualización del proyecto iniciada por %s", username), "system", userID)

	go func(user string, uid *int, path string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "-C", path, "pull", "--ff-only")
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))

		if ctx.Err() == context.DeadlineExceeded {
			_ = InsertLog("ERROR", "Actualización del proyecto cancelada por timeout", "system", uid)
			return
		}
		if err != nil {
			msg := fmt.Sprintf("Error actualizando proyecto (%s): %v", user, err)
			if output != "" {
				msg = msg + " | " + output
			}
			_ = InsertLog("ERROR", msg, "system", uid)
			return
		}

		_ = SetConfig("project_last_update", time.Now().Format(time.RFC3339))
		_ = InsertLog("INFO", fmt.Sprintf("Proyecto actualizado correctamente por %s", user), "system", uid)
	}(username, userID, repoPath)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Actualización del proyecto iniciada en segundo plano",
	})
}

func getConfigValue(key string) string {
	value, err := GetConfig(key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func systemNotificationsTestEmailHandler(c *fiber.Ctx) error {
	var req struct {
		To string `json:"to"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload inválido"})
	}

	req.To = strings.TrimSpace(req.To)
	if req.To == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "El destinatario es obligatorio",
			"message": "El destinatario es obligatorio",
		})
	}
	if err := ValidateEmail(req.To); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   err.Error(),
			"message": err.Error(),
		})
	}

	smtpHost := getConfigValue("smtp_host")
	smtpPort := getConfigValue("smtp_port")
	smtpUser := getConfigValue("smtp_user")
	smtpPassword := getConfigValue("smtp_password")
	smtpFrom := getConfigValue("smtp_from")
	if smtpFrom == "" {
		smtpFrom = smtpUser
	}

	if smtpHost == "" || smtpPort == "" || smtpFrom == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Falta configuración SMTP",
			"message": "Rellena y guarda Host, Puerto y From antes de enviar email de prueba",
		})
	}
	if _, err := strconv.Atoi(smtpPort); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Puerto SMTP inválido",
			"message": "Puerto SMTP inválido",
		})
	}

	subject := "HostBerry: email de prueba"
	body := "Este es un email de prueba enviado por HostBerry."
	message := fmt.Sprintf(
		"To: %s\r\nFrom: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		req.To,
		smtpFrom,
		subject,
		body,
	)

	address := net.JoinHostPort(smtpHost, smtpPort)
	var auth smtp.Auth
	if smtpUser != "" && smtpPassword != "" {
		auth = smtp.PlainAuth("", smtpUser, smtpPassword, smtpHost)
	}

	if err := smtp.SendMail(address, auth, smtpFrom, []string{req.To}, []byte(message)); err != nil {
		username, userID := currentUserInfo(c)
		_ = InsertLog("ERROR", fmt.Sprintf("Error enviando email de prueba por %s: %v", username, err), "system", userID)
		return c.Status(500).JSON(fiber.Map{
			"error":   "No se pudo enviar el email de prueba",
			"detail":  "Revisa la configuración SMTP y vuelve a intentarlo",
			"message": "No se pudo enviar el email de prueba",
		})
	}

	username, userID := currentUserInfo(c)
	_ = InsertLog("INFO", fmt.Sprintf("Email de prueba enviado por %s a %s", username, req.To), "system", userID)
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Email de prueba enviado.",
	})
}

func adblockListsHandler(c *fiber.Ctx) error {
	disabled := getDisabledAdblockLists()
	lastUpdate := getConfigValue("adblock_last_update")
	if lastUpdate == "" {
		lastUpdate = "N/A"
	}

	lists := make([]fiber.Map, 0, len(adblockDefaultLists))
	for _, item := range adblockDefaultLists {
		key := strings.ToLower(item.Name)
		lists = append(lists, fiber.Map{
			"name":          item.Name,
			"enabled":       !disabled[key],
			"domains_count": item.Domains,
			"last_update":   lastUpdate,
		})
	}

	return c.JSON(lists)
}

func adblockDomainsHandler(c *fiber.Ctx) error {
	base := readBlockedDomainsFromHosts(400)
	overrides := getAdblockDomainOverrides()

	for domain, blocked := range overrides {
		base[domain] = blocked
	}

	names := make([]string, 0, len(base))
	for domain := range base {
		names = append(names, domain)
	}
	sort.Strings(names)
	if len(names) > 200 {
		names = names[:200]
	}

	domains := make([]fiber.Map, 0, len(names))
	for _, domain := range names {
		domains = append(domains, fiber.Map{
			"name":    domain,
			"blocked": base[domain],
		})
	}

	return c.JSON(domains)
}

func adblockUpdateHandler(c *fiber.Ctx) error {
	username, userID := currentUserInfo(c)

	if _, err := executeCommand("sudo systemctl reload dnsmasq 2>/dev/null || sudo systemctl restart dnsmasq 2>/dev/null || true"); err != nil {
		_ = InsertLog("ERROR", fmt.Sprintf("Error actualizando listas AdBlock (%s): %v", username, err), "adblock", userID)
		return c.Status(500).JSON(fiber.Map{
			"error": "No se pudieron actualizar las listas de AdBlock",
		})
	}

	_ = SetConfig("adblock_last_update", time.Now().Format(time.RFC3339))
	_ = InsertLog("INFO", fmt.Sprintf("Listas AdBlock actualizadas por %s", username), "adblock", userID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Listas de AdBlock actualizadas",
	})
}

func adblockToggleListHandler(c *fiber.Ctx) error {
	listName := strings.TrimSpace(c.Params("name"))
	if listName == "" || len(listName) > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "Nombre de lista inválido"})
	}

	disabled := getDisabledAdblockLists()
	key := strings.ToLower(listName)

	enabled := true
	if disabled[key] {
		delete(disabled, key)
		enabled = true
	} else {
		disabled[key] = true
		enabled = false
	}

	if err := saveDisabledAdblockLists(disabled); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "No se pudo guardar el estado de la lista"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"name":    listName,
		"enabled": enabled,
	})
}

func adblockToggleDomainHandler(c *fiber.Ctx) error {
	domain := strings.ToLower(strings.TrimSpace(c.Params("name")))
	if domain == "" || len(domain) > 255 || !adblockDomainRegex.MatchString(domain) {
		return c.Status(400).JSON(fiber.Map{"error": "Dominio inválido"})
	}

	base := readBlockedDomainsFromHosts(2000)
	overrides := getAdblockDomainOverrides()

	current := base[domain]
	if val, ok := overrides[domain]; ok {
		current = val
	}
	newValue := !current
	overrides[domain] = newValue

	if err := saveAdblockDomainOverrides(overrides); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "No se pudo guardar el estado del dominio"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"name":    domain,
		"blocked": newValue,
	})
}

func adblockConfigHandler(c *fiber.Ctx) error {
	var req struct {
		UpdateInterval string `json:"update_interval"`
		MaxLists       int    `json:"max_lists"`
		CacheSize      int    `json:"cache_size"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload inválido"})
	}

	req.UpdateInterval = strings.TrimSpace(req.UpdateInterval)
	switch req.UpdateInterval {
	case "daily", "weekly", "monthly":
	default:
		return c.Status(400).JSON(fiber.Map{"error": "update_interval inválido"})
	}

	if req.MaxLists < 1 || req.MaxLists > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "max_lists fuera de rango (1-100)"})
	}
	if req.CacheSize < 1 || req.CacheSize > 100000 {
		return c.Status(400).JSON(fiber.Map{"error": "cache_size fuera de rango (1-100000)"})
	}

	if err := SetConfig("adblock_update_interval", req.UpdateInterval); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "No se pudo guardar update_interval"})
	}
	if err := SetConfig("adblock_max_lists", strconv.Itoa(req.MaxLists)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "No se pudo guardar max_lists"})
	}
	if err := SetConfig("adblock_cache_size", strconv.Itoa(req.CacheSize)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "No se pudo guardar cache_size"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Configuración de AdBlock guardada",
	})
}

func getDisabledAdblockLists() map[string]bool {
	result := make(map[string]bool)
	raw := getConfigValue("adblock_disabled_lists")
	if raw == "" {
		return result
	}
	for _, part := range strings.Split(raw, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key != "" {
			result[key] = true
		}
	}
	return result
}

func saveDisabledAdblockLists(disabled map[string]bool) error {
	entries := make([]string, 0, len(disabled))
	for key, value := range disabled {
		if value {
			entries = append(entries, key)
		}
	}
	sort.Strings(entries)
	return SetConfig("adblock_disabled_lists", strings.Join(entries, ","))
}

func getAdblockDomainOverrides() map[string]bool {
	result := make(map[string]bool)
	raw := getConfigValue("adblock_domain_overrides")
	if raw == "" {
		return result
	}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func saveAdblockDomainOverrides(overrides map[string]bool) error {
	data, err := json.Marshal(overrides)
	if err != nil {
		return err
	}
	return SetConfig("adblock_domain_overrides", string(data))
}

func readBlockedDomainsFromHosts(limit int) map[string]bool {
	result := make(map[string]bool)
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return result
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if commentIndex := strings.Index(line, "#"); commentIndex >= 0 {
			line = strings.TrimSpace(line[:commentIndex])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := fields[0]
		if ip != "0.0.0.0" && ip != "127.0.0.1" {
			continue
		}

		for _, domain := range fields[1:] {
			domain = strings.ToLower(strings.TrimSpace(domain))
			if domain == "" || domain == "localhost" || !adblockDomainRegex.MatchString(domain) {
				continue
			}
			result[domain] = true
			if limit > 0 && len(result) >= limit {
				return result
			}
		}
	}
	return result
}
