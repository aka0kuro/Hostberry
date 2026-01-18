package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func getSystemInfo() map[string]interface{} {
	result := make(map[string]interface{})

	if hostname, err := exec.Command("hostname").Output(); err == nil {
		result["hostname"] = strings.TrimSpace(string(hostname))
	} else {
		result["hostname"] = "unknown"
	}

	if kernel, err := exec.Command("uname", "-r").Output(); err == nil {
		result["kernel_version"] = strings.TrimSpace(string(kernel))
	} else {
		result["kernel_version"] = "unknown"
	}

	if arch, err := exec.Command("uname", "-m").Output(); err == nil {
		result["architecture"] = strings.TrimSpace(string(arch))
	} else {
		result["architecture"] = "unknown"
	}

	processorCmd := exec.Command("sh", "-c", "cat /proc/cpuinfo | grep -m1 'model name\\|Processor\\|Hardware' | cut -d ':' -f 2 | sed 's/^[[:space:]]*//'")
	if processor, err := processorCmd.Output(); err == nil {
		result["processor"] = strings.TrimSpace(string(processor))
	} else {
		result["processor"] = "ARM Processor"
	}

	osVersion := "Unknown"
	if osRelease, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(osRelease), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osVersion = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}
	result["os_version"] = osVersion

	uptimeCmd := exec.Command("sh", "-c", "cat /proc/uptime | awk '{print int($1)}'")
	if uptimeOut, err := uptimeCmd.Output(); err == nil {
		if uptimeSeconds, err := strconv.Atoi(strings.TrimSpace(string(uptimeOut))); err == nil {
			result["uptime_seconds"] = uptimeSeconds
			result["boot_time"] = time.Now().Unix() - int64(uptimeSeconds)
		} else {
			result["uptime_seconds"] = 0
			result["boot_time"] = time.Now().Unix()
		}
	} else {
		result["uptime_seconds"] = 0
		result["boot_time"] = time.Now().Unix()
	}

	loadavgCmd := exec.Command("sh", "-c", "cat /proc/loadavg | awk '{print $1 \", \" $2 \", \" $3}'")
	if loadavg, err := loadavgCmd.Output(); err == nil {
		result["load_average"] = strings.TrimSpace(string(loadavg))
	} else {
		result["load_average"] = "0.00, 0.00, 0.00"
	}

	return result
}

func getSystemStats() map[string]interface{} {
	result := make(map[string]interface{})

	cpuCmd := exec.Command("sh", "-c", "grep 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$3+$4+$5)} END {print usage}'")
	if cpuOut, err := cpuCmd.Output(); err == nil {
		cpuStr := strings.TrimSpace(string(cpuOut))
		cpuStr = strings.ReplaceAll(cpuStr, ",", ".")
		if cpuUsage, err := strconv.ParseFloat(cpuStr, 64); err == nil && cpuUsage >= 0 && cpuUsage <= 100 {
			result["cpu_usage"] = cpuUsage
		} else {
			cpuCmd2 := exec.Command("sh", "-c", "top -bn1 | grep 'Cpu(s)' | awk -F'id,' '{split($1,a,\"%\"); for(i in a){if(a[i] ~ /^[0-9]/){print 100-a[i];break}}}'")
			if cpuOut2, err2 := cpuCmd2.Output(); err2 == nil {
				cpuStr2 := strings.TrimSpace(string(cpuOut2))
				cpuStr2 = strings.ReplaceAll(cpuStr2, ",", ".")
				if cpuUsage2, err2 := strconv.ParseFloat(cpuStr2, 64); err2 == nil && cpuUsage2 >= 0 && cpuUsage2 <= 100 {
					result["cpu_usage"] = cpuUsage2
				} else {
					result["cpu_usage"] = 0.0
				}
			} else {
				result["cpu_usage"] = 0.0
			}
		}
	} else {
		result["cpu_usage"] = 0.0
	}

	memCmd := exec.Command("sh", "-c", "free | grep Mem | awk '{printf \"%.2f\", $3/$2 * 100.0}'")
	if memOut, err := memCmd.Output(); err == nil {
		memStr := strings.TrimSpace(string(memOut))
		memStr = strings.ReplaceAll(memStr, ",", ".")
		if memUsage, err := strconv.ParseFloat(memStr, 64); err == nil && memUsage >= 0 && memUsage <= 100 {
			result["memory_usage"] = memUsage
		} else {
			result["memory_usage"] = 0.0
		}
	} else {
		result["memory_usage"] = 0.0
	}

	diskCmd := exec.Command("sh", "-c", "df / | tail -1 | awk '{print $5}' | sed 's/%//'")
	if diskOut, err := diskCmd.Output(); err == nil {
		if diskUsage, err := strconv.ParseFloat(strings.TrimSpace(string(diskOut)), 64); err == nil && diskUsage >= 0 && diskUsage <= 100 {
			result["disk_usage"] = diskUsage
		} else {
			result["disk_usage"] = 0.0
		}
	} else {
		result["disk_usage"] = 0.0
	}

	uptimeCmd := exec.Command("sh", "-c", "cat /proc/uptime | awk '{print int($1)}'")
	if uptimeOut, err := uptimeCmd.Output(); err == nil {
		if uptimeSeconds, err := strconv.Atoi(strings.TrimSpace(string(uptimeOut))); err == nil {
			result["uptime"] = uptimeSeconds
		} else {
			result["uptime"] = 0
		}
	} else {
		result["uptime"] = 0
	}

	tempCmd := exec.Command("sh", "-c", "cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null | awk '{print $1/1000}'")
	if tempOut, err := tempCmd.Output(); err == nil {
		if temp, err := strconv.ParseFloat(strings.TrimSpace(string(tempOut)), 64); err == nil {
			result["cpu_temperature"] = temp
		} else {
			result["cpu_temperature"] = 0.0
		}
	} else {
		result["cpu_temperature"] = 0.0
	}

	coresCmd := exec.Command("nproc")
	if coresOut, err := coresCmd.Output(); err == nil {
		if cores, err := strconv.Atoi(strings.TrimSpace(string(coresOut))); err == nil {
			result["cpu_cores"] = cores
		} else {
			result["cpu_cores"] = 1
		}
	} else {
		result["cpu_cores"] = 1
	}

	if hostname, err := exec.Command("hostname").Output(); err == nil {
		result["hostname"] = strings.TrimSpace(string(hostname))
	} else {
		result["hostname"] = "unknown"
	}

	if kernel, err := exec.Command("uname", "-r").Output(); err == nil {
		result["kernel_version"] = strings.TrimSpace(string(kernel))
	} else {
		result["kernel_version"] = "unknown"
	}

	if arch, err := exec.Command("uname", "-m").Output(); err == nil {
		result["architecture"] = strings.TrimSpace(string(arch))
	} else {
		result["architecture"] = "unknown"
	}

	processorCmd := exec.Command("sh", "-c", "cat /proc/cpuinfo | grep -m1 'model name\\|Processor\\|Hardware' | cut -d ':' -f 2 | sed 's/^[[:space:]]*//'")
	if processor, err := processorCmd.Output(); err == nil {
		processorStr := strings.TrimSpace(string(processor))
		if processorStr != "" {
			result["processor"] = processorStr
		} else {
			result["processor"] = "ARM Processor"
		}
	} else {
		result["processor"] = "ARM Processor"
	}

	osVersion := "Unknown"
	if osRelease, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(osRelease), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osVersion = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}
	result["os_version"] = osVersion

	loadavgCmd := exec.Command("sh", "-c", "cat /proc/loadavg | awk '{print $1 \", \" $2 \", \" $3}'")
	if loadavg, err := loadavgCmd.Output(); err == nil {
		result["load_average"] = strings.TrimSpace(string(loadavg))
	} else {
		result["load_average"] = "0.00, 0.00, 0.00"
	}

	return result
}

