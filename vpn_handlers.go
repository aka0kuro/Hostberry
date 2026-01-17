package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func getVPNStatus() map[string]interface{} {
	result := make(map[string]interface{})

	openvpnCmd := exec.Command("sh", "-c", "systemctl is-active openvpn 2>/dev/null || pgrep openvpn > /dev/null && echo active || echo inactive")
	openvpnOut, _ := openvpnCmd.Output()
	openvpnStatus := strings.TrimSpace(string(openvpnOut))
	if openvpnStatus == "" {
		openvpnStatus = "inactive"
	}

	result["openvpn"] = map[string]interface{}{
		"active": openvpnStatus == "active",
		"status": openvpnStatus,
	}

	wgCmd := exec.Command("sh", "-c", "wg show 2>/dev/null | head -1")
	wgOut, _ := wgCmd.Output()
	wgActive := strings.TrimSpace(string(wgOut)) != ""

	result["wireguard"] = map[string]interface{}{
		"active":     wgActive,
		"interfaces": []string{},
	}

	if wgActive {
		wgInterfacesCmd := exec.Command("sh", "-c", "wg show interfaces 2>/dev/null")
		if wgInterfacesOut, err := wgInterfacesCmd.Output(); err == nil {
			interfaces := strings.Split(strings.TrimSpace(string(wgInterfacesOut)), "\n")
			interfaceList := []string{}
			for _, iface := range interfaces {
				iface = strings.TrimSpace(iface)
				if iface != "" {
					interfaceList = append(interfaceList, iface)
				}
			}
			result["wireguard"] = map[string]interface{}{
				"active":     wgActive,
				"interfaces": interfaceList,
			}
		}
	}

	result["success"] = true
	return result
}

func connectVPN(config, vpnType, user string) map[string]interface{} {
	result := make(map[string]interface{})

	if config == "" {
		result["success"] = false
		result["error"] = "Configuración requerida"
		return result
	}

	if vpnType == "" {
		vpnType = "openvpn"
	}
	if user == "" {
		user = "unknown"
	}

	log.Printf("Conectando VPN tipo: %s (usuario: %s)", vpnType, user)

	if vpnType == "openvpn" {
		configFile := "/etc/openvpn/client.conf"
		if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
			result["success"] = false
			result["error"] = fmt.Sprintf("Error guardando configuración: %v", err)
			return result
		}

		cmd := "sudo systemctl start openvpn@client"
		if out, err := executeCommand(cmd); err != nil {
			result["success"] = false
			result["error"] = err.Error()
			if out != "" {
				result["error"] = strings.TrimSpace(out)
			}
		} else {
			result["success"] = true
			result["message"] = "OpenVPN iniciado"
		}
	} else if vpnType == "wireguard" {
		configFile := "/etc/wireguard/wg0.conf"
		if err := os.WriteFile(configFile, []byte(config), 0600); err != nil {
			result["success"] = false
			result["error"] = fmt.Sprintf("Error guardando configuración: %v", err)
			return result
		}

		cmd := "sudo wg-quick up wg0"
		if out, err := executeCommand(cmd); err != nil {
			result["success"] = false
			result["error"] = err.Error()
			if out != "" {
				result["error"] = strings.TrimSpace(out)
			}
		} else {
			result["success"] = true
			result["message"] = "WireGuard activado"
		}
	} else {
		result["success"] = false
		result["error"] = fmt.Sprintf("Tipo de VPN no soportado: %s", vpnType)
	}

	return result
}

func getWireGuardStatus() map[string]interface{} {
	result := make(map[string]interface{})

	wgCmd := exec.Command("sh", "-c", "wg show 2>/dev/null")
	wgOut, _ := wgCmd.Output()
	wgActive := strings.TrimSpace(string(wgOut)) != ""

	result["active"] = wgActive
	result["interfaces"] = []map[string]interface{}{}

	if wgActive {
		interfacesCmd := exec.Command("sh", "-c", "wg show interfaces 2>/dev/null")
		if interfacesOut, err := interfacesCmd.Output(); err == nil {
			interfaces := strings.Split(strings.TrimSpace(string(interfacesOut)), "\n")
			interfaceList := []map[string]interface{}{}

			for _, iface := range interfaces {
				iface = strings.TrimSpace(iface)
				if iface != "" {
					interfaceInfo := map[string]interface{}{
						"name": iface,
					}

					detailsCmd := exec.Command("sh", "-c", fmt.Sprintf("wg show %s 2>/dev/null", iface))
					if detailsOut, err := detailsCmd.Output(); err == nil {
						interfaceInfo["details"] = strings.TrimSpace(string(detailsOut))
					} else {
						interfaceInfo["details"] = ""
					}

					interfaceList = append(interfaceList, interfaceInfo)
				}
			}

			result["interfaces"] = interfaceList
		}
	} else {
		result["message"] = "WireGuard no está activo"
	}

	result["success"] = true
	return result
}

func configureWireGuard(config, user string) map[string]interface{} {
	result := make(map[string]interface{})

	if config == "" {
		result["success"] = false
		result["error"] = "Configuración requerida"
		return result
	}

	if user == "" {
		user = "unknown"
	}

	log.Printf("Configurando WireGuard (usuario: %s)", user)

	configFile := "/etc/wireguard/wg0.conf"
	if err := os.WriteFile(configFile, []byte(config), 0600); err != nil {
		result["success"] = false
		result["error"] = fmt.Sprintf("Error guardando configuración: %v", err)
		result["message"] = "Error guardando configuración"
		log.Printf("ERROR: Error guardando configuración WireGuard: %v", err)
		return result
	}

	wgStatusCmd := exec.Command("sh", "-c", "wg show wg0 2>/dev/null")
	if wgStatusOut, err := wgStatusCmd.Output(); err == nil && strings.TrimSpace(string(wgStatusOut)) != "" {
		executeCommand("sudo wg-quick down wg0 2>/dev/null")
		executeCommand("sudo wg-quick up wg0 2>/dev/null")
		result["message"] = "WireGuard reiniciado con nueva configuración"
	} else {
		result["message"] = "Configuración guardada (no activa)"
	}

	result["success"] = true
	log.Printf("INFO: WireGuard configurado exitosamente")
	return result
}
