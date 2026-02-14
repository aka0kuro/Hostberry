package main

import (
	"encoding/json"
	"fmt"
	"log"
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

func systemActivityHandler(c *fiber.Ctx) error {
	limitStr := c.Query("limit", "10")
	limit := 10
	if v, err := strconvAtoiSafe(limitStr); err == nil && v > 0 && v <= 100 {
		limit = v
	}

	logs, _, err := GetLogs("all", limit, 0)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	var activities []fiber.Map
	for _, l := range logs {
		activities = append(activities, fiber.Map{
			"timestamp": l.CreatedAt.Format(time.RFC3339),
			"level":     l.Level,
			"message":   l.Message,
			"source":    l.Source,
		})
	}

	return c.JSON(activities)
}

func systemNetworkHandler(c *fiber.Ctx) error {
	out, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"raw": string(out)})
}

func systemUpdatesHandler(c *fiber.Ctx) error {
	commands := []string{
		"apt list --upgradable 2>/dev/null | tail -n +2 | cut -d/ -f1",
		"apt-get -s upgrade 2>/dev/null | awk '/^Inst /{print $2}'",
	}

	pkgSet := make(map[string]struct{})
	for _, cmdText := range commands {
		out, err := exec.Command("sh", "-c", cmdText).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			pkg := strings.TrimSpace(line)
			if pkg == "" {
				continue
			}
			pkgSet[pkg] = struct{}{}
		}
		if len(pkgSet) > 0 {
			break
		}
	}

	updates := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		updates = append(updates, pkg)
	}
	sort.Strings(updates)

	return c.JSON(fiber.Map{
		"success":           true,
		"updates_available": len(updates) > 0,
		"update_count":      len(updates),
		"updates":           updates,
		"available":         len(updates) > 0, // compatibilidad con clientes antiguos
	})
}

func systemBackupHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"success": false, "message": "Backup no implementado aún"})
}

func networkRoutingHandler(c *fiber.Ctx) error {
	out, err := exec.Command("sh", "-c", "ip route 2>/dev/null").CombinedOutput()
	if err != nil {
		LogTf("logs.api_route_error", err, string(out))
		return c.Status(500).JSON(fiber.Map{"error": strings.TrimSpace(string(out))})
	}
	var routes []fiber.Map
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	LogTf("logs.api_processing_routes", len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		route := fiber.Map{"raw": line}
		route["destination"] = parts[0]

		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "via" && i+1 < len(parts) {
				route["gateway"] = parts[i+1]
			}
			if parts[i] == "dev" && i+1 < len(parts) {
				route["interface"] = parts[i+1]
			}
			if parts[i] == "metric" && i+1 < len(parts) {
				route["metric"] = parts[i+1]
			}
		}

		if _, hasGateway := route["gateway"]; !hasGateway {
			route["gateway"] = "*"
		}

		if _, hasInterface := route["interface"]; !hasInterface {
			route["interface"] = "-"
		}

		if _, hasMetric := route["metric"]; !hasMetric {
			route["metric"] = "0"
		}

		routes = append(routes, route)
	}

	LogTf("logs.api_returning_routes", len(routes))
	return c.JSON(routes)
}

func networkFirewallToggleHandler(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{"error": "Firewall toggle no implementado"})
}

func networkConfigHandler(c *fiber.Ctx) error {
	if c.Method() == "GET" {
		config := fiber.Map{
			"hostname": "",
			"gateway":  "",
			"dns1":     "",
			"dns2":     "",
		}

		hostnameCmd := exec.Command("sh", "-c", "hostnamectl --static 2>/dev/null || hostname 2>/dev/null || echo ''")
		if hostnameOut, err := hostnameCmd.Output(); err == nil {
			config["hostname"] = strings.TrimSpace(string(hostnameOut))
		}

		gatewayCmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}' | head -1")
		if gatewayOut, err := gatewayCmd.Output(); err == nil {
			gateway := strings.TrimSpace(string(gatewayOut))
			if gateway != "" {
				config["gateway"] = gateway
			}
		}

		dnsCmd := exec.Command("sh", "-c", "nmcli -t -f IP4.DNS connection show $(nmcli -t -f NAME connection show --active | head -1) 2>/dev/null | head -2")
		if dnsOut, err := dnsCmd.Output(); err == nil {
			dnsLines := strings.Split(strings.TrimSpace(string(dnsOut)), "\n")
			for i, dns := range dnsLines {
				dns = strings.TrimSpace(dns)
				if dns != "" && strings.Contains(dns, ":") {
					parts := strings.Split(dns, ":")
					if len(parts) > 1 {
						dns = parts[len(parts)-1]
					}
					if i == 0 {
						config["dns1"] = dns
					} else if i == 1 {
						config["dns2"] = dns
					}
				} else if dns != "" && !strings.Contains(dns, ":") {
					if i == 0 {
						config["dns1"] = dns
					} else if i == 1 {
						config["dns2"] = dns
					}
				}
			}
		}

		if config["dns1"] == "" {
			resolveCmd := exec.Command("sh", "-c", "resolvectl dns 2>/dev/null | grep -E '^[0-9]' | awk '{print $2}' | head -2")
			if resolveOut, err := resolveCmd.Output(); err == nil {
				resolveLines := strings.Split(strings.TrimSpace(string(resolveOut)), "\n")
				for i, dns := range resolveLines {
					dns = strings.TrimSpace(dns)
					if dns != "" {
						if i == 0 {
							config["dns1"] = dns
						} else if i == 1 {
							config["dns2"] = dns
						}
					}
				}
			}
		}

		if config["dns1"] == "" {
			resolvCmd := exec.Command("sh", "-c", "grep '^nameserver' /etc/resolv.conf 2>/dev/null | awk '{print $2}' | head -2")
			if resolvOut, err := resolvCmd.Output(); err == nil {
				resolvLines := strings.Split(strings.TrimSpace(string(resolvOut)), "\n")
				for i, dns := range resolvLines {
					dns = strings.TrimSpace(dns)
					if dns != "" && dns != "127.0.0.1" && dns != "127.0.0.53" {
						if i == 0 {
							config["dns1"] = dns
						} else if i == 1 {
							config["dns2"] = dns
						}
					}
				}
			}
		}

		return c.JSON(config)
	}

	var req struct {
		Hostname string `json:"hostname"`
		DNS1     string `json:"dns1"`
		DNS2     string `json:"dns2"`
		Gateway  string `json:"gateway"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	errors := []string{}
	applied := []string{}

	if req.Hostname != "" {
		if len(req.Hostname) > 64 || len(req.Hostname) < 1 {
			errors = append(errors, "Hostname must be between 1 and 64 characters")
		} else {
			hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-\.]*[a-zA-Z0-9])?$`)
			if !hostnameRegex.MatchString(req.Hostname) {
				errors = append(errors, "Hostname contains invalid characters. Use only letters, numbers, hyphens and dots. Cannot start or end with hyphen or dot.")
			} else {
				hostnameApplied := false
				var lastError error
				var lastOutput string

				if !hostnameApplied {
					cmd := fmt.Sprintf("sudo hostnamectl set-hostname %s", req.Hostname)
					if out, err := executeCommand(cmd); err == nil {
						time.Sleep(500 * time.Millisecond)
						verifyCmd := exec.Command("sh", "-c", "hostnamectl --static 2>/dev/null || hostname 2>/dev/null || echo ''")
						if verifyOut, err := verifyCmd.Output(); err == nil {
							currentHostname := strings.TrimSpace(string(verifyOut))
							if currentHostname == req.Hostname {
								hostnameApplied = true
								LogTf("logs.api_hostname_set", req.Hostname)
								_ = out
							} else {
								LogTf("logs.api_hostname_verify_failed", req.Hostname, currentHostname)
								lastError = fmt.Errorf("verification failed: got %s", currentHostname)
								lastOutput = out
							}
						} else {
							LogTf("logs.api_hostname_verify_error", err)
							lastError = err
							lastOutput = out
						}
					} else {
						LogTf("logs.api_hostnamectl_failed", err, out)
						lastError = err
						lastOutput = out
					}
				}

				if !hostnameApplied {
					hostnameFile := "/etc/hostname"
					tmpHostname := "/tmp/hostname_hostberry_" + fmt.Sprintf("%d", time.Now().Unix())
					if wErr := os.WriteFile(tmpHostname, []byte(req.Hostname+"\n"), 0644); wErr == nil {
						defer os.Remove(tmpHostname)
						cpCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo cp -f %s %s", tmpHostname, hostnameFile))
						cpOut, cpErr := cpCmd.CombinedOutput()
						out := strings.TrimSpace(string(cpOut))
						if cpErr == nil {
							if content, err := os.ReadFile(hostnameFile); err == nil {
								writtenHostname := strings.TrimSpace(string(content))
								if writtenHostname == req.Hostname {
									applyCmdStr := fmt.Sprintf("sudo hostname %s", req.Hostname)
									if applyOut, applyErr := executeCommand(applyCmdStr); applyErr == nil {
										verifyCmd := exec.Command("hostname")
										if verifyOut, err := verifyCmd.Output(); err == nil {
											currentHostname := strings.TrimSpace(string(verifyOut))
											if currentHostname == req.Hostname {
												hostnameApplied = true
												LogTf("logs.api_hostname_file_set", req.Hostname)
												_ = applyOut
											} else {
												LogTf("logs.api_hostname_file_verify_failed", req.Hostname, currentHostname)
												lastError = fmt.Errorf("verification failed: got %s", currentHostname)
												lastOutput = applyOut
											}
										}
									} else {
										LogTf("logs.api_hostname_apply_failed", applyErr, applyOut)
										lastError = applyErr
										lastOutput = applyOut
									}
								} else {
									LogTf("logs.api_hostname_write_mismatch", req.Hostname, writtenHostname)
									lastError = fmt.Errorf("written hostname mismatch")
									lastOutput = out
								}
							} else {
								LogTf("logs.api_hostname_read_failed", err)
								lastError = err
								lastOutput = out
							}
						} else {
							LogTf("logs.api_hostname_write_failed", cpErr, out)
							lastError = cpErr
							lastOutput = out
						}
					}
				}

				if !hostnameApplied {
					cmd := fmt.Sprintf("sudo hostname %s", req.Hostname)
					if out, err := executeCommand(cmd); err == nil {
						time.Sleep(200 * time.Millisecond)
						verifyCmd := exec.Command("hostname")
						if verifyOut, err := verifyCmd.Output(); err == nil {
							currentHostname := strings.TrimSpace(string(verifyOut))
							if currentHostname == req.Hostname {
								hostnameApplied = true
								LogTf("logs.api_hostname_temp_set", req.Hostname)
								_ = out
							} else {
								LogTf("logs.api_hostname_temp_verify_failed", req.Hostname, currentHostname)
								lastError = fmt.Errorf("verification failed: got %s", currentHostname)
								lastOutput = out
							}
						} else {
							LogTf("logs.api_hostname_verify_error2", err)
							lastError = err
							lastOutput = out
						}
					} else {
						LogTf("logs.api_hostname_cmd_failed", err, out)
						lastError = err
						lastOutput = out
					}
				}

				if hostnameApplied {
					hostsFile := "/etc/hosts"
					tmpFile := "/tmp/hosts_hostberry_" + fmt.Sprintf("%d", time.Now().Unix())

					LogTf("logs.api_hosts_creating", req.Hostname)

					newContent := "# See `man hosts` for details.\n"
					newContent += "#\n"
					newContent += "# By default, systemd-resolved or libnss-myhostname will resolve\n"
					newContent += "# localhost and the system hostname if they're not specified here.\n"

					if req.Hostname != "" {
						newContent += fmt.Sprintf("127.0.0.1\tlocalhost\t%s\n", req.Hostname)
					} else {
						newContent += "127.0.0.1\tlocalhost\n"
					}

					newContent += "::1\t\tlocalhost\n"

					if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
						LogTf("logs.api_hosts_temp_error", err)
					} else {
						log.Printf("Created new hosts file in /tmp: %s", tmpFile)
						log.Printf("File content:\n%s", newContent)

						if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
							log.Printf("Error: Temp file does not exist after creation")
						} else {
							log.Printf("Temp file verified: exists and readable")

							copySuccess := false
							cpPath := "/bin/cp"
							if _, err := os.Stat(cpPath); os.IsNotExist(err) {
								cpPath = "/usr/bin/cp"
							}

							log.Printf("Attempting to copy with cp: %s -f %s %s", cpPath, tmpFile, hostsFile)
							cpCmd := exec.Command("sudo", cpPath, "-f", tmpFile, hostsFile)
							cpCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
							cpOut, cpErr := cpCmd.CombinedOutput()

							time.Sleep(200 * time.Millisecond) // Pausa para asegurar que el archivo se escribió
							if content, err := os.ReadFile(hostsFile); err == nil {
								if strings.Contains(string(content), req.Hostname) {
									log.Printf("Successfully copied with cp - hostname found in /etc/hosts")
									copySuccess = true
								} else {
									if cpErr != nil {
										log.Printf("Error with cp: %v, output: %s", cpErr, string(cpOut))
									} else {
										log.Printf("cp command executed but hostname not found in /etc/hosts")
									}
									log.Printf("Current /etc/hosts content:\n%s", string(content))

									if strings.Contains(string(cpOut), "Read-only file system") || strings.Contains(string(cpOut), "Read-only") {
										log.Printf("Warning: /etc/hosts appears to be on a read-only file system")
										log.Printf("Attempting to remount /etc as read-write...")
										remountCmd := exec.Command("sudo", "mount", "-o", "remount,rw", "/")
										remountCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
										if remountOut, remountErr := remountCmd.CombinedOutput(); remountErr != nil {
											log.Printf("Could not remount / as read-write: %v, output: %s", remountErr, string(remountOut))
										} else {
											log.Printf("Successfully remounted / as read-write")
											cpCmd2 := exec.Command("sudo", cpPath, "-f", tmpFile, hostsFile)
											cpCmd2.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
											if _, cpErr2 := cpCmd2.CombinedOutput(); cpErr2 == nil {
												time.Sleep(200 * time.Millisecond)
												if content2, err2 := os.ReadFile(hostsFile); err2 == nil {
													if strings.Contains(string(content2), req.Hostname) {
														log.Printf("Successfully copied after remount - hostname found in /etc/hosts")
														copySuccess = true
													}
												}
											}
										}
									}
								}
							} else {
								if cpErr != nil {
									log.Printf("Error with cp: %v, output: %s", cpErr, string(cpOut))
								} else {
									log.Printf("cp command executed but could not read /etc/hosts: %v", err)
								}
							}

							if !copySuccess {
								log.Printf("Trying alternative method: cat with tee")
								catCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo cat %s | sudo tee %s > /dev/null", tmpFile, hostsFile))
								catCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
								if catOut, catErr := catCmd.CombinedOutput(); catErr != nil {
									log.Printf("Error with cat/tee: %v, output: %s", catErr, string(catOut))
								} else {
									log.Printf("cat/tee command executed, output: %s", string(catOut))
									time.Sleep(100 * time.Millisecond)
									if content, err := os.ReadFile(hostsFile); err == nil {
										if strings.Contains(string(content), req.Hostname) {
											log.Printf("Successfully copied with cat/tee - hostname found in /etc/hosts")
											copySuccess = true
										} else {
											log.Printf("cat/tee executed but hostname not found in /etc/hosts")
											log.Printf("Current /etc/hosts content:\n%s", string(content))
										}
									}
								}
							}

							if !copySuccess {
								log.Printf("Trying sh -c method with direct redirection")
								writeCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat %s > %s", tmpFile, hostsFile))
								writeCmd.Env = append(os.Environ(), "SUDO_ASKPASS=/bin/false")
								if writeOut, writeErr := writeCmd.CombinedOutput(); writeErr != nil {
									log.Printf("Error with sh -c: %v, output: %s", writeErr, string(writeOut))
								} else {
									log.Printf("sh -c command executed, output: %s", string(writeOut))
									time.Sleep(100 * time.Millisecond)
									if content, err := os.ReadFile(hostsFile); err == nil {
										if strings.Contains(string(content), req.Hostname) {
											log.Printf("Successfully copied with sh -c - hostname found in /etc/hosts")
											copySuccess = true
										} else {
											log.Printf("sh -c executed but hostname not found in /etc/hosts")
											log.Printf("Current /etc/hosts content:\n%s", string(content))
										}
									}
								}
							}

							if copySuccess {
								chmodPath := "/bin/chmod"
								if _, err := os.Stat(chmodPath); os.IsNotExist(err) {
									chmodPath = "/usr/bin/chmod"
								}
								chmodCmd := fmt.Sprintf("sudo %s 644 %s", chmodPath, hostsFile)
								if out, err := executeCommand(chmodCmd); err != nil {
									log.Printf("Warning: Could not set permissions: %v, output: %s", err, out)
								} else {
									log.Printf("Permissions set correctly")
								}

								if content, err := os.ReadFile(hostsFile); err == nil {
									if strings.Contains(string(content), req.Hostname) {
										log.Printf("Final verification: hostname %s successfully updated in /etc/hosts", req.Hostname)
									} else {
										log.Printf("Final verification failed: hostname not found")
										log.Printf("Final /etc/hosts content:\n%s", string(content))
									}
								}
							} else {
								log.Printf("Error: All copy methods failed. /etc/hosts was not updated.")
							}
						}

						os.Remove(tmpFile)
					}

					applied = append(applied, fmt.Sprintf("Hostname set to %s and /etc/hosts updated", req.Hostname))
				} else {
					errorMsg := fmt.Sprintf("Failed to set hostname: tried hostnamectl, /etc/hostname, and hostname command")
					if lastError != nil {
						errorMsg += fmt.Sprintf(" (last error: %v", lastError)
						if lastOutput != "" {
							errorMsg += fmt.Sprintf(", output: %s", strings.TrimSpace(lastOutput))
						}
						errorMsg += ")"
					}
					errors = append(errors, errorMsg)
					// Log de debug interno - no traducir
					if appConfig.Server.Debug {
						log.Printf("All hostname setting methods failed for hostname: %s. Last error: %v, Last output: %s", req.Hostname, lastError, lastOutput)
					}
				}
			}
		}
	}

	dnsServers := []string{}
	if req.DNS1 != "" {
		ipRegex := regexp.MustCompile(`^([0-9]{1,3}\.){3}[0-9]{1,3}$`)
		if !ipRegex.MatchString(req.DNS1) {
			errors = append(errors, "Invalid DNS1 format")
		} else {
			dnsServers = append(dnsServers, req.DNS1)
		}
	}

	if req.DNS2 != "" {
		ipRegex := regexp.MustCompile(`^([0-9]{1,3}\.){3}[0-9]{1,3}$`)
		if !ipRegex.MatchString(req.DNS2) {
			errors = append(errors, "Invalid DNS2 format")
		} else {
			dnsServers = append(dnsServers, req.DNS2)
		}
	}

	if len(dnsServers) > 0 {
		dnsApplied := false
		dnsStr := strings.Join(dnsServers, " ")

		if !dnsApplied {
			connCmd := exec.Command("sh", "-c", "nmcli -t -f NAME connection show --active 2>/dev/null | head -1")
			if connOut, err := connCmd.Output(); err == nil {
				connName := strings.TrimSpace(string(connOut))
				if connName != "" {
					cmd := fmt.Sprintf("sudo nmcli connection modify '%s' ipv4.dns '%s' 2>&1", connName, dnsStr)
					if out, err := executeCommand(cmd); err == nil {
						applyCmd := fmt.Sprintf("sudo nmcli connection up '%s' 2>&1", connName)
						executeCommand(applyCmd) // Ignorar errores de apply
						applied = append(applied, fmt.Sprintf("DNS set to %s (via NetworkManager)", strings.Join(dnsServers, ", ")))
						dnsApplied = true
						log.Printf("DNS configured via nmcli: %s", dnsStr)
					} else {
						log.Printf("nmcli DNS configuration failed: %v, output: %s", err, out)
					}
				}
			}
		}

		if !dnsApplied {
			resolvedConf := "/etc/systemd/resolved.conf"
			if _, err := os.Stat(resolvedConf); err == nil {
				content, err := os.ReadFile(resolvedConf)
				if err == nil {
					lines := strings.Split(string(content), "\n")
					updated := false
					newLines := []string{}

					for _, line := range lines {
						trimmed := strings.TrimSpace(line)
						if strings.HasPrefix(trimmed, "DNS=") {
							newLines = append(newLines, "DNS="+strings.Join(dnsServers, " "))
							updated = true
						} else if strings.HasPrefix(trimmed, "#DNS=") && !updated {
							newLines = append(newLines, "DNS="+strings.Join(dnsServers, " "))
							updated = true
						} else {
							newLines = append(newLines, line)
						}
					}

					if !updated {
						newLines = append(newLines, "DNS="+strings.Join(dnsServers, " "))
					}

					newContent := strings.Join(newLines, "\n")
					writeCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", resolvedConf))
					writeCmd.Stdin = strings.NewReader(newContent)
					if err := writeCmd.Run(); err == nil {
						executeCommand("sudo systemctl restart systemd-resolved 2>&1")
						applied = append(applied, fmt.Sprintf("DNS set to %s (via systemd-resolved)", strings.Join(dnsServers, ", ")))
						dnsApplied = true
						log.Printf("DNS configured via systemd-resolved: %s", dnsStr)
					}
				}
			}
		}

		if !dnsApplied {
			resolvConf := "/etc/resolv.conf"
			executeCommand(fmt.Sprintf("sudo cp %s %s.backup 2>/dev/null || true", resolvConf, resolvConf))

			content, err := os.ReadFile(resolvConf)
			if err == nil {
				lines := strings.Split(string(content), "\n")
				newLines := []string{}

				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if !strings.HasPrefix(trimmed, "nameserver") {
						newLines = append(newLines, line)
					}
				}

				for _, dns := range dnsServers {
					newLines = append(newLines, "nameserver "+dns)
				}

				newContent := strings.Join(newLines, "\n")
				writeCmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", resolvConf))
				writeCmd.Stdin = strings.NewReader(newContent)
				if err := writeCmd.Run(); err == nil {
					applied = append(applied, fmt.Sprintf("DNS set to %s (via /etc/resolv.conf)", strings.Join(dnsServers, ", ")))
					dnsApplied = true
					log.Printf("DNS configured via /etc/resolv.conf: %s", dnsStr)
				} else {
					log.Printf("Failed to write /etc/resolv.conf: %v", err)
				}
			}
		}

		if !dnsApplied {
			errors = append(errors, fmt.Sprintf("Failed to set DNS: tried NetworkManager, systemd-resolved, and /etc/resolv.conf"))
		}
	}

	if req.Gateway != "" {
		ipRegex := regexp.MustCompile(`^([0-9]{1,3}\.){3}[0-9]{1,3}$`)
		if !ipRegex.MatchString(req.Gateway) {
			errors = append(errors, "Invalid Gateway format")
		} else {
			gatewayApplied := false

			if !gatewayApplied {
				connCmd := exec.Command("sh", "-c", "nmcli -t -f NAME connection show --active 2>/dev/null | head -1")
				if connOut, err := connCmd.Output(); err == nil {
					connName := strings.TrimSpace(string(connOut))
					if connName != "" {
						cmd := fmt.Sprintf("sudo nmcli connection modify '%s' ipv4.gateway %s 2>&1", connName, req.Gateway)
						if out, err := executeCommand(cmd); err == nil {
							applyCmd := fmt.Sprintf("sudo nmcli connection up '%s' 2>&1", connName)
							if _, err := executeCommand(applyCmd); err == nil {
								applied = append(applied, fmt.Sprintf("Gateway set to %s (via NetworkManager)", req.Gateway))
								gatewayApplied = true
								log.Printf("Gateway configured via nmcli: %s", req.Gateway)
							} else {
								log.Printf("Failed to apply gateway via nmcli: %v", err)
							}
						} else {
							log.Printf("nmcli gateway configuration failed: %v, output: %s", err, out)
						}
					}
				}
			}

			if !gatewayApplied {
				ifaceCmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $5}' | head -1")
				iface := ""
				if ifaceOut, err := ifaceCmd.Output(); err == nil {
					iface = strings.TrimSpace(string(ifaceOut))
				}

				if iface != "" {
					executeCommand("sudo ip route del default 2>/dev/null || true")
					cmd := fmt.Sprintf("sudo ip route add default via %s dev %s 2>&1", req.Gateway, iface)
					if out, err := executeCommand(cmd); err == nil {
						applied = append(applied, fmt.Sprintf("Gateway set to %s (via ip route)", req.Gateway))
						gatewayApplied = true
						log.Printf("Gateway configured via ip route: %s on %s", req.Gateway, iface)
					} else {
						log.Printf("ip route gateway configuration failed: %v, output: %s", err, out)
					}
				} else {
					executeCommand("sudo ip route del default 2>/dev/null || true")
					cmd := fmt.Sprintf("sudo ip route add default via %s 2>&1", req.Gateway)
					if out, err := executeCommand(cmd); err == nil {
						applied = append(applied, fmt.Sprintf("Gateway set to %s (via ip route)", req.Gateway))
						gatewayApplied = true
						log.Printf("Gateway configured via ip route: %s", req.Gateway)
					} else {
						log.Printf("ip route gateway configuration failed: %v, output: %s", err, out)
					}
				}
			}

			if !gatewayApplied {
				ifaceCmd := exec.Command("sh", "-c", "route -n | grep '^0.0.0.0' | awk '{print $8}' | head -1")
				iface := ""
				if ifaceOut, err := ifaceCmd.Output(); err == nil {
					iface = strings.TrimSpace(string(ifaceOut))
				}

				if iface != "" {
					executeCommand("sudo route del default 2>/dev/null || true")
					cmd := fmt.Sprintf("sudo route add default gw %s %s 2>&1", req.Gateway, iface)
					if out, err := executeCommand(cmd); err == nil {
						applied = append(applied, fmt.Sprintf("Gateway set to %s (via route)", req.Gateway))
						gatewayApplied = true
						log.Printf("Gateway configured via route: %s on %s", req.Gateway, iface)
					} else {
						log.Printf("route gateway configuration failed: %v, output: %s", err, out)
					}
				}
			}

			if !gatewayApplied {
				errors = append(errors, fmt.Sprintf("Failed to set gateway: tried NetworkManager, ip route, and route command"))
			}
		}
	}

	if len(errors) > 0 {
		errorMsg := strings.Join(errors, "; ")
		if len(applied) > 0 {
			errorMsg += " (Some settings were applied: " + strings.Join(applied, ", ") + ")"
		}
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   errorMsg,
		})
	}

	message := "Configuration applied successfully"
	if len(applied) > 0 {
		message = strings.Join(applied, "; ")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": message,
	})
}