func systemRestart(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.system_restart_requested", user)

	restartCmd := "systemctl reboot"
	if _, err := executeCommand(restartCmd); err != nil {
		LogTf("logs.system_restart_fallback", err)
		shutdownPaths := []string{"/usr/sbin/shutdown", "/sbin/shutdown", "shutdown"}
		found := false
		for _, path := range shutdownPaths {
			testCmd := fmt.Sprintf("command -v %s 2>/dev/null", path)
			if testOut, testErr := executeCommand(testCmd); testErr == nil && strings.TrimSpace(testOut) != "" {
				restartCmd = fmt.Sprintf("%s -r +1", path)
				if out2, err2 := executeCommand(restartCmd); err2 == nil {
					result["success"] = true
					result["message"] = "Sistema se reiniciará en 1 minuto"
					result["output"] = strings.TrimSpace(out2)
					LogT("logs.system_restart_success")
					return result
				}
				found = true
				break
			}
		}
		if !found {
			restartCmd = "reboot"
			if _, err3 := executeCommand(restartCmd); err3 != nil {
				result["success"] = false
				result["error"] = err3.Error()
				result["message"] = "Error al ejecutar comando de reinicio"
				LogTf("logs.system_restart_error", err3)
				return result
			}
			result["success"] = true
			result["message"] = "Sistema se reiniciará en breve"
			result["output"] = ""
			return result
		}
		result["success"] = false
		result["error"] = err.Error()
		result["message"] = "Error al ejecutar comando de reinicio"
		LogTf("logs.system_restart_error", err)
		return result
	}

	result["success"] = true
	result["message"] = "Sistema se reiniciará en breve"
	result["output"] = ""
	LogT("logs.system_restart_success")
	return result
}

func systemShutdown(user string) map[string]interface{} {
	result := make(map[string]interface{})

	if user == "" {
		user = "unknown"
	}

	LogTf("logs.system_shutdown_requested", user)

	shutdownCmd := "systemctl poweroff"
	if _, err := executeCommand(shutdownCmd); err != nil {
		LogTf("logs.system_shutdown_fallback", err)
		shutdownPaths := []string{"/usr/sbin/shutdown", "/sbin/shutdown", "shutdown"}
		found := false
		for _, path := range shutdownPaths {
			testCmd := fmt.Sprintf("command -v %s 2>/dev/null", path)
			if testOut, testErr := executeCommand(testCmd); testErr == nil && strings.TrimSpace(testOut) != "" {
				shutdownCmd = fmt.Sprintf("%s -h +1", path)
				if out2, err2 := executeCommand(shutdownCmd); err2 == nil {
					result["success"] = true
					result["message"] = "Sistema se apagará en 1 minuto"
					result["output"] = strings.TrimSpace(out2)
					LogT("logs.system_shutdown_success")
					return result
				}
				found = true
				break
			}
		}
		if !found {
			shutdownCmd = "poweroff"
			if _, err3 := executeCommand(shutdownCmd); err3 != nil {
				result["success"] = false
				result["error"] = err3.Error()
				result["message"] = "Error al ejecutar comando de apagado"
				LogTf("logs.system_shutdown_error", err3)
				return result
			}
			result["success"] = true
			result["message"] = "Sistema se apagará en breve"
			result["output"] = ""
			return result
		}
		result["success"] = false
		result["error"] = err.Error()
		result["message"] = "Error al ejecutar comando de apagado"
		LogTf("logs.system_shutdown_error", err)
		return result
	}

	result["success"] = true
	result["message"] = "Sistema se apagará en breve"
	result["output"] = ""
	LogT("logs.system_shutdown_success")
	return result
}
