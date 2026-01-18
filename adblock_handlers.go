package main

import (
	"os"
	"os/exec"
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

	result["active"] = dnsmasqStatus == "active" || piholeStatus == "active"
	result["type"] = "none"

	if dnsmasqStatus == "active" {
		result["type"] = "dnsmasq"
	} else if piholeStatus == "active" {
		result["type"] = "pihole"
	}

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

	result["success"] = true
	result["message"] = "AdBlock deshabilitado"
	LogT("logs.adblock_disabled")
	return result
}