func wifiNetworksHandler(c *fiber.Ctx) error {
	interfaceName := c.Query("interface", DefaultWiFiInterface)
	result := scanWiFiNetworks(interfaceName)
	if networks, ok := result["networks"]; ok {
		return c.JSON(networks)
	}
	return c.JSON([]fiber.Map{})
}

func wifiClientsHandler(c *fiber.Ctx) error {
	return c.JSON([]fiber.Map{})
}

func wifiToggleHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	interfaceName := c.Query("interface", DefaultWiFiInterface)
	rfkillCheck := exec.Command("sh", "-c", "sudo rfkill list wifi 2>/dev/null | grep -i 'soft blocked'")
	rfkillOut, _ := rfkillCheck.Output()
	isBlocked := strings.Contains(strings.ToLower(string(rfkillOut)), "yes")

	result := toggleWiFi(interfaceName, isBlocked)

	if success, ok := result["success"].(bool); ok && success {
		InsertLog("INFO", fmt.Sprintf("WiFi toggle exitoso (usuario: %s)", user.Username), "wifi", &userID)
		return c.JSON(result)
	}

	if errorMsg, ok := result["error"].(string); ok && errorMsg != "" {
		InsertLog("ERROR", fmt.Sprintf("Error en WiFi toggle (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
	}

	rfkillOut, rfkillErr := execCommand("rfkill list wifi 2>/dev/null | grep -i 'wifi' | head -1").CombinedOutput()
	if rfkillErr == nil && strings.Contains(strings.ToLower(string(rfkillOut)), "wifi") {
		statusOut, _ := execCommand("rfkill list wifi 2>/dev/null | grep -i 'soft blocked'").CombinedOutput()
		isBlocked := strings.Contains(strings.ToLower(string(statusOut)), "yes")

		var rfkillCmd string
		var wasEnabled bool
		if isBlocked {
			rfkillCmd = "rfkill unblock wifi"
			wasEnabled = false
		} else {
			rfkillCmd = "rfkill block wifi"
			wasEnabled = true
		}

		_, rfkillToggleErr := execCommand(rfkillCmd + " 2>/dev/null").CombinedOutput()
		if rfkillToggleErr == nil {
			if !wasEnabled {
				time.Sleep(1 * time.Second)

				ifaceCmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl' | head -1")
				ifaceOut, ifaceErr := ifaceCmd.Output()
				if ifaceErr == nil {
					iface := strings.TrimSpace(string(ifaceOut))
					if iface != "" {
						execCommand(fmt.Sprintf("ip link set %s up 2>/dev/null", iface)).Run()
						time.Sleep(1 * time.Second)
					}
				}
			}
			InsertLog("INFO", fmt.Sprintf("WiFi toggle exitoso usando rfkill con sudo (usuario: %s)", user.Username), "wifi", &userID)
			return c.JSON(fiber.Map{"success": true, "message": "WiFi toggle exitoso"})
		}
	}

	var iface string
	ipOut, ipErr := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl' | head -1").Output()
	if ipErr == nil {
		iface = strings.TrimSpace(string(ipOut))
	}

	if iface == "" {
		iwOut, iwErr := execCommand("iwconfig 2>/dev/null | grep -i 'wlan' | head -1 | awk '{print $1}'").CombinedOutput()
		if iwErr == nil {
			iface = strings.TrimSpace(string(iwOut))
		}
	}

	if iface != "" {
		statusOut, _ := exec.Command("sh", "-c", fmt.Sprintf("ip link show %s 2>/dev/null | grep -i 'state'", iface)).CombinedOutput()
		isDown := strings.Contains(strings.ToLower(string(statusOut)), "down") || strings.Contains(strings.ToLower(string(statusOut)), "disabled")

		if isDown {
			execCommand("rfkill unblock wifi 2>/dev/null").Run()
			execCommand(fmt.Sprintf("ip link set %s up 2>/dev/null", iface)).Run()
			execCommand(fmt.Sprintf("ifconfig %s up 2>/dev/null", iface)).Run()
			time.Sleep(1 * time.Second)
			InsertLog("INFO", fmt.Sprintf("WiFi activado usando ifconfig/ip en interfaz %s (usuario: %s)", iface, user.Username), "wifi", &userID)
			return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("WiFi activado en interfaz %s", iface)})
		} else {
			iwCmd := fmt.Sprintf("ifconfig %s down", iface)
			execCommand(iwCmd + " 2>/dev/null").Run()
			InsertLog("INFO", fmt.Sprintf("WiFi desactivado usando ifconfig en interfaz %s (usuario: %s)", iface, user.Username), "wifi", &userID)
			return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("WiFi desactivado en interfaz %s", iface)})
		}
	}

	errorMsg := "No se pudo cambiar el estado de WiFi. Verifica que tengas permisos sudo configurados (NOPASSWD) o que rfkill/ip estén disponibles. Para configurar sudo sin contraseña, ejecuta: sudo visudo y agrega: usuario ALL=(ALL) NOPASSWD: /usr/sbin/rfkill, /sbin/ip, /sbin/ifconfig"
	InsertLog("ERROR", fmt.Sprintf("Error en WiFi toggle (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
	return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
}

func wifiUnblockHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	success := false
	method := ""
	var lastError error

	rfkillCheck := exec.Command("sh", "-c", "command -v rfkill 2>/dev/null")
	if rfkillCheck.Run() == nil {
		rfkillOut, rfkillErr := execCommand("rfkill list wifi 2>/dev/null | grep -i 'wifi' | head -1").CombinedOutput()
		if rfkillErr == nil && strings.Contains(strings.ToLower(string(rfkillOut)), "wifi") {
			rfkillCmd := "rfkill unblock wifi"
			rfkillOutSudo, rfkillUnblockErr := execCommand(rfkillCmd + " 2>&1").CombinedOutput()
			if rfkillUnblockErr == nil {
				success = true
				method = "rfkill (con sudo)"
			} else {
				lastError = fmt.Errorf("rfkill error: %s", string(rfkillOutSudo))
			}
		}
	}

	if !success {
		nmcliCheck := exec.Command("sh", "-c", "command -v nmcli 2>/dev/null")
		if nmcliCheck.Run() == nil {
			nmcliCmd := "nmcli radio wifi on"
			nmcliOut, nmcliErr := execCommand(nmcliCmd + " 2>&1").CombinedOutput()
			if nmcliErr == nil {
				success = true
				method = "nmcli (con sudo)"
			} else {
				if lastError == nil {
					lastError = fmt.Errorf("nmcli error: %s", string(nmcliOut))
				}
			}
		}
	}

	if success && method == "rfkill (con sudo)" {
		nmcliCheck := exec.Command("sh", "-c", "command -v nmcli 2>/dev/null")
		if nmcliCheck.Run() == nil {
			execCommand("nmcli radio wifi on 2>/dev/null").Run()
		}
	}

	if success {
		time.Sleep(1 * time.Second)

		InsertLog("INFO", fmt.Sprintf("WiFi desbloqueado exitosamente usando %s (usuario: %s)", method, user.Username), "wifi", &userID)
		return c.JSON(fiber.Map{"success": true, "message": "WiFi desbloqueado exitosamente"})
	}

	errorDetails := "No se pudo desbloquear WiFi."
	if lastError != nil {
		errorDetails += fmt.Sprintf(" Último error: %v", lastError)
	}

	availableCmds := []string{}
	if exec.Command("sh", "-c", "command -v rfkill 2>/dev/null").Run() == nil {
		availableCmds = append(availableCmds, "rfkill")
	}
	if exec.Command("sh", "-c", "command -v nmcli 2>/dev/null").Run() == nil {
		availableCmds = append(availableCmds, "nmcli")
	}

	if len(availableCmds) == 0 {
		errorDetails += " No se encontraron comandos rfkill ni nmcli instalados."
	} else {
		errorDetails += fmt.Sprintf(" Comandos disponibles: %s. Verifica permisos sudo (NOPASSWD) ejecutando: sudo fix_wifi_permissions.sh", strings.Join(availableCmds, ", "))
	}

	InsertLog("ERROR", fmt.Sprintf("Error desbloqueando WiFi (usuario: %s): %s", user.Username, errorDetails), "wifi", &userID)
	return c.Status(500).JSON(fiber.Map{"error": errorDetails})
}

func wifiSoftwareSwitchHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	rfkillCheck := exec.Command("sh", "-c", "command -v rfkill 2>/dev/null")
	if rfkillCheck.Run() != nil {
		errorMsg := "rfkill no está disponible en el sistema"
		InsertLog("ERROR", fmt.Sprintf("Error en software switch (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
	}

	statusOut, _ := execCommand("rfkill list wifi 2>/dev/null | grep -i 'soft blocked'").CombinedOutput()
	statusStr := strings.ToLower(string(statusOut))
	isBlocked := strings.Contains(statusStr, "yes")

	var cmd string
	var action string
	if isBlocked {
		cmd = "rfkill unblock wifi"
		action = "desbloqueado"
	} else {
		cmd = "rfkill block wifi"
		action = "bloqueado"
	}

	output, err := execCommand(cmd + " 2>&1").CombinedOutput()
	if err != nil {
		errorMsg := fmt.Sprintf("Error ejecutando rfkill: %s", string(output))
		InsertLog("ERROR", fmt.Sprintf("Error en software switch (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
	}

	time.Sleep(1 * time.Second)

	newStatusOut, _ := execCommand("rfkill list wifi 2>/dev/null | grep -i 'soft blocked'").CombinedOutput()
	newStatusStr := strings.ToLower(string(newStatusOut))
	newIsBlocked := strings.Contains(newStatusStr, "yes")

	if isBlocked == newIsBlocked {
		errorMsg := "El switch de software no cambió de estado"
		InsertLog("WARN", fmt.Sprintf("Switch de software no cambió (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
	}

	message := fmt.Sprintf("Switch de software %s exitosamente", action)
	InsertLog("INFO", fmt.Sprintf("Switch de software %s (usuario: %s)", action, user.Username), "wifi", &userID)
	return c.JSON(fiber.Map{
		"success": true,
		"message": message,
		"blocked": newIsBlocked,
	})
}

func wifiConfigHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
		Security string `json:"security"`
		Region   string `json:"region"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}

	if req.Region != "" {
		if len(req.Region) != 2 {
			return c.Status(400).JSON(fiber.Map{"error": "Código de región inválido. Debe ser de 2 letras (ej: US, ES, GB)"})
		}

		req.Region = strings.ToUpper(req.Region)

		iwCheck := exec.Command("sh", "-c", "command -v iw 2>/dev/null")
		if iwCheck.Run() == nil {
			cmd := exec.Command("sh", "-c", fmt.Sprintf("sudo iw reg set %s 2>&1", req.Region))
			out, err := cmd.CombinedOutput()
			output := strings.TrimSpace(string(out))

			if err == nil {
				verifyCmd := exec.Command("sh", "-c", "iw reg get 2>&1")
				verifyOut, _ := verifyCmd.CombinedOutput()
				verifyOutput := strings.TrimSpace(string(verifyOut))

				if strings.Contains(verifyOutput, req.Region) || output == "" {
					InsertLog("INFO", fmt.Sprintf("Región WiFi cambiada a %s usando iw (usuario: %s)", req.Region, user.Username), "wifi", &userID)
					return c.JSON(fiber.Map{"success": true, "message": "Región WiFi cambiada exitosamente a " + req.Region})
				}
			}

			crdaCmd := exec.Command("sh", "-c", fmt.Sprintf("echo 'REGDOMAIN=%s' | sudo tee /etc/default/crda >/dev/null 2>&1", req.Region))
			if crdaCmd.Run() == nil {
				InsertLog("INFO", fmt.Sprintf("Región WiFi configurada a %s en crda (usuario: %s)", req.Region, user.Username), "wifi", &userID)
				exec.Command("sh", "-c", "sudo nmcli radio wifi off 2>/dev/null").Run()
				time.Sleep(1 * time.Second)
				exec.Command("sh", "-c", "sudo nmcli radio wifi on 2>/dev/null").Run()
				return c.JSON(fiber.Map{"success": true, "message": "Región WiFi configurada exitosamente. WiFi reiniciado para aplicar cambios."})
			}

			regdomCmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | sudo tee /etc/conf.d/wireless-regdom >/dev/null 2>&1", req.Region))
			if regdomCmd.Run() == nil {
				InsertLog("INFO", fmt.Sprintf("Región WiFi configurada a %s en wireless-regdom (usuario: %s)", req.Region, user.Username), "wifi", &userID)
				return c.JSON(fiber.Map{"success": true, "message": "Región WiFi configurada. Reinicia WiFi o el sistema para aplicar cambios."})
			}
		}

		crdaCmd2 := exec.Command("sh", "-c", fmt.Sprintf("echo 'REGDOMAIN=%s' | sudo tee /etc/default/crda >/dev/null 2>&1", req.Region))
		if crdaCmd2.Run() == nil {
			InsertLog("INFO", fmt.Sprintf("Región WiFi configurada a %s (usuario: %s)", req.Region, user.Username), "wifi", &userID)
			return c.JSON(fiber.Map{"success": true, "message": "Región WiFi configurada. Reinicia WiFi para aplicar cambios."})
		}

		errorMsg := fmt.Sprintf("No se pudo cambiar la región WiFi automáticamente. Verifica que 'iw' esté instalado (sudo apt-get install iw) y que tengas permisos sudo configurados. Puedes configurarlo manualmente ejecutando: sudo iw reg set %s", req.Region)
		InsertLog("ERROR", fmt.Sprintf("Error cambiando región WiFi a %s (usuario: %s): %s", req.Region, user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"error": errorMsg})
	}

	if req.SSID != "" {
		c.Request().Header.SetContentType(fiber.MIMEApplicationJSON)
		body, _ := json.Marshal(fiber.Map{"ssid": req.SSID, "password": req.Password})
		c.Request().SetBody(body)
		return wifiConnectHandler(c)
	}

	return c.Status(400).JSON(fiber.Map{"error": "Se requiere ssid o region"})
}

func vpnConnectionsHandler(c *fiber.Ctx) error {
	result := getVPNStatus()

	var conns []fiber.Map
	if ov, ok := result["openvpn"].(map[string]interface{}); ok {
		status := fmt.Sprintf("%v", ov["status"])
		conns = append(conns, fiber.Map{"name": "openvpn", "type": "openvpn", "status": mapActiveStatus(status), "bandwidth": "-"})
	}
	if wg, ok := result["wireguard"].(map[string]interface{}); ok {
		active := fmt.Sprintf("%v", wg["active"])
		conns = append(conns, fiber.Map{"name": "wireguard", "type": "wireguard", "status": mapBoolStatus(active), "bandwidth": "-"})
	}
	return c.JSON(conns)
}

func vpnServersHandler(c *fiber.Ctx) error { return c.JSON([]fiber.Map{}) }
func vpnClientsHandler(c *fiber.Ctx) error { return c.JSON([]fiber.Map{}) }
func vpnToggleHandler(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{"error": "VPN toggle no implementado"})
}
func vpnGetConfigHandler(c *fiber.Ctx) error {
	result := getOpenVPNConfig()
	return c.JSON(result)
}

func vpnConfigHandler(c *fiber.Ctx) error {
	var req struct {
		Config string `json:"config"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Datos inválidos"})
	}
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	result := saveOpenVPNConfig(req.Config, user.Username)
	if success, ok := result["success"].(bool); ok && success {
		return c.JSON(result)
	}
	if errorMsg, ok := result["error"].(string); ok {
		return c.Status(400).JSON(fiber.Map{"error": errorMsg})
	}
	return c.Status(500).JSON(fiber.Map{"error": "Error desconocido"})
}
func vpnConnectionToggleHandler(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{"error": "VPN connection toggle no implementado"})
}
func vpnCertificatesGenerateHandler(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{"error": "VPN certificates no implementado"})
}

func hostapdAccessPointsHandler(c *fiber.Ctx) error {
	var aps []fiber.Map

	hostapdActive := false
	hostapdTransmitting := false // Verificar si realmente está transmitiendo

	systemctlOut, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null").CombinedOutput()
	systemctlStatus := strings.TrimSpace(string(systemctlOut))
	if systemctlStatus == "active" {
		hostapdActive = true
	}

	if !hostapdActive {
		pgrepOut, _ := exec.Command("sh", "-c", "pgrep hostapd > /dev/null 2>&1 && echo active || echo inactive").CombinedOutput()
		pgrepStatus := strings.TrimSpace(string(pgrepOut))
		if pgrepStatus == "active" {
			hostapdActive = true
		}
	}

	if hostapdActive {
		interfaceName := "ap0" // default para modo AP+STA
		if configContent, err := os.ReadFile("/etc/hostapd/hostapd.conf"); err == nil {
			lines := strings.Split(string(configContent), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "interface=") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						interfaceName = strings.TrimSpace(parts[1])
						break
					}
				}
			}
		}

		iwOut, _ := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s info 2>/dev/null | grep -i 'type AP' || iwconfig %s 2>/dev/null | grep -i 'mode:master' || echo ''", interfaceName, interfaceName)).CombinedOutput()
		iwStatus := strings.TrimSpace(string(iwOut))
		if iwStatus != "" {
			hostapdTransmitting = true
		}

		if !hostapdTransmitting {
			cliStatusOut, _ := exec.Command("sh", "-c", fmt.Sprintf("hostapd_cli -i %s status 2>/dev/null | grep -i 'state=ENABLED' || echo ''", interfaceName)).CombinedOutput()
			cliStatus := strings.TrimSpace(string(cliStatusOut))
			if cliStatus != "" {
				hostapdTransmitting = true
			}
		}

		if !hostapdTransmitting {
			journalOut, _ := exec.Command("sh", "-c", "sudo journalctl -u hostapd -n 30 --no-pager 2>/dev/null | tail -20").CombinedOutput()
			journalLogs := strings.ToLower(string(journalOut))
			if strings.Contains(journalLogs, "could not configure driver") ||
				strings.Contains(journalLogs, "nl80211: could not") ||
				strings.Contains(journalLogs, "interface") && strings.Contains(journalLogs, "not found") ||
				strings.Contains(journalLogs, "failed to initialize") {
				hostapdTransmitting = false
			}
		}
	}

	configPath := "/etc/hostapd/hostapd.conf"
	config := make(map[string]string)

	if configContent, err := os.ReadFile(configPath); err == nil {
		lines := strings.Split(string(configContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				config[key] = value
			}
		}
	}

	if hostapdActive || len(config) > 0 {
		ssid := config["ssid"]
		if ssid == "" {
			ssid = "hostberry" // Valor por defecto (red + portal cautivo)
		}

		interfaceName := config["interface"]
		if interfaceName == "" {
			interfaceName = DefaultWiFiInterface
		}

		channel := config["channel"]
		if channel == "" {
			channel = "6" // Valor por defecto
		}

		security := "WPA2"
		if config["auth_algs"] == "0" {
			security = "Open"
		} else if strings.Contains(config["wpa_key_mgmt"], "SHA256") {
			security = "WPA3"
		} else if config["wpa"] == "2" {
			security = "WPA2"
		}

		clientsCount := 0
		if hostapdActive {
			cliOut, err := exec.Command("sh", "-c", fmt.Sprintf("hostapd_cli -i %s all_sta 2>/dev/null | grep -c '^sta=' || echo 0", interfaceName)).CombinedOutput()
			if err == nil {
				if count, err := strconvAtoiSafe(strings.TrimSpace(string(cliOut))); err == nil {
					clientsCount = count
				}
			}
		}

		actuallyActive := hostapdActive && hostapdTransmitting

		aps = append(aps, fiber.Map{
			"name":      interfaceName,
			"ssid":      ssid,
			"interface": interfaceName,
			"channel":   channel,
			"security":  security,
			"enabled":   actuallyActive, // Solo true si realmente está transmitiendo
			"active":    actuallyActive, // Solo true si realmente está transmitiendo
			"status": func() string {
				if actuallyActive {
					return "active"
				} else if hostapdActive {
					return "error" // Servicio corriendo pero no transmite
				}
				return "inactive"
			}(),
			"transmitting":    hostapdTransmitting, // Nuevo campo para diagnóstico
			"service_running": hostapdActive,       // Servicio corriendo (pero puede no transmitir)
			"clients_count":   clientsCount,
		})
	}

	return c.JSON(aps)
}

func hostapdClientsHandler(c *fiber.Ctx) error {
	var clients []fiber.Map

	hostapdOut, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null || pgrep hostapd > /dev/null && echo active || echo inactive").CombinedOutput()
	hostapdStatus := strings.TrimSpace(string(hostapdOut))

	if hostapdStatus == "active" {
		cliOut, err := exec.Command("hostapd_cli", "-i", "wlan0", "all_sta").CombinedOutput()
		if err == nil && len(cliOut) > 0 {
			lines := strings.Split(strings.TrimSpace(string(cliOut)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && strings.HasPrefix(line, "sta=") {
					mac := strings.TrimPrefix(line, "sta=")
					clients = append(clients, fiber.Map{
						"mac_address": mac,
						"ip_address":  "-",
						"signal":      "-",
						"uptime":      "-",
					})
				}
			}
		}
	}

	return c.JSON(clients)
}

func hostapdCreateAp0Handler(c *fiber.Ctx) error {
	phyInterface := "wlan0"

	interfacesResp, _ := executeCommand("ip link show | grep -E '^[0-9]+: wlan' | awk -F: '{print $2}' | awk '{print $1}' | head -1")
	if strings.TrimSpace(interfacesResp) != "" {
		phyInterface = strings.TrimSpace(interfacesResp)
	}

	log.Printf("Creating ap0 interface from %s", phyInterface)

	ap0CheckCmd := "ip link show ap0 2>/dev/null"
	ap0Exists := false
	if out, err := executeCommand(ap0CheckCmd); err == nil && strings.TrimSpace(out) != "" {
		ap0Exists = true
		log.Printf("Interface ap0 already exists")
		return c.JSON(fiber.Map{
			"success": true,
			"message": "Interface ap0 already exists",
			"exists":  true,
		})
	}

	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", phyInterface))
	time.Sleep(500 * time.Millisecond)

	phyName := ""
	phyCmd2 := fmt.Sprintf("cat /sys/class/net/%s/phy80211/name 2>/dev/null", phyInterface)
	phyOut2, _ := executeCommand(phyCmd2)
	phyName = strings.TrimSpace(phyOut2)

	if phyName == "" {
		phyCmd := fmt.Sprintf("iw dev %s info 2>/dev/null | grep 'wiphy' | awk '{print $2}'", phyInterface)
		phyOut, _ := executeCommand(phyCmd)
		phyName = strings.TrimSpace(phyOut)
	}

	if phyName == "" {
		phyCmd3 := "iw list 2>/dev/null | grep 'Wiphy' | head -1 | awk '{print $2}'"
		phyOut3, _ := executeCommand(phyCmd3)
		phyName = strings.TrimSpace(phyOut3)
	}

	if phyName == "" {
		phyName = "phy0"
		log.Printf("Warning: Could not detect phy name, using default: %s", phyName)
	}

	log.Printf("Detected phy name: %s for interface %s", phyName, phyInterface)

	delOut, _ := executeCommand("sudo iw dev ap0 del 2>/dev/null || true")
	if delOut != "" {
		log.Printf("Removed existing ap0 interface (if it existed): %s", strings.TrimSpace(delOut))
	}
	time.Sleep(1 * time.Second)

	createApCmd := fmt.Sprintf("sudo iw phy %s interface add ap0 type __ap 2>&1", phyName)
	log.Printf("Executing: %s", createApCmd)
	createOut, createErr := executeCommand(createApCmd)
	if createOut != "" {
		log.Printf("Command output: %s", strings.TrimSpace(createOut))
	}

	if createErr != nil {
		log.Printf("Error creating ap0 with phy %s: %s", phyName, strings.TrimSpace(createOut))
		log.Printf("Trying alternative method: using interface %s directly...", phyInterface)

		createApCmd2 := fmt.Sprintf("sudo iw dev %s interface add ap0 type __ap 2>&1", phyInterface)
		log.Printf("Executing: %s", createApCmd2)
		createOut2, createErr2 := executeCommand(createApCmd2)
		if createOut2 != "" {
			log.Printf("Method 1 output: %s", strings.TrimSpace(createOut2))
		}

		if createErr2 != nil {
			log.Printf("Error with alternative method: %s", strings.TrimSpace(createOut2))
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"error":   fmt.Sprintf("Failed to create ap0 interface: %s", strings.TrimSpace(createOut2)),
			})
		} else {
			log.Printf("Successfully created ap0 interface using alternative method (from %s)", phyInterface)
			ap0Exists = true
		}
	} else {
		log.Printf("Successfully created ap0 interface using phy %s", phyName)
		ap0Exists = true
	}

	if ap0Exists {
		time.Sleep(2 * time.Second)

		verified := false
		for i := 0; i < 5; i++ {
			verifyCmd := "ip link show ap0 2>/dev/null"
			verifyOut, verifyErr := executeCommand(verifyCmd)
			if verifyErr == nil && strings.TrimSpace(verifyOut) != "" {
				log.Printf("Interface ap0 verified successfully (attempt %d)", i+1)
				verified = true
				break
			}

			if i < 4 {
				log.Printf("Verification attempt %d failed, retrying...", i+1)
				time.Sleep(1 * time.Second)
			}
		}

		if !verified {
			log.Printf("ERROR: Interface ap0 was NOT created successfully after all attempts")
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"error":   "Failed to verify ap0 interface creation. Please check WiFi hardware and drivers.",
			})
		} else {
			log.Printf("SUCCESS: Interface ap0 created and verified")
			executeCommand("sudo ip link set ap0 up 2>/dev/null || true")
			return c.JSON(fiber.Map{
				"success": true,
				"message": "Interface ap0 created successfully",
				"exists":  true,
			})
		}
	}

	return c.Status(500).JSON(fiber.Map{
		"success": false,
		"error":   "Failed to create ap0 interface",
	})
}

func hostapdToggleHandler(c *fiber.Ctx) error {
	log.Printf("HostAPD toggle request received")

	hostapdOut, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null || pgrep hostapd > /dev/null && echo active || echo inactive").CombinedOutput()
	hostapdStatus := strings.TrimSpace(string(hostapdOut))
	isActive := hostapdStatus == "active"

	log.Printf("Current HostAPD status: %s (isActive: %v)", hostapdStatus, isActive)

	var cmdStr string
	var enableCmd string
	var action string

	if isActive {
		action = "disable"
		executeCommand("sudo systemctl stop dnsmasq 2>/dev/null || true")
		cmdStr = "sudo systemctl stop hostapd"
		enableCmd = "sudo systemctl disable hostapd 2>/dev/null || true"

	} else {
		action = "enable"

		configPath := "/etc/hostapd/hostapd.conf"
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			log.Printf("HostAPD configuration file not found: %s", configPath)
			return c.Status(400).JSON(fiber.Map{
				"error":          "HostAPD configuration not found. Please configure HostAPD first using the configuration form below.",
				"success":        false,
				"config_missing": true,
				"config_path":    configPath,
			})
		}

		configContent, err := os.ReadFile(configPath)
		if err != nil || len(configContent) == 0 {
			log.Printf("HostAPD configuration file is empty or unreadable: %s", configPath)
			return c.Status(400).JSON(fiber.Map{
				"error":          "HostAPD configuration file is empty or invalid. Please configure HostAPD first using the configuration form below.",
				"success":        false,
				"config_missing": true,
				"config_path":    configPath,
			})
		}

		ap0CheckCmd := "ip link show ap0 2>/dev/null"
		ap0Exists := false
		if out, err := executeCommand(ap0CheckCmd); err == nil && strings.TrimSpace(out) != "" {
			ap0Exists = true
			log.Printf("Interface ap0 already exists")
		} else {
			log.Printf("Interface ap0 does not exist, creating it...")
			phyInterface := "wlan0"
			if configContent, err := os.ReadFile(configPath); err == nil {
				lines := strings.Split(string(configContent), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "interface=") {
						break
					}
				}
			}

			executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", phyInterface))
			time.Sleep(500 * time.Millisecond)

			phyName := ""

			phyCmd := fmt.Sprintf("iw dev %s info 2>/dev/null | grep 'wiphy' | awk '{print $2}'", phyInterface)
			phyOut, _ := executeCommand(phyCmd)
			phyName = strings.TrimSpace(phyOut)

			if phyName == "" {
				phyCmd2 := fmt.Sprintf("cat /sys/class/net/%s/phy80211/name 2>/dev/null", phyInterface)
				phyOut2, _ := executeCommand(phyCmd2)
				phyName = strings.TrimSpace(phyOut2)
			}

			if phyName == "" {
				phyCmd3 := "iw list 2>/dev/null | grep -A 1 'Wiphy' | tail -1 | awk '{print $2}'"
				phyOut3, _ := executeCommand(phyCmd3)
				phyName = strings.TrimSpace(phyOut3)
			}

			if phyName == "" {
				phyName = "phy0"
				log.Printf("Warning: Could not detect phy name, using default: %s", phyName)
			}

			log.Printf("Creating ap0 interface using phy %s from interface %s...", phyName, phyInterface)

			executeCommand("sudo iw dev ap0 del 2>/dev/null || true")
			time.Sleep(1 * time.Second)

			createApCmd := fmt.Sprintf("sudo iw phy %s interface add ap0 type __ap", phyName)
			createOut, createErr := executeCommand(createApCmd)
			if createErr != nil {
				log.Printf("Warning: Could not create ap0 interface with phy %s: %s", phyName, strings.TrimSpace(createOut))
				createApCmd2 := fmt.Sprintf("sudo iw dev %s interface add ap0 type __ap", phyInterface)
				createOut2, createErr2 := executeCommand(createApCmd2)
				if createErr2 != nil {
					log.Printf("Warning: Alternative method also failed: %s", strings.TrimSpace(createOut2))
				} else {
					log.Printf("Successfully created ap0 interface using alternative method (from %s)", phyInterface)
					ap0Exists = true
				}
			} else {
				log.Printf("Successfully created ap0 interface using phy %s", phyName)
				ap0Exists = true
			}

			if ap0Exists {
				verifyCmd := "ip link show ap0 2>/dev/null"
				verifyOut, verifyErr := executeCommand(verifyCmd)
				if verifyErr == nil && strings.TrimSpace(verifyOut) != "" {
					log.Printf("Interface ap0 verified: %s", strings.TrimSpace(verifyOut))
					executeCommand("sudo ip link set ap0 up 2>/dev/null || true")
					log.Printf("Activated ap0 interface")
				} else {
					log.Printf("Warning: ap0 was created but verification failed")
				}
			}
		}

		maskedCheck, _ := exec.Command("sh", "-c", "systemctl is-enabled hostapd 2>&1").CombinedOutput()
		maskedStatus := strings.TrimSpace(string(maskedCheck))
		if strings.Contains(maskedStatus, "masked") {
			log.Printf("HostAPD service is masked, unmasking...")
			executeCommand("sudo systemctl unmask hostapd 2>/dev/null || true")
		}

		configLines := strings.Split(string(configContent), "\n")
		var interfaceName, gatewayIP string
		for _, line := range configLines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "interface=") {
				interfaceName = strings.TrimPrefix(line, "interface=")
			}
		}

		if interfaceName == "" {
			log.Printf("HostAPD configuration file missing interface setting: %s", configPath)
			return c.Status(400).JSON(fiber.Map{
				"error":          "HostAPD configuration file is missing required 'interface' setting. Please configure HostAPD first using the configuration form below.",
				"success":        false,
				"config_missing": true,
				"config_path":    configPath,
			})
		}

		if interfaceName != "" {
			gatewayIP = "192.168.4.1"

			ipCheckCmd := fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1", interfaceName)
			ipOut, _ := exec.Command("sh", "-c", ipCheckCmd).CombinedOutput()
			currentIP := strings.TrimSpace(string(ipOut))

			if currentIP == "" {
				log.Printf("Configuring IP %s on interface %s", gatewayIP, interfaceName)
				ipCmd := fmt.Sprintf("sudo ip addr add %s/24 dev %s 2>/dev/null || sudo ip addr replace %s/24 dev %s", gatewayIP, interfaceName, gatewayIP, interfaceName)
				if out, err := executeCommand(ipCmd); err != nil {
					log.Printf("Warning: Error setting IP on interface: %s", strings.TrimSpace(out))
				}

				if out, err := executeCommand(fmt.Sprintf("sudo ip link set %s up", interfaceName)); err != nil {
					log.Printf("Warning: Error bringing interface up: %s", strings.TrimSpace(out))
				}
			}
		}

		executeCommand("sudo systemctl unmask hostapd 2>/dev/null || true")
		executeCommand("sudo systemctl unmask dnsmasq 2>/dev/null || true")

		executeCommand("sudo systemctl daemon-reload 2>/dev/null || true")

		enableCmd = "sudo systemctl enable hostapd 2>/dev/null || true"
		executeCommand("sudo systemctl enable dnsmasq 2>/dev/null || true")

		executeCommand(fmt.Sprintf("sudo chmod 644 %s 2>/dev/null || true", configPath))

		cmdStr = "sudo systemctl start hostapd"
		executeCommand("sudo systemctl start dnsmasq 2>/dev/null || true")
	}

	log.Printf("Action: %s, Command: %s", action, cmdStr)

	if enableCmd != "" {
		if out, err := executeCommand(enableCmd); err != nil {
			log.Printf("Warning: Error enabling/disabling hostapd: %s", strings.TrimSpace(out))
		} else {
			log.Printf("Enable/disable command executed successfully: %s", strings.TrimSpace(out))
		}
	}

	out, err := executeCommand(cmdStr)
	if err != nil {
		log.Printf("Error executing %s command: %s", action, strings.TrimSpace(out))

		var errorDetails string
		if action == "enable" {
			journalOut, _ := exec.Command("sh", "-c", "sudo journalctl -u hostapd -n 20 --no-pager 2>/dev/null | tail -10").CombinedOutput()
			journalLogs := strings.TrimSpace(string(journalOut))
			if journalLogs != "" {
				lines := strings.Split(journalLogs, "\n")
				errorLines := []string{}
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && (strings.Contains(strings.ToLower(line), "error") ||
						strings.Contains(strings.ToLower(line), "failed") ||
						strings.Contains(strings.ToLower(line), "fail")) {
						errorLines = append(errorLines, line)
					}
				}
				if len(errorLines) > 0 {
					errorDetails = fmt.Sprintf(" Recent errors: %s", strings.Join(errorLines, "; "))
				} else {
					errorDetails = fmt.Sprintf(" Last logs: %s", strings.Join(lines[len(lines)-3:], "; "))
				}
			} else {
				statusOut, _ := exec.Command("sh", "-c", "sudo systemctl status hostapd --no-pager 2>/dev/null | head -15").CombinedOutput()
				statusInfo := strings.TrimSpace(string(statusOut))
				if statusInfo != "" {
					errorDetails = fmt.Sprintf(" Service status: %s", statusInfo)
				}
			}
		}

		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Failed to %s hostapd: %s%s", action, strings.TrimSpace(out), errorDetails),
			"success": false,
		})
	}

	log.Printf("HostAPD %s command executed. Output: %s", action, strings.TrimSpace(out))

	if action == "enable" {
		time.Sleep(1500 * time.Millisecond) // Más tiempo para que hostapd inicie
	} else {
		time.Sleep(500 * time.Millisecond)
	}

	hostapdOut2, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null || pgrep hostapd > /dev/null && echo active || echo inactive").CombinedOutput()
	hostapdStatus2 := strings.TrimSpace(string(hostapdOut2))
	actuallyActive := hostapdStatus2 == "active"

	if action == "enable" && !actuallyActive {
		log.Printf("HostAPD failed to start. Checking logs...")
		enabledOut, _ := exec.Command("sh", "-c", "systemctl is-enabled hostapd 2>/dev/null || echo disabled").CombinedOutput()
		enabledStatus := strings.TrimSpace(string(enabledOut))

		journalOut, _ := exec.Command("sh", "-c", "sudo journalctl -u hostapd -n 15 --no-pager 2>/dev/null | tail -8").CombinedOutput()
		journalLogs := strings.TrimSpace(string(journalOut))

		statusOut, _ := exec.Command("sh", "-c", "sudo systemctl status hostapd --no-pager 2>/dev/null | head -20").CombinedOutput()
		statusInfo := strings.TrimSpace(string(statusOut))

		var errorMsg string
		if journalLogs != "" {
			lines := strings.Split(journalLogs, "\n")
			errorLines := []string{}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					lowerLine := strings.ToLower(line)
					if strings.Contains(lowerLine, "error") || strings.Contains(lowerLine, "failed") ||
						strings.Contains(lowerLine, "fail") || strings.Contains(lowerLine, "cannot") {
						errorLines = append(errorLines, line)
					}
				}
			}
			if len(errorLines) > 0 {
				maxLines := 3
				if len(errorLines) < maxLines {
					maxLines = len(errorLines)
				}
				errorMsg = strings.Join(errorLines[:maxLines], "; ")
			} else if len(lines) > 0 {
				maxLines := 3
				if len(lines) < maxLines {
					maxLines = len(lines)
				}
				errorMsg = strings.Join(lines[len(lines)-maxLines:], "; ")
			}
		}

		if errorMsg == "" && statusInfo != "" {
			statusLines := strings.Split(statusInfo, "\n")
			for _, line := range statusLines {
				if strings.Contains(strings.ToLower(line), "active:") ||
					strings.Contains(strings.ToLower(line), "failed") ||
					strings.Contains(strings.ToLower(line), "error") {
					errorMsg = strings.TrimSpace(line)
					break
				}
			}
		}

		if errorMsg != "" {
			return c.Status(500).JSON(fiber.Map{
				"error":   fmt.Sprintf("Failed to enable HostAPD. Service status: %s (enabled: %s). %s", hostapdStatus2, enabledStatus, errorMsg),
				"success": false,
				"status":  hostapdStatus2,
				"enabled": false,
			})
		} else {
			return c.Status(500).JSON(fiber.Map{
				"error":   fmt.Sprintf("Failed to enable HostAPD. Service status: %s (enabled: %s). Check configuration and logs.", hostapdStatus2, enabledStatus),
				"success": false,
				"status":  hostapdStatus2,
				"enabled": false,
			})
		}
	}

	log.Printf("HostAPD status after %s: %s (actuallyActive: %v)", action, hostapdStatus2, actuallyActive)

	return c.JSON(fiber.Map{
		"success": true,
		"output":  strings.TrimSpace(out),
		"enabled": actuallyActive,
		"action":  action,
		"status":  hostapdStatus2,
	})
}

func hostapdRestartHandler(c *fiber.Ctx) error {
	out1, err1 := executeCommand("sudo systemctl stop hostapd")

	time.Sleep(500 * time.Millisecond)

	out2, err2 := executeCommand("sudo systemctl start hostapd")

	if err1 != nil || err2 != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Error reiniciando HostAPD",
			"stop":    strings.TrimSpace(out1),
			"start":   strings.TrimSpace(out2),
			"stopOk":  err1 == nil,
			"startOk": err2 == nil,
			"success": false,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"output":  "HostAPD restarted successfully",
	})
}

func hostapdDiagnosticsHandler(c *fiber.Ctx) error {
	diagnostics := make(map[string]interface{})

	systemctlOut, _ := exec.Command("sh", "-c", "systemctl is-active hostapd 2>/dev/null").CombinedOutput()
	systemctlStatus := strings.TrimSpace(string(systemctlOut))
	pgrepOut, _ := exec.Command("sh", "-c", "pgrep hostapd > /dev/null 2>&1 && echo active || echo inactive").CombinedOutput()
	pgrepStatus := strings.TrimSpace(string(pgrepOut))

	serviceRunning := systemctlStatus == "active" || pgrepStatus == "active"
	diagnostics["service_running"] = serviceRunning
	diagnostics["systemctl_status"] = systemctlStatus
	diagnostics["process_running"] = pgrepStatus == "active"

	interfaceName := "wlan0"
	configPath := "/etc/hostapd/hostapd.conf"
	if configContent, err := os.ReadFile(configPath); err == nil {
		lines := strings.Split(string(configContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "interface=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					interfaceName = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}
	diagnostics["interface"] = interfaceName

	iwOut, _ := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s info 2>/dev/null | grep -i 'type AP' || iwconfig %s 2>/dev/null | grep -i 'mode:master' || echo ''", interfaceName, interfaceName)).CombinedOutput()
	iwStatus := strings.TrimSpace(string(iwOut))
	transmitting := iwStatus != ""

	if !transmitting && serviceRunning {
		cliStatusOut, _ := exec.Command("sh", "-c", fmt.Sprintf("hostapd_cli -i %s status 2>/dev/null | grep -i 'state=ENABLED' || echo ''", interfaceName)).CombinedOutput()
		cliStatus := strings.TrimSpace(string(cliStatusOut))
		if cliStatus != "" {
			transmitting = true
		}
	}

	diagnostics["transmitting"] = transmitting
	diagnostics["interface_in_ap_mode"] = iwStatus != ""

	journalOut, _ := exec.Command("sh", "-c", "sudo journalctl -u hostapd -n 50 --no-pager 2>/dev/null | tail -30").CombinedOutput()
	journalLogs := string(journalOut)
	diagnostics["recent_logs"] = journalLogs

	errors := []string{}
	journalLower := strings.ToLower(journalLogs)
	if strings.Contains(journalLower, "could not configure driver") {
		errors = append(errors, "Driver configuration error")
	}
	if strings.Contains(journalLower, "nl80211: could not") {
		errors = append(errors, "nl80211 driver error")
	}
	if strings.Contains(journalLower, "interface") && strings.Contains(journalLower, "not found") {
		errors = append(errors, "Interface not found")
	}
	if strings.Contains(journalLower, "failed to initialize") {
		errors = append(errors, "Initialization failed")
	}
	if strings.Contains(journalLower, "could not set channel") {
		errors = append(errors, "Channel configuration error")
	}

	diagnostics["errors"] = errors
	diagnostics["has_errors"] = len(errors) > 0

	ipOut, _ := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep -i 'state UP' || echo ''", interfaceName)).CombinedOutput()
	interfaceUp := strings.Contains(strings.ToLower(string(ipOut)), "state up")
	diagnostics["interface_up"] = interfaceUp

	dnsmasqOut, _ := exec.Command("sh", "-c", "systemctl is-active dnsmasq 2>/dev/null || echo inactive").CombinedOutput()
	dnsmasqStatus := strings.TrimSpace(string(dnsmasqOut))
	diagnostics["dnsmasq_running"] = dnsmasqStatus == "active"

	diagnostics["status"] = func() string {
		if !serviceRunning {
			return "service_stopped"
		}
		if !transmitting {
			return "service_running_not_transmitting"
		}
		return "ok"
	}()

	return c.JSON(diagnostics)
}

func hostapdGetConfigHandler(c *fiber.Ctx) error {
	configPath := "/etc/hostapd/hostapd.conf"

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return c.JSON(fiber.Map{
			"success": false,
			"error":   "Configuration file not found",
			"config":  nil,
		})
	}

	configContent, err := os.ReadFile(configPath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("Error reading config file: %v", err),
			"config":  nil,
		})
	}

	config := make(map[string]string)
	lines := strings.Split(string(configContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			config[key] = value
		}
	}

	interfaceForDisplay := config["interface"]
	if interfaceForDisplay == "ap0" {
		interfaceForDisplay = "wlan0"
	}

	result := fiber.Map{
		"success": true,
		"config": fiber.Map{
			"interface": interfaceForDisplay, // Mostrar interfaz física al usuario
			"ssid":      config["ssid"],
			"channel":   config["channel"],
			"password":  config["wpa_passphrase"], // Devolver la contraseña para que el usuario pueda verla/editarla
		},
	}

	if config["auth_algs"] == "0" {
		result["config"].(fiber.Map)["security"] = "open"
	} else if strings.Contains(config["wpa_key_mgmt"], "SHA256") {
		result["config"].(fiber.Map)["security"] = "wpa3"
	} else if config["wpa"] == "2" {
		result["config"].(fiber.Map)["security"] = "wpa2"
	} else {
		result["config"].(fiber.Map)["security"] = "wpa2" // Por defecto
	}

	dnsmasqPath := "/etc/dnsmasq.conf"
	if dnsmasqContent, err := os.ReadFile(dnsmasqPath); err == nil {
		dnsmasqLines := strings.Split(string(dnsmasqContent), "\n")
		for _, line := range dnsmasqLines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "dhcp-option=3,") {
				gateway := strings.TrimPrefix(line, "dhcp-option=3,")
				result["config"].(fiber.Map)["gateway"] = gateway
			} else if strings.HasPrefix(line, "dhcp-range=") {
				rangeStr := strings.TrimPrefix(line, "dhcp-range=")
				parts := strings.Split(rangeStr, ",")
				if len(parts) >= 2 {
					result["config"].(fiber.Map)["dhcp_range_start"] = parts[0]
					result["config"].(fiber.Map)["dhcp_range_end"] = parts[1]
					if len(parts) >= 4 {
						result["config"].(fiber.Map)["lease_time"] = parts[3]
					}
				}
			}
		}
	}

	configMap := result["config"].(fiber.Map)
	if configMap["gateway"] == nil || configMap["gateway"] == "" {
		configMap["gateway"] = "192.168.4.1"
	}
	if configMap["dhcp_range_start"] == nil || configMap["dhcp_range_start"] == "" {
		configMap["dhcp_range_start"] = "192.168.4.2"
	}
	if configMap["dhcp_range_end"] == nil || configMap["dhcp_range_end"] == "" {
		configMap["dhcp_range_end"] = "192.168.4.254"
	}
	if configMap["lease_time"] == nil || configMap["lease_time"] == "" {
		configMap["lease_time"] = "12h"
	}
	if configMap["channel"] == nil || configMap["channel"] == "" {
		configMap["channel"] = "6"
	}

	countryCode := config["country_code"]
	if countryCode == "" {
		countryCode = config["country"] // Algunas configuraciones usan "country" en lugar de "country_code"
	}
	if countryCode == "" {
		countryCode = DefaultCountryCode
	}
	configMap["country"] = countryCode

	return c.JSON(result)
}

func hostapdConfigHandler(c *fiber.Ctx) error {
	var req struct {
		Interface      string `json:"interface"`
		SSID           string `json:"ssid"`
		Password       string `json:"password"`
		Channel        int    `json:"channel"`
		Security       string `json:"security"`
		Gateway        string `json:"gateway"`
		DHCPRangeStart string `json:"dhcp_range_start"`
		DHCPRangeEnd   string `json:"dhcp_range_end"`
		LeaseTime      string `json:"lease_time"`
		Country        string `json:"country"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request body",
			"success": false,
		})
	}

	if req.Interface == "" || req.SSID == "" || req.Channel < 1 || req.Channel > 13 {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Missing required fields: interface, ssid, channel",
			"success": false,
		})
	}

	if req.Gateway == "" {
		req.Gateway = "192.168.4.1"
	}
	if req.DHCPRangeStart == "" {
		req.DHCPRangeStart = "192.168.4.2"
	}
	if req.DHCPRangeEnd == "" {
		req.DHCPRangeEnd = "192.168.4.254"
	}
	if req.LeaseTime == "" {
		req.LeaseTime = "12h"
	}
	if req.Country == "" {
		req.Country = DefaultCountryCode
	}

	if len(req.Country) != 2 {
		req.Country = "US"
	}
	req.Country = strings.ToUpper(req.Country)

	if req.Security != "wpa2" && req.Security != "wpa3" && req.Security != "open" {
		req.Security = "wpa2"
	}

	apInterface := "ap0"
	phyInterface := req.Interface // wlan0 o la interfaz física

	log.Printf("Configuring AP+STA mode: creating virtual interface %s from %s", apInterface, phyInterface)

	executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", phyInterface))
	time.Sleep(500 * time.Millisecond)

	phyName := ""
	phyCmd2 := fmt.Sprintf("cat /sys/class/net/%s/phy80211/name 2>/dev/null", phyInterface)
	phyOut2, _ := executeCommand(phyCmd2)
	phyName = strings.TrimSpace(phyOut2)

	if phyName == "" {
		phyCmd := fmt.Sprintf("iw dev %s info 2>/dev/null | grep 'wiphy' | awk '{print $2}'", phyInterface)
		phyOut, _ := executeCommand(phyCmd)
		phyName = strings.TrimSpace(phyOut)
	}

	if phyName == "" {
		phyCmd3 := fmt.Sprintf("iw phy | grep -B 5 '%s' | grep 'Wiphy' | awk '{print $2}' | head -1", phyInterface)
		phyOut3, _ := executeCommand(phyCmd3)
		phyName = strings.TrimSpace(phyOut3)
	}

	if phyName == "" {
		phyCmd4 := "iw list 2>/dev/null | grep 'Wiphy' | head -1 | awk '{print $2}'"
		phyOut4, _ := executeCommand(phyCmd4)
		phyName = strings.TrimSpace(phyOut4)
	}

	if phyName == "" {
		if strings.HasPrefix(phyInterface, "wlan") {
			if numStr := strings.TrimPrefix(phyInterface, "wlan"); numStr != "" {
				phyName = "phy" + numStr
				log.Printf("Trying phy name based on interface number: %s", phyName)
			}
		}
	}

	if phyName == "" {
		phyName = "phy0"
		log.Printf("Warning: Could not detect phy name, using default: %s", phyName)
	}

	log.Printf("Detected phy name: %s for interface %s", phyName, phyInterface)

	macAddress := ""
	macCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/address 2>/dev/null", phyInterface))
	if macOut, err := macCmd.Output(); err == nil {
		macAddress = strings.TrimSpace(string(macOut))
	}
	if macAddress == "" {
		log.Printf("Warning: Could not get MAC address for %s", phyInterface)
		macAddress = "00:00:00:00:00:00" // Valor por defecto
	}

	log.Printf("Using phy: %s (MAC: %s) for virtual interface creation from %s", phyName, macAddress, phyInterface)

	if apInterface == "ap0" {
		log.Printf("Creating udev rule for automatic ap0 interface creation (TheWalrus method - Raspberry Pi 3 B+)")
		udevRulePath := "/etc/udev/rules.d/70-persistent-net.rules"

		checkCmd := exec.Command("sh", "-c", fmt.Sprintf("grep -q 'ap0' %s 2>/dev/null && echo 'exists' || echo 'not_exists'", udevRulePath))
		checkOut, _ := checkCmd.Output()
		if strings.TrimSpace(string(checkOut)) != "exists" {
			udevRuleContent := fmt.Sprintf(`# Regla para crear interfaz virtual ap0 automáticamente (método TheWalrus - Raspberry Pi 3 B+)
SUBSYSTEM=="ieee80211", ACTION=="add|change", ATTR{macaddress}=="%s", KERNEL=="%s", \
RUN+="/sbin/iw phy %s interface add ap0 type __ap", \
RUN+="/bin/ip link set ap0 address %s"
`, macAddress, phyName, phyName, macAddress)

			tmpUdevFile := "/tmp/70-persistent-net.rules.tmp"
			if err := os.WriteFile(tmpUdevFile, []byte(udevRuleContent), 0644); err == nil {
				if _, err := os.Stat(udevRulePath); err == nil {
					existingContent, _ := os.ReadFile(udevRulePath)
					combinedContent := string(existingContent) + "\n" + udevRuleContent
					os.WriteFile(tmpUdevFile, []byte(combinedContent), 0644)
				}
				executeCommand(fmt.Sprintf("sudo cp %s %s && sudo chmod 644 %s", tmpUdevFile, udevRulePath, udevRulePath))
				os.Remove(tmpUdevFile)
				log.Printf("Created udev rule for automatic ap0 creation (TheWalrus method - Raspberry Pi 3 B+)")
				executeCommand("sudo udevadm control --reload-rules 2>/dev/null || true")
				executeCommand("sudo udevadm trigger 2>/dev/null || true")
			} else {
				log.Printf("Warning: Could not create udev rule: %v", err)
			}
		} else {
			log.Printf("udev rule for ap0 already exists")
		}
	}

	checkApCmd := fmt.Sprintf("ip link show %s 2>/dev/null", apInterface)
	apExists := false
	checkOut, checkErr := executeCommand(checkApCmd)
	if checkErr == nil && strings.TrimSpace(checkOut) != "" {
		apExists = true
		log.Printf("Interface %s already exists, reusing it", apInterface)
		executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", apInterface))
	}

	if !apExists {
		log.Printf("Interface %s does not exist, creating it...", apInterface)

		delOut, delErr := executeCommand(fmt.Sprintf("sudo iw dev %s del 2>/dev/null || true", apInterface))
		if delErr == nil {
			log.Printf("Removed existing %s interface (if it existed): %s", apInterface, strings.TrimSpace(delOut))
		}
		time.Sleep(1 * time.Second)

		log.Printf("Creating virtual interface %s using phy %s...", apInterface, phyName)

		phyExistsCmd := fmt.Sprintf("iw phy %s info 2>/dev/null", phyName)
		phyExistsOut, phyExistsErr := executeCommand(phyExistsCmd)
		if phyExistsErr != nil || strings.TrimSpace(phyExistsOut) == "" {
			log.Printf("Warning: phy %s may not exist, output: %s", phyName, strings.TrimSpace(phyExistsOut))
		} else {
			log.Printf("phy %s exists and is accessible", phyName)
		}

		phyCheckCmd := fmt.Sprintf("iw phy %s info 2>/dev/null | grep -i 'AP'", phyName)
		phyCheckOut, _ := executeCommand(phyCheckCmd)
		if strings.TrimSpace(phyCheckOut) == "" {
			log.Printf("Warning: phy %s may not support AP mode, but attempting anyway", phyName)
		} else {
			log.Printf("phy %s supports AP mode: %s", phyName, strings.TrimSpace(phyCheckOut))
		}

		iwInfoCmd := fmt.Sprintf("iw dev %s info 2>/dev/null", phyInterface)
		iwInfoOut, _ := executeCommand(iwInfoCmd)
		if strings.Contains(iwInfoOut, "type AP") {
			log.Printf("Warning: Physical interface %s is in AP mode, changing to managed first", phyInterface)
			executeCommand(fmt.Sprintf("sudo iw dev %s set type managed 2>/dev/null", phyInterface))
			time.Sleep(1 * time.Second)
		}

		executeCommand(fmt.Sprintf("sudo ip link set %s up 2>/dev/null || true", phyInterface))
		time.Sleep(500 * time.Millisecond)

		createApCmd := fmt.Sprintf("sudo iw phy %s interface add %s type __ap 2>&1", phyName, apInterface)
		log.Printf("Executing: %s", createApCmd)
		createOut, createErr := executeCommand(createApCmd)
		if createOut != "" {
			log.Printf("Command output: %s", strings.TrimSpace(createOut))
		}

		if createErr != nil {
			log.Printf("Error creating virtual interface %s with phy %s: %s", apInterface, phyName, strings.TrimSpace(createOut))
			log.Printf("Error details: %v", createErr)
			log.Printf("Trying alternative method 1: using interface %s directly...", phyInterface)

			createApCmd2 := fmt.Sprintf("sudo iw dev %s interface add %s type __ap 2>&1", phyInterface, apInterface)
			log.Printf("Executing: %s", createApCmd2)
			createOut2, createErr2 := executeCommand(createApCmd2)
			if createOut2 != "" {
				log.Printf("Method 1 output: %s", strings.TrimSpace(createOut2))
			}

			if createErr2 != nil {
				log.Printf("Error with alternative method 1: %s", strings.TrimSpace(createOut2))
				log.Printf("Trying alternative method 2: using iw phy without sudo...")

				createApCmd3 := fmt.Sprintf("iw phy %s interface add %s type __ap 2>&1", phyName, apInterface)
				log.Printf("Executing: %s", createApCmd3)
				createOut3, createErr3 := executeCommand(createApCmd3)
				if createOut3 != "" {
					log.Printf("Method 2 output: %s", strings.TrimSpace(createOut3))
				}

				if createErr3 != nil {
					log.Printf("Error with alternative method 2: %s", strings.TrimSpace(createOut3))
					log.Printf("Trying alternative method 3: using mac80211_hwsim if available...")

					phyListCmd := "iw phy 2>/dev/null | grep 'Wiphy' | awk '{print $2}'"
					phyListOut, _ := executeCommand(phyListCmd)
					log.Printf("Available phys: %s", strings.TrimSpace(phyListOut))
					altPhyName := strings.TrimSpace(phyListOut)
					if altPhyName != "" && altPhyName != phyName {
						phyLines := strings.Split(altPhyName, "\n")
						if len(phyLines) > 0 {
							altPhyName = strings.TrimSpace(phyLines[0])
						}
						if altPhyName != "" && altPhyName != phyName {
							log.Printf("Trying with alternative phy: %s", altPhyName)
							createApCmd4 := fmt.Sprintf("sudo iw phy %s interface add %s type __ap 2>&1", altPhyName, apInterface)
							log.Printf("Executing: %s", createApCmd4)
							createOut4, createErr4 := executeCommand(createApCmd4)
							if createOut4 != "" {
								log.Printf("Method 3 output: %s", strings.TrimSpace(createOut4))
							}
							if createErr4 == nil {
								log.Printf("Successfully created interface %s using alternative phy %s", apInterface, altPhyName)
								apExists = true
								phyName = altPhyName // Actualizar phyName para uso posterior
							} else {
								log.Printf("Error with alternative phy: %s", strings.TrimSpace(createOut4))
								apInterface = phyInterface
								log.Printf("Falling back to using physical interface %s directly (non-concurrent mode)", apInterface)
							}
						} else {
							apInterface = phyInterface
							log.Printf("Falling back to using physical interface %s directly (non-concurrent mode)", apInterface)
						}
					} else {
						apInterface = phyInterface
						log.Printf("Falling back to using physical interface %s directly (non-concurrent mode)", apInterface)
					}
				} else {
					log.Printf("Successfully created interface %s using method 2 (without sudo)", apInterface)
					apExists = true
				}
			} else {
				log.Printf("Successfully created interface %s using alternative method 1 (from %s)", apInterface, phyInterface)
				apExists = true
			}
		} else {
			log.Printf("Successfully created interface %s using phy %s", apInterface, phyName)
			apExists = true
		}

		if apExists && apInterface == "ap0" {
			time.Sleep(2 * time.Second)

			verified := false
			for i := 0; i < 5; i++ {
				verifyCmd := fmt.Sprintf("ip link show %s 2>/dev/null", apInterface)
				verifyOut, verifyErr := executeCommand(verifyCmd)
				if verifyErr == nil && strings.TrimSpace(verifyOut) != "" {
					log.Printf("Interface %s verified successfully with ip link (attempt %d)", apInterface, i+1)
					verified = true
					break
				}

				lsCmd := fmt.Sprintf("ls /sys/class/net/ 2>/dev/null | grep -q '^%s$' && echo 'exists'", apInterface)
				lsOut, _ := executeCommand(lsCmd)
				if strings.TrimSpace(lsOut) == "exists" {
					log.Printf("Interface %s verified successfully with ls /sys/class/net (attempt %d)", apInterface, i+1)
					verified = true
					break
				}

				iwListCmd := fmt.Sprintf("iw dev 2>/dev/null | grep -q 'Interface %s' && echo 'exists'", apInterface)
				iwListOut, _ := executeCommand(iwListCmd)
				if strings.TrimSpace(iwListOut) == "exists" {
					log.Printf("Interface %s verified successfully with iw dev (attempt %d)", apInterface, i+1)
					verified = true
					break
				}

				if i < 4 {
					log.Printf("Verification attempt %d failed, retrying...", i+1)
					time.Sleep(1 * time.Second)
				}
			}

			if !verified {
				log.Printf("ERROR: Interface %s was NOT created successfully after all attempts", apInterface)
				log.Printf("Diagnostics:")
				log.Printf("  - phy name: %s", phyName)
				log.Printf("  - physical interface: %s", phyInterface)
				log.Printf("  - MAC address: %s", macAddress)

				log.Printf("Attempting manual creation as last resort...")
				manualCmd := fmt.Sprintf("sudo iw phy %s interface add %s type __ap 2>&1; sleep 1; ip link show %s 2>&1", phyName, apInterface, apInterface)
				manualOut, _ := executeCommand(manualCmd)
				log.Printf("Manual creation result: %s", strings.TrimSpace(manualOut))
			} else {
				log.Printf("SUCCESS: Interface %s created and verified", apInterface)
			}
		}
	}

	ipCmd := fmt.Sprintf("sudo ip addr add %s/24 dev %s 2>/dev/null || sudo ip addr replace %s/24 dev %s", req.Gateway, apInterface, req.Gateway, apInterface)
	if out, err := executeCommand(ipCmd); err != nil {
		log.Printf("Warning: Error setting IP on interface %s: %s", apInterface, strings.TrimSpace(out))
	}

	if out, err := executeCommand(fmt.Sprintf("sudo ip link set %s up", apInterface)); err != nil {
		log.Printf("Warning: Error bringing interface %s up: %s", apInterface, strings.TrimSpace(out))
	} else {
		log.Printf("Successfully created and activated virtual interface %s", apInterface)
		checkCmd := fmt.Sprintf("ip link show %s", apInterface)
		if checkOut, checkErr := executeCommand(checkCmd); checkErr == nil {
			log.Printf("Interface %s verified: %s", apInterface, strings.TrimSpace(checkOut))
		}
	}

	configPath := "/etc/hostapd/hostapd.conf"

	executeCommand("sudo mkdir -p /etc/hostapd 2>/dev/null || true")

	configContent := fmt.Sprintf(`interface=%s
driver=nl80211
ssid=%s
hw_mode=g
channel=%d
country_code=%s
`, apInterface, req.SSID, req.Channel, req.Country)

	if req.Security == "open" {
		configContent += "auth_algs=0\n"
	} else if req.Security == "wpa2" {
		if req.Password == "" {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Password required for WPA2/WPA3",
				"success": false,
			})
		}
		configContent += fmt.Sprintf(`wpa=2
wpa_passphrase=%s
wpa_key_mgmt=WPA-PSK
wpa_pairwise=TKIP
rsn_pairwise=CCMP
`, req.Password)
	} else if req.Security == "wpa3" {
		if req.Password == "" {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Password required for WPA2/WPA3",
				"success": false,
			})
		}
		configContent += fmt.Sprintf(`wpa=2
wpa_passphrase=%s
wpa_key_mgmt=WPA-PSK-SHA256
wpa_pairwise=CCMP
rsn_pairwise=CCMP
`, req.Password)
	}

	tmpFile := "/tmp/hostapd.conf.tmp"
	log.Printf("Creating temporary config file: %s", tmpFile)
	if err := os.WriteFile(tmpFile, []byte(configContent), 0644); err != nil {
		log.Printf("Error creating temporary config file: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Error creating temporary config file: %v", err),
			"success": false,
		})
	}

	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		log.Printf("Temporary file was not created: %s", tmpFile)
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to create temporary config file",
			"success": false,
		})
	}

	log.Printf("Temporary file created successfully, size: %d bytes", len(configContent))

	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		log.Printf("Temporary file does not exist before copy: %s", tmpFile)
		return c.Status(500).JSON(fiber.Map{
			"error":   "Temporary file was not created or was deleted",
			"success": false,
		})
	}

	if fileInfo, err := os.Stat(tmpFile); err == nil {
		log.Printf("Temporary file exists, size: %d bytes, mode: %v", fileInfo.Size(), fileInfo.Mode())
	} else {
		log.Printf("Cannot stat temporary file: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Cannot access temporary file: %v", err),
			"success": false,
		})
	}

	log.Printf("Ensuring /etc/hostapd directory exists...")
	if out, err := executeCommand("sudo mkdir -p /etc/hostapd"); err != nil {
		log.Printf("Warning: Error creating /etc/hostapd directory: %v, output: %s", err, out)
	}
	if out, err := executeCommand("sudo chmod 755 /etc/hostapd"); err != nil {
		log.Printf("Warning: Error setting permissions on /etc/hostapd: %v, output: %s", err, out)
	}

	if _, err := os.Stat("/etc/hostapd"); os.IsNotExist(err) {
		log.Printf("Error: /etc/hostapd directory does not exist after creation attempt")
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to create /etc/hostapd directory. Please run: sudo mkdir -p /etc/hostapd && sudo chmod 755 /etc/hostapd",
			"success": false,
		})
	}

	os.Chmod(tmpFile, 0644)

	cmdStr := fmt.Sprintf("sudo cp %s %s", tmpFile, configPath)
	log.Printf("Executing: %s", cmdStr)
	out, err := executeCommand(cmdStr)
	if err != nil {
		log.Printf("Error copying config file: %v, output: '%s'", err, out)
		os.Remove(tmpFile) // Limpiar archivo temporal
		errorMsg := strings.TrimSpace(out)
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Error saving hostapd configuration: %s. Please check sudo permissions for cp command.", errorMsg),
			"success": false,
		})
	}
	log.Printf("File copied successfully, output: '%s'", strings.TrimSpace(out))

	chmodCmd := fmt.Sprintf("sudo chmod 644 %s", configPath)
	log.Printf("Setting permissions: %s", chmodCmd)
	if out, err := executeCommand(chmodCmd); err != nil {
		log.Printf("Warning: Error setting permissions: %v, output: %s", err, strings.TrimSpace(out))
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		os.Remove(tmpFile)
		log.Printf("Config file was not created at: %s", configPath)
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Config file was not created at %s", configPath),
			"success": false,
		})
	}

	log.Printf("HostAPD config file created successfully at: %s", configPath)

	os.Remove(tmpFile)

	dnsmasqConfigPath := "/etc/dnsmasq.conf"
	executeCommand(fmt.Sprintf("sudo cp %s %s.backup 2>/dev/null || true", dnsmasqConfigPath, dnsmasqConfigPath))

	dnsmasqContent := fmt.Sprintf(`# Configuración de dnsmasq para modo AP+STA según método TheWalrus (Raspberry Pi 3 B+)
# Solo servir DHCP en ap0, no en wlan0 (que es STA)
interface=%s
no-dhcp-interface=%s
bind-interfaces
server=8.8.8.8
server=8.8.4.4
domain-needed
bogus-priv
dhcp-range=%s,%s,255.255.255.0,%s
dhcp-option=3,%s
dhcp-option=6,%s
`, apInterface, phyInterface, req.DHCPRangeStart, req.DHCPRangeEnd, req.LeaseTime, req.Gateway, req.Gateway)

	tmpDnsmasqFile := "/tmp/dnsmasq.conf.tmp"
	if err := os.WriteFile(tmpDnsmasqFile, []byte(dnsmasqContent), 0644); err != nil {
		log.Printf("Error creating temporary dnsmasq config file: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Error creating temporary dnsmasq config file: %v", err),
			"success": false,
		})
	}

	os.Chmod(tmpDnsmasqFile, 0644)
	cmdStr2 := fmt.Sprintf("sudo cp %s %s && sudo chmod 644 %s", tmpDnsmasqFile, dnsmasqConfigPath, dnsmasqConfigPath)
	if out, err := executeCommand(cmdStr2); err != nil {
		os.Remove(tmpDnsmasqFile) // Limpiar archivo temporal
		log.Printf("Error writing dnsmasq config file: %s, output: %s", err, strings.TrimSpace(out))
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Error saving dnsmasq configuration: %s", strings.TrimSpace(out)),
			"success": false,
		})
	}

	os.Remove(tmpDnsmasqFile)

	executeCommand("echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward > /dev/null")
	executeCommand("sudo sysctl -w net.ipv4.ip_forward=1 > /dev/null 2>&1")

	sysctlCheckCmd := "grep -q '^net.ipv4.ip_forward=1' /etc/sysctl.conf || echo 'net.ipv4.ip_forward=1' | sudo tee -a /etc/sysctl.conf > /dev/null"
	executeCommand(sysctlCheckCmd)

	mainInterface := "eth0"
	if out, _ := executeCommand("ip route | grep default | awk '{print $5}' | head -1"); strings.TrimSpace(out) != "" {
		mainInterface = strings.TrimSpace(out)
	}

	apIPBegin := req.Gateway
	if lastDot := strings.LastIndex(req.Gateway, "."); lastDot > 0 {
		apIPBegin = req.Gateway[:lastDot]
	}

	if mainInterface != "" && mainInterface != apInterface {
		executeCommand(fmt.Sprintf("sudo iptables -t nat -D POSTROUTING -s %s.0/24 ! -d %s.0/24 -j MASQUERADE 2>/dev/null || true", apIPBegin, apIPBegin))
		executeCommand(fmt.Sprintf("sudo iptables -t nat -D POSTROUTING -o %s -j MASQUERADE 2>/dev/null || true", mainInterface))
		executeCommand(fmt.Sprintf("sudo iptables -D FORWARD -i %s -o %s -j ACCEPT 2>/dev/null || true", apInterface, mainInterface))
		executeCommand(fmt.Sprintf("sudo iptables -D FORWARD -i %s -o %s -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true", mainInterface, apInterface))

		executeCommand(fmt.Sprintf("sudo iptables -t nat -A POSTROUTING -s %s.0/24 ! -d %s.0/24 -j MASQUERADE", apIPBegin, apIPBegin))
		executeCommand(fmt.Sprintf("sudo iptables -t nat -A POSTROUTING -o %s -j MASQUERADE", mainInterface))
		executeCommand(fmt.Sprintf("sudo iptables -A FORWARD -i %s -o %s -j ACCEPT", apInterface, mainInterface))
		executeCommand(fmt.Sprintf("sudo iptables -A FORWARD -i %s -o %s -m state --state RELATED,ESTABLISHED -j ACCEPT", mainInterface, apInterface))
	}

	overrideDir := "/etc/systemd/system/hostapd.service.d"
	executeCommand(fmt.Sprintf("sudo mkdir -p %s 2>/dev/null || true", overrideDir))
	overrideContent := fmt.Sprintf(`[Service]
ExecStart=
ExecStart=/usr/sbin/hostapd -B -P /run/hostapd.pid %s
PIDFile=/run/hostapd.pid
Type=forking
`, configPath)
	tmpOverrideFile := "/tmp/hostapd-override.conf.tmp"
	if err := os.WriteFile(tmpOverrideFile, []byte(overrideContent), 0644); err != nil {
		log.Printf("Warning: Error creating temporary override file: %v", err)
	} else {
		overridePath := fmt.Sprintf("%s/override.conf", overrideDir)
		os.Chmod(tmpOverrideFile, 0644)
		cmdStr3 := fmt.Sprintf("sudo cp %s %s && sudo chmod 644 %s", tmpOverrideFile, overridePath, overridePath)
		if out, err := executeCommand(cmdStr3); err != nil {
			log.Printf("Warning: Error writing override file: %s, output: %s", err, strings.TrimSpace(out))
		} else {
			log.Printf("Override file created successfully")
		}
		os.Remove(tmpOverrideFile)
	}

	manageAp0Script := `#!/bin/bash
# check if hostapd service success to start or not
# in our case, it cannot start when /var/run/hostapd/ap0 exist
# so we have to delete it
echo 'Check if hostapd.service is hang cause ap0 exist...'
hostapd_is_running=$(systemctl is-active hostapd 2>/dev/null | grep -c "active")
if test 1 -ne "${hostapd_is_running}"; then
    rm -rf /var/run/hostapd/ap0 || echo "ap0 interface does not exist, the failure is elsewhere"
    # También limpiar el PID file si existe
    rm -f /run/hostapd.pid || true
fi
# Asegurar que ap0 existe antes de iniciar hostapd
if ! ip link show ap0 > /dev/null 2>&1; then
    # Intentar crear ap0 si no existe
    phy=$(iw dev wlan0 info 2>/dev/null | grep wiphy | awk '{print $2}')
    if [ -n "$phy" ]; then
        iw phy $phy interface add ap0 type __ap 2>/dev/null || true
        sleep 1
    fi
fi
`
	manageAp0Path := "/bin/manage-ap0-iface.sh"
	tmpManageAp0 := "/tmp/manage-ap0-iface.sh.tmp"
	if err := os.WriteFile(tmpManageAp0, []byte(manageAp0Script), 0755); err == nil {
		executeCommand(fmt.Sprintf("sudo cp %s %s && sudo chmod +x %s", tmpManageAp0, manageAp0Path, manageAp0Path))
		os.Remove(tmpManageAp0)
		log.Printf("Created manage-ap0-iface.sh script")
	}

	overridePath := fmt.Sprintf("%s/override.conf", overrideDir)
	overrideContentWithPreStart := fmt.Sprintf(`[Service]
ExecStart=
ExecStartPre=/bin/manage-ap0-iface.sh
ExecStart=/usr/sbin/hostapd -B -P /run/hostapd.pid %s
PIDFile=/run/hostapd.pid
Type=forking
TimeoutStartSec=30
TimeoutStopSec=10
`, configPath)
	tmpOverrideFile2 := "/tmp/hostapd-override.conf.tmp"
	if err := os.WriteFile(tmpOverrideFile2, []byte(overrideContentWithPreStart), 0644); err == nil {
		cmdStr4 := fmt.Sprintf("sudo cp %s %s && sudo chmod 644 %s", tmpOverrideFile2, overridePath, overridePath)
		if out, err := executeCommand(cmdStr4); err != nil {
			log.Printf("Warning: Error updating override file with pre-start: %s, output: %s", err, strings.TrimSpace(out))
		} else {
			log.Printf("Override file updated with pre-start script and PID file configuration")
		}
		os.Remove(tmpOverrideFile2)
	}

	executeCommand("sudo systemctl daemon-reload")

	apIPBeginForScript := req.Gateway
	if lastDot := strings.LastIndex(req.Gateway, "."); lastDot > 0 {
		apIPBeginForScript = req.Gateway[:lastDot]
	}
	rpiWifiScript := fmt.Sprintf(`#!/bin/bash
echo 'Starting Wifi AP and STA client...'
/usr/sbin/ifdown --force %s 2>/dev/null || true
/usr/sbin/ifdown --force %s 2>/dev/null || true
/usr/sbin/ifup --force %s 2>/dev/null || true
/usr/sbin/ifup --force %s 2>/dev/null || true
/usr/sbin/sysctl -w net.ipv4.ip_forward=1 > /dev/null 2>&1
/usr/sbin/iptables -t nat -A POSTROUTING -s %s.0/24 ! -d %s.0/24 -j MASQUERADE 2>/dev/null || true
/usr/bin/systemctl restart dnsmasq 2>/dev/null || true
echo 'WPA Supplicant reconfigure in 5sec...'
/usr/bin/sleep 5
wpa_cli -i %s reconfigure 2>/dev/null || true
`, phyInterface, apInterface, apInterface, phyInterface, apIPBeginForScript, apIPBeginForScript, phyInterface)
	rpiWifiPath := "/bin/rpi-wifi.sh"
	tmpRpiWifi := "/tmp/rpi-wifi.sh.tmp"
	if err := os.WriteFile(tmpRpiWifi, []byte(rpiWifiScript), 0755); err == nil {
		executeCommand(fmt.Sprintf("sudo cp %s %s && sudo chmod +x %s", tmpRpiWifi, rpiWifiPath, rpiWifiPath))
		os.Remove(tmpRpiWifi)
		log.Printf("Created rpi-wifi.sh script")
	}

	executeCommand("sudo systemctl enable hostapd 2>/dev/null || true")
	executeCommand("sudo systemctl enable dnsmasq 2>/dev/null || true")

	if out, err := executeCommand("sudo systemctl restart dnsmasq"); err != nil {
		log.Printf("Warning: Error restarting dnsmasq: %s", strings.TrimSpace(out))
	}

	if apInterface == "ap0" {
		ap0CheckCmd := "ip link show ap0 2>/dev/null"
		ap0CheckOut, ap0CheckErr := executeCommand(ap0CheckCmd)
		if ap0CheckErr != nil || strings.TrimSpace(ap0CheckOut) == "" {
			log.Printf("Warning: ap0 interface does not exist, attempting to create it before starting hostapd")
			createAp0Cmd := fmt.Sprintf("sudo iw phy %s interface add ap0 type __ap 2>&1", phyName)
			createAp0Out, _ := executeCommand(createAp0Cmd)
			if createAp0Out != "" {
				log.Printf("ap0 creation attempt: %s", strings.TrimSpace(createAp0Out))
			}
			time.Sleep(1 * time.Second)
			ap0CheckOut2, _ := executeCommand(ap0CheckCmd)
			if strings.TrimSpace(ap0CheckOut2) == "" {
				log.Printf("ERROR: ap0 interface still does not exist after creation attempt")
				return c.Status(500).JSON(fiber.Map{
					"error":   "Failed to create ap0 interface. Please check WiFi hardware and drivers.",
					"success": false,
				})
			}
		}
	}

	executeCommand("sudo rm -rf /var/run/hostapd/ap0 2>/dev/null || true")
	executeCommand("sudo rm -f /run/hostapd.pid 2>/dev/null || true")

	if out, err := executeCommand("sudo systemctl restart hostapd"); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   fmt.Sprintf("Configuration saved but failed to restart hostapd: %s", strings.TrimSpace(out)),
			"success": false,
		})
	}

	time.Sleep(2 * time.Second)
	hostapdStatusCmd := "systemctl is-active hostapd 2>/dev/null"
	hostapdStatusOut, _ := executeCommand(hostapdStatusCmd)
	if strings.TrimSpace(hostapdStatusOut) != "active" {
		log.Printf("Warning: hostapd service may not be active after restart. Status: %s", strings.TrimSpace(hostapdStatusOut))
		pgrepOut, _ := executeCommand("pgrep hostapd 2>/dev/null && echo 'running' || echo 'not running'")
		log.Printf("hostapd process check: %s", strings.TrimSpace(pgrepOut))
	}

	// Asegurar que ap0 tenga la IP del gateway y reiniciar dnsmasq para que los clientes reciban IP por DHCP
	executeCommand("sudo ip addr add 192.168.4.1/24 dev ap0 2>/dev/null || true")
	if out, err := executeCommand("sudo systemctl restart dnsmasq 2>/dev/null"); err != nil {
		log.Printf("Warning: dnsmasq restart after hostapd config: %s", strings.TrimSpace(out))
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Configuration saved and services restarted",
	})
}

func helpContactHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID
	InsertLog("INFO", "Contacto/help recibido", "help", &userID)
	return c.JSON(fiber.Map{"success": true})
}

func translationsHandler(c *fiber.Ctx) error {
	lang := c.Params("lang", "en")
	if lang != "en" && lang != "es" {
		lang = "en"
	}
	path := filepath.Clean(filepath.Join("locales", lang+".json"))
	if !strings.HasPrefix(path, "locales"+string(filepath.Separator)) && path != "locales" {
		return c.Status(400).JSON(fiber.Map{"error": "idioma no permitido"})
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "JSON inválido en locales"})
	}
	return c.JSON(out)
}

func wifiStatusHandler(c *fiber.Ctx) error {
	return wifiLegacyStatusHandler(c)
}

func wifiLegacyStatusHandler(c *fiber.Ctx) error {
	var enabled bool = false
	var hardBlocked bool = false
	var softBlocked bool = false

	wifiCheck := execCommand("nmcli -t -f WIFI g 2>/dev/null")
	wifiOut, err := wifiCheck.Output()
	if err == nil {
		wifiState := strings.ToLower(strings.TrimSpace(filterSudoErrors(wifiOut)))
		if strings.Contains(wifiState, "enabled") || strings.Contains(wifiState, "on") {
			enabled = true
		} else if strings.Contains(wifiState, "disabled") || strings.Contains(wifiState, "off") {
			enabled = false
		}
	}

	rfkillOut, _ := execCommand("rfkill list wifi 2>/dev/null").CombinedOutput()
	rfkillStr := strings.ToLower(filterSudoErrors(rfkillOut))
	if strings.Contains(rfkillStr, "hard blocked: yes") {
		hardBlocked = true
		enabled = false
	} else if strings.Contains(rfkillStr, "soft blocked: yes") {
		softBlocked = true
		enabled = false
	} else {
		iwOut, _ := execCommand("iwconfig 2>/dev/null | grep -i 'wlan' | head -1").CombinedOutput()
		cleanIwOut := filterSudoErrors(iwOut)
		if len(cleanIwOut) > 0 {
			iwStatus, _ := execCommand("iwconfig 2>/dev/null | grep -i 'wlan' | head -1 | grep -i 'unassociated'").CombinedOutput()
			cleanIwStatus := filterSudoErrors(iwStatus)
			if len(cleanIwStatus) == 0 {
				enabled = true
			}
		} else {
			ipCheck := exec.Command("sh", "-c", "ip link show | grep -E '^[0-9]+: wlan' | grep -i 'state UP'")
			if ipOut, err := ipCheck.Output(); err == nil && len(ipOut) > 0 {
				enabled = true
			}
		}
	}

	ssid := ""
	connected := false
	iface := "wlan0"

	ipIfaceCmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl' | head -1")
	if ipIfaceOut, err := ipIfaceCmd.Output(); err == nil {
		if ipIfaceStr := strings.TrimSpace(string(ipIfaceOut)); ipIfaceStr != "" {
			iface = ipIfaceStr
		}
	}

	wpaStatusCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s status 2>/dev/null", iface))
	wpaStatusOut, wpaErr := wpaStatusCmd.CombinedOutput()
	if wpaErr == nil && len(wpaStatusOut) > 0 {
		wpaStatus := string(wpaStatusOut)
		for _, line := range strings.Split(wpaStatus, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ssid=") {
				ssid = strings.TrimPrefix(line, "ssid=")
				if ssid != "" {
					if strings.Contains(wpaStatus, "wpa_state=COMPLETED") {
						connected = true
					}
				}
				break
			}
		}
	}

	if !connected || ssid == "" {
		iwLinkCmd := exec.Command("sh", "-c", fmt.Sprintf("iw dev %s link 2>/dev/null", iface))
		iwLinkOut, iwErr := iwLinkCmd.CombinedOutput()
		if iwErr == nil && len(iwLinkOut) > 0 {
			iwLink := string(iwLinkOut)
			for _, line := range strings.Split(iwLink, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Connected to ") {
					connected = true
				} else if strings.Contains(line, "SSID:") {
					ssid = strings.TrimSpace(strings.TrimPrefix(line, "SSID:"))
					if ssid != "" {
						connected = true
					}
				}
			}
		}
	}

	if !connected || ssid == "" {
		iwOut, _ := execCommand("iwconfig 2>/dev/null | grep -i 'essid' | grep -v 'off/any' | head -1").CombinedOutput()
		iwStr := filterSudoErrors(iwOut)
		if strings.Contains(iwStr, "ESSID:") {
			parts := strings.Split(iwStr, "ESSID:")
			if len(parts) > 1 {
				ssid = strings.TrimSpace(strings.Trim(parts[1], "\""))
				if ssid != "" && ssid != "off/any" {
					connected = true
				}
			}
		}
	}

	reallyConnected := false
	if connected && ssid != "" {
		// Verificar si wpa_state es COMPLETED (autenticado)
		wpaStateCompleted := false
		if wpaErr == nil && len(wpaStatusOut) > 0 {
			wpaStatus := string(wpaStatusOut)
			if strings.Contains(wpaStatus, "wpa_state=COMPLETED") {
				wpaStateCompleted = true
			}
		}

		ipCheckCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", iface))
		ipOut, ipErr := ipCheckCmd.Output()
		if ipErr == nil {
			ip := strings.TrimSpace(string(ipOut))
			if ip != "" && ip != "N/A" && !strings.HasPrefix(ip, "169.254") {
				reallyConnected = true
				log.Printf("WiFi realmente conectado: SSID=%s, IP=%s", ssid, ip)
			} else {
				// Si wpa_state es COMPLETED, considerar conectado aunque no tenga IP aún
				if wpaStateCompleted {
					reallyConnected = true
					log.Printf("WiFi autenticado (wpa_state=COMPLETED) pero sin IP aún: SSID=%s", ssid)
					// Intentar obtener IP si no hay proceso DHCP corriendo
					dhcpCheck := exec.Command("sh", "-c", fmt.Sprintf("ps aux | grep -E '[d]hclient|udhcpc' | grep %s", iface))
					if dhcpOut, _ := dhcpCheck.Output(); len(dhcpOut) == 0 {
						log.Printf("Iniciando DHCP para obtener IP...")
						executeCommand(fmt.Sprintf("sudo dhclient -v %s 2>&1 || sudo udhcpc -i %s -q -n 2>&1 || true", iface, iface))
					} else {
						log.Printf("WiFi está obteniendo IP (DHCP en proceso)")
					}
				} else {
					log.Printf("WiFi tiene SSID pero no está completamente autenticado: SSID=%s, IP=%s", ssid, ip)
				}
			}
		} else if wpaStateCompleted {
			// Si wpa_state es COMPLETED pero no se pudo verificar IP, considerar conectado
			reallyConnected = true
			log.Printf("WiFi autenticado (wpa_state=COMPLETED): SSID=%s", ssid)
		}
	}

	var connectionInfo fiber.Map = nil
	if reallyConnected && ssid != "" {
		connectionInfo = fiber.Map{
			"ssid": ssid,
		}

		iface := "wlan0"
		ipIfaceCmd := exec.Command("sh", "-c", "ip -o link show | awk -F': ' '{print $2}' | grep -E '^wlan|^wl' | head -1")
		if ipIfaceOut, err := ipIfaceCmd.Output(); err == nil {
			if ipIfaceStr := strings.TrimSpace(string(ipIfaceOut)); ipIfaceStr != "" {
				iface = ipIfaceStr
			}
		}

		wpaStatusCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s status 2>/dev/null", iface))
		wpaStatusOut, wpaErr := wpaStatusCmd.CombinedOutput()
		if wpaErr == nil && len(wpaStatusOut) > 0 {
			wpaStatus := string(wpaStatusOut)
			log.Printf("wpa_cli status output for %s: %s", iface, wpaStatus)
			for _, line := range strings.Split(wpaStatus, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "signal=") {
					signalStr := strings.TrimPrefix(line, "signal=")
					signalStr = strings.TrimSpace(signalStr)
					if signalStr != "" && signalStr != "0" {
						if signalInt, err := strconv.Atoi(signalStr); err == nil && signalInt != 0 {
							if signalInt > 0 {
								signalInt = -signalInt
							}
							if signalInt >= -100 && signalInt <= -30 {
								signalStr = strconv.Itoa(signalInt)
								connectionInfo["signal"] = signalStr
								log.Printf("Found signal from wpa_cli: %s dBm", signalStr)
							} else {
								log.Printf("Signal out of range from wpa_cli: %d dBm (ignoring)", signalInt)
							}
						}
					}
				} else if strings.HasPrefix(line, "key_mgmt=") {
					keyMgmt := strings.TrimPrefix(line, "key_mgmt=")
					keyMgmt = strings.TrimSpace(keyMgmt)
					if keyMgmt != "" {
						keyMgmtUpper := strings.ToUpper(keyMgmt)
						if strings.Contains(keyMgmtUpper, "WPA3") || strings.Contains(keyMgmtUpper, "SAE") {
							connectionInfo["security"] = "WPA3"
						} else if strings.Contains(keyMgmtUpper, "WPA2") || strings.Contains(keyMgmtUpper, "WPA-PSK") || strings.Contains(keyMgmtUpper, "WPA") {
							connectionInfo["security"] = "WPA2"
						} else if strings.Contains(keyMgmtUpper, "NONE") || keyMgmtUpper == "" {
							connectionInfo["security"] = "Open"
						} else {
							if strings.Contains(keyMgmtUpper, "PSK") {
								connectionInfo["security"] = "WPA2"
							} else {
								connectionInfo["security"] = keyMgmt
							}
						}
						log.Printf("Found security from wpa_cli: %s (key_mgmt=%s)", connectionInfo["security"], keyMgmt)
					}
				} else if strings.HasPrefix(line, "wpa=") {
					wpaStr := strings.TrimPrefix(line, "wpa=")
					wpaStr = strings.TrimSpace(wpaStr)
					if wpaStr == "2" && (connectionInfo["security"] == nil || connectionInfo["security"] == "") {
						connectionInfo["security"] = "WPA2"
						log.Printf("Found security from wpa_cli wpa field: WPA2")
					} else if wpaStr == "1" && (connectionInfo["security"] == nil || connectionInfo["security"] == "") {
						connectionInfo["security"] = "WPA"
						log.Printf("Found security from wpa_cli wpa field: WPA")
					}
				} else if strings.HasPrefix(line, "freq=") {
					freqStr := strings.TrimPrefix(line, "freq=")
					freqStr = strings.TrimSpace(freqStr)
					if freq, err := strconv.Atoi(freqStr); err == nil && freq > 0 {
						var channel int
						if freq >= 2412 && freq <= 2484 {
							channel = (freq-2412)/5 + 1
						} else if freq >= 5000 && freq <= 5825 {
							channel = (freq - 5000) / 5
						} else if freq >= 5955 && freq <= 7115 {
							channel = (freq - 5955) / 5
						}
						if channel > 0 {
							connectionInfo["channel"] = strconv.Itoa(channel)
							log.Printf("Found channel from wpa_cli: %d (from freq %d MHz)", channel, freq)
						} else {
							log.Printf("Could not convert freq %d to channel", freq)
						}
					}
				}
			}
		} else {
			log.Printf("wpa_cli failed or returned empty for %s: %v", iface, wpaErr)
		}

		if connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0" ||
			connectionInfo["channel"] == nil || connectionInfo["channel"] == "" ||
			connectionInfo["security"] == nil || connectionInfo["security"] == "" {
			log.Printf("Getting additional info from iw for interface %s", iface)
			iwLinkCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo iw dev %s link 2>/dev/null", iface))
			iwLinkOut, iwErr := iwLinkCmd.CombinedOutput()
			if iwErr != nil || len(iwLinkOut) == 0 {
				iwLinkCmd = exec.Command("sh", "-c", fmt.Sprintf("iw dev %s link 2>/dev/null", iface))
				iwLinkOut, iwErr = iwLinkCmd.CombinedOutput()
			}
			if iwErr == nil && len(iwLinkOut) > 0 {
				iwLink := string(iwLinkOut)
				log.Printf("iw link output for %s: %s", iface, iwLink)
				for _, line := range strings.Split(iwLink, "\n") {
					line = strings.TrimSpace(line)
					if (connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0") && strings.Contains(strings.ToLower(line), "signal") {
						parts := strings.Fields(line)
						for i, part := range parts {
							partLower := strings.ToLower(part)
							if (partLower == "signal:" || partLower == "signal") && i+1 < len(parts) {
								signalStr := strings.TrimSpace(parts[i+1])
								signalStr = strings.TrimSuffix(signalStr, "dBm")
								signalStr = strings.TrimSpace(signalStr)
								if signalStr != "" && signalStr != "0" {
									if signalInt, err := strconv.Atoi(signalStr); err == nil && signalInt != 0 {
										if signalInt > 0 {
											signalInt = -signalInt
										}
										if signalInt >= -100 && signalInt <= -30 {
											signalStr = strconv.Itoa(signalInt)
											connectionInfo["signal"] = signalStr
											log.Printf("Found signal from iw: %s dBm", signalStr)
										}
									} else {
										re := regexp.MustCompile(`-?\d+`)
										matches := re.FindString(signalStr)
										if matches != "" {
											if signalInt, err := strconv.Atoi(matches); err == nil {
												if signalInt > 0 {
													signalInt = -signalInt
												}
												if signalInt >= -100 && signalInt <= -30 {
													connectionInfo["signal"] = strconv.Itoa(signalInt)
													log.Printf("Found signal from iw (parsed): %d dBm", signalInt)
												}
											}
										}
									}
								}
								break
							}
						}
					}
					if (connectionInfo["channel"] == nil || connectionInfo["channel"] == "") && strings.Contains(line, "freq:") {
						parts := strings.Fields(line)
						for i, part := range parts {
							if part == "freq:" && i+1 < len(parts) {
								freqStr := strings.TrimSpace(parts[i+1])
								if freq, err := strconv.Atoi(freqStr); err == nil && freq > 0 {
									var channel int
									if freq >= 2412 && freq <= 2484 {
										channel = (freq-2412)/5 + 1
									} else if freq >= 5000 && freq <= 5825 {
										channel = (freq - 5000) / 5
									} else if freq >= 5955 && freq <= 7115 {
										channel = (freq - 5955) / 5
									}
									if channel > 0 {
										connectionInfo["channel"] = strconv.Itoa(channel)
										log.Printf("Found channel from iw: %d (from freq %d)", channel, freq)
									}
								}
								break
							}
						}
					}
					if connectionInfo["security"] == nil || connectionInfo["security"] == "" {
						if strings.Contains(line, "WPA3") || strings.Contains(line, "SAE") {
							connectionInfo["security"] = "WPA3"
							log.Printf("Found security from iw: WPA3")
						} else if strings.Contains(line, "WPA2") || strings.Contains(line, "WPA") {
							connectionInfo["security"] = "WPA2"
							log.Printf("Found security from iw: WPA2")
						}
					}
				}
			} else {
				log.Printf("iw link command failed or returned empty: %v, output: %s", iwErr, string(iwLinkOut))
			}
		}

		if connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0" {
			log.Printf("Trying /proc/net/wireless for signal on %s", iface)
			wirelessCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /proc/net/wireless 2>/dev/null | grep %s", iface))
			wirelessOut, wirelessErr := wirelessCmd.Output()
			if wirelessErr == nil && len(wirelessOut) > 0 {
				wirelessLine := strings.TrimSpace(string(wirelessOut))
				log.Printf("/proc/net/wireless output: %s", wirelessLine)
				parts := strings.Fields(wirelessLine)
				if len(parts) >= 3 {
					if signalLevel, err := strconv.Atoi(strings.TrimSuffix(parts[2], ".")); err == nil && signalLevel > 0 {
						signalDbm := signalLevel / 10
						if signalDbm > 0 {
							connectionInfo["signal"] = fmt.Sprintf("-%d", signalDbm)
							log.Printf("Found signal from /proc/net/wireless: -%d dBm", signalDbm)
						}
					}
				}
			}
		}

		if (connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0") ||
			(connectionInfo["channel"] == nil || connectionInfo["channel"] == "") {
			log.Printf("Trying iwconfig as last resort for interface %s", iface)
			iwconfigCmd := exec.Command("sh", "-c", fmt.Sprintf("iwconfig %s 2>/dev/null", iface))
			iwconfigOut, iwconfigErr := iwconfigCmd.CombinedOutput()
			if iwconfigErr == nil && len(iwconfigOut) > 0 {
				iwconfigStr := string(iwconfigOut)
				log.Printf("iwconfig output: %s", iwconfigStr)
				if connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0" {
					if strings.Contains(iwconfigStr, "Signal level=") {
						parts := strings.Split(iwconfigStr, "Signal level=")
						if len(parts) > 1 {
							signalPart := strings.Fields(parts[1])[0]
							signalStr := strings.TrimSpace(signalPart)
							signalStr = strings.TrimSuffix(signalStr, "dBm")
							signalStr = strings.TrimSpace(signalStr)
							if signalStr != "" && signalStr != "0" {
								connectionInfo["signal"] = signalStr
								log.Printf("Found signal from iwconfig: %s", signalStr)
							}
						}
					}
				}
				if connectionInfo["channel"] == nil || connectionInfo["channel"] == "" {
					if strings.Contains(iwconfigStr, "Channel:") {
						parts := strings.Split(iwconfigStr, "Channel:")
						if len(parts) > 1 {
							channelPart := strings.Fields(parts[1])[0]
							channelStr := strings.TrimSpace(channelPart)
							if channelStr != "" {
								connectionInfo["channel"] = channelStr
								log.Printf("Found channel from iwconfig: %s", channelStr)
							}
						}
					}
				}
			}
		}

		if (connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0") ||
			(connectionInfo["channel"] == nil || connectionInfo["channel"] == "") {
			log.Printf("Trying iw dev %s station dump as additional method", iface)
			iwStationCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo iw dev %s station dump 2>/dev/null", iface))
			iwStationOut, iwStationErr := iwStationCmd.CombinedOutput()
			if iwStationErr == nil && len(iwStationOut) > 0 {
				iwStationStr := string(iwStationOut)
				log.Printf("iw station dump output: %s", iwStationStr)
				if connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0" {
					lines := strings.Split(iwStationStr, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if strings.Contains(strings.ToLower(line), "signal") {
							re := regexp.MustCompile(`-?\d+`)
							matches := re.FindAllString(line, -1)
							for _, match := range matches {
								if signalInt, err := strconv.Atoi(match); err == nil {
									if signalInt > 0 {
										signalInt = -signalInt
									}
									if signalInt >= -100 && signalInt <= -30 {
										connectionInfo["signal"] = strconv.Itoa(signalInt)
										log.Printf("Found signal from iw station dump: %d dBm", signalInt)
										break
									}
								}
							}
						}
					}
				}
			}
		}

		if connectionInfo["signal"] == nil || connectionInfo["signal"] == "" || connectionInfo["signal"] == "0" {
			log.Printf("Warning: Could not determine signal strength for %s after all methods", iface)
			delete(connectionInfo, "signal")
		}
		if connectionInfo["channel"] == nil || connectionInfo["channel"] == "" {
			log.Printf("Warning: Could not determine channel for %s after all methods", iface)
			delete(connectionInfo, "channel")
		}
		if connectionInfo["security"] == nil || connectionInfo["security"] == "" {
			log.Printf("Warning: Could not determine security for %s, defaulting to WPA2", iface)
			connectionInfo["security"] = "WPA2" // Valor por defecto común
		}

		if iface != "" {
			ipCmd := exec.Command("sh", "-c", fmt.Sprintf("ip addr show %s 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1 | head -1", iface))
			ipOut, _ := ipCmd.Output()
			if ipStr := strings.TrimSpace(string(ipOut)); ipStr != "" {
				connectionInfo["ip"] = ipStr
			}

			macCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/address 2>/dev/null", iface))
			macOut, _ := macCmd.Output()
			if macStr := strings.TrimSpace(string(macOut)); macStr != "" {
				connectionInfo["mac"] = macStr
			}

			speedCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/speed 2>/dev/null", iface))
			speedOut, _ := speedCmd.Output()
			if speedStr := strings.TrimSpace(string(speedOut)); speedStr != "" && speedStr != "-1" {
				connectionInfo["speed"] = speedStr + " Mbps"
			}
		}
	}

	if !connected && enabled {
		ifaceCmd := execCommand("nmcli -t -f DEVICE,TYPE dev status 2>/dev/null | grep wifi | head -1 | cut -d: -f1")
		if ifaceOut, err := ifaceCmd.Output(); err == nil {
			iface := strings.TrimSpace(string(ifaceOut))
			if iface != "" {
				if connectionInfo == nil {
					connectionInfo = fiber.Map{}
				}
				macCmd := exec.Command("sh", "-c", fmt.Sprintf("cat /sys/class/net/%s/address 2>/dev/null", iface))
				macOut, _ := macCmd.Output()
				if macStr := strings.TrimSpace(string(macOut)); macStr != "" {
					connectionInfo["mac"] = macStr
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"enabled":            enabled,
		"connected":          reallyConnected,
		"current_connection": ssid,
		"ssid":               ssid,
		"hard_blocked":       hardBlocked,
		"soft_blocked":       softBlocked,
		"connection_info":    connectionInfo,
	})
}

func wifiLegacyStoredNetworksHandler(c *fiber.Ctx) error {
	var networks []fiber.Map
	var lastConnected []string

	interfaceName := "wlan0"

	listCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s list_networks 2>/dev/null", interfaceName))
	listOut, err := listCmd.CombinedOutput()

	if err == nil && len(listOut) > 0 {
		lines := strings.Split(string(listOut), "\n")
		for i, line := range lines {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue // Saltar encabezado y líneas vacías
			}

			fields := strings.Fields(line)
			if len(fields) >= 2 {
				networkID := fields[0]
				ssid := fields[1]

				if ssid != "" && ssid != "--" {
					ssid = strings.Trim(ssid, "\"")

					network := fiber.Map{
						"id":     networkID,
						"ssid":   ssid,
						"status": "saved",
					}

					enabledCmd := exec.Command("sh", "-c", fmt.Sprintf("sudo wpa_cli -i %s get_network %s disabled 2>/dev/null", interfaceName, networkID))
					enabledOut, _ := enabledCmd.CombinedOutput()
					if strings.TrimSpace(string(enabledOut)) == "0" {
						network["enabled"] = true
						lastConnected = append(lastConnected, ssid)
					} else {
						network["enabled"] = false
					}

					networks = append(networks, network)
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"networks":       networks,
		"last_connected": lastConnected,
	})
}

func wifiLegacyAutoconnectHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"success": false})
}

func wifiLegacyScanHandler(c *fiber.Ctx) error {
	userInterface := c.Locals("user")
	if userInterface == nil {
		log.Printf("ERROR: Usuario no encontrado en wifiLegacyScanHandler")
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "No autenticado. Por favor, inicia sesión nuevamente.",
		})
	}

	user, ok := userInterface.(*User)
	if !ok || user == nil {
		log.Printf("ERROR: Usuario inválido en wifiLegacyScanHandler")
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "Usuario no encontrado. Por favor, inicia sesión nuevamente.",
		})
	}

	interfaceName := c.Query("interface", DefaultWiFiInterface)
	result := scanWiFiNetworks(interfaceName)
	if networks, ok := result["networks"]; ok {
		return c.JSON(fiber.Map{"success": true, "networks": networks})
	}
	return c.JSON(fiber.Map{"success": true, "networks": []fiber.Map{}})
}

func wifiLegacyDisconnectHandler(c *fiber.Ctx) error {
	user, ok := GetUser(c)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
	}
	userID := user.ID

	activeConnCmd := execCommand("nmcli -t -f NAME,TYPE,DEVICE connection show --active | grep -i wifi")
	activeConnOut, err := activeConnCmd.Output()

	var connectionName string
	if err == nil && len(activeConnOut) > 0 {
		lines := strings.Split(strings.TrimSpace(string(activeConnOut)), "\n")
		if len(lines) > 0 {
			parts := strings.Split(lines[0], ":")
			if len(parts) > 0 {
				connectionName = strings.TrimSpace(parts[0])
			}
		}
	}

	if connectionName != "" {
		disconnectCmd := execCommand(fmt.Sprintf("nmcli connection down '%s'", connectionName))
		disconnectOut, disconnectErr := disconnectCmd.CombinedOutput()

		if disconnectErr == nil {
			InsertLog("INFO", fmt.Sprintf("WiFi desconectado: %s (usuario: %s)", connectionName, user.Username), "wifi", &userID)
			return c.JSON(fiber.Map{"success": true, "message": "Disconnected from " + connectionName})
		}

		log.Printf("Error desconectando conexión %s: %s, intentando desconectar dispositivo", connectionName, string(disconnectOut))
	}

	wifiDeviceCmd := execCommand("nmcli -t -f DEVICE,TYPE device status | grep -i wifi | head -1 | cut -d: -f1")
	wifiDeviceOut, err := wifiDeviceCmd.Output()

	if err == nil && len(wifiDeviceOut) > 0 {
		deviceName := strings.TrimSpace(string(wifiDeviceOut))
		if deviceName != "" {
			deviceDisconnectCmd := execCommand(fmt.Sprintf("nmcli device disconnect '%s'", deviceName))
			deviceDisconnectOut, deviceDisconnectErr := deviceDisconnectCmd.CombinedOutput()

			if deviceDisconnectErr == nil {
				InsertLog("INFO", fmt.Sprintf("Dispositivo WiFi desconectado: %s (usuario: %s)", deviceName, user.Username), "wifi", &userID)
				return c.JSON(fiber.Map{"success": true, "message": "Disconnected from WiFi device " + deviceName})
			}

			log.Printf("Error desconectando dispositivo %s: %s", deviceName, string(deviceDisconnectOut))
		}
	}

	networkingOffCmd := execCommand("nmcli networking off")
	networkingOffOut, networkingOffErr := networkingOffCmd.CombinedOutput()

	if networkingOffErr != nil {
		errorMsg := fmt.Sprintf("Error desconectando WiFi: %s", strings.TrimSpace(string(networkingOffOut)))
		InsertLog("ERROR", fmt.Sprintf("Error en desconexión WiFi (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
	}

	time.Sleep(1 * time.Second)

	networkingOnCmd := execCommand("nmcli networking on")
	networkingOnOut, networkingOnErr := networkingOnCmd.CombinedOutput()

	if networkingOnErr != nil {
		errorMsg := fmt.Sprintf("Error reactivando networking: %s", strings.TrimSpace(string(networkingOnOut)))
		InsertLog("ERROR", fmt.Sprintf("Error reactivando networking (usuario: %s): %s", user.Username, errorMsg), "wifi", &userID)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": errorMsg})
	}

	InsertLog("INFO", fmt.Sprintf("WiFi desconectado mediante fallback (usuario: %s)", user.Username), "wifi", &userID)
	return c.JSON(fiber.Map{"success": true, "message": "Disconnected from WiFi"})
}

func strconvAtoiSafe(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid int")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func mapActiveStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "active" {
		return "connected"
	}
	return "disconnected"
}

func mapBoolStatus(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "true" || v == "1" || v == "yes" {
		return "connected"
	}
	return "disconnected"
}
