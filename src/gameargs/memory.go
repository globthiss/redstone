package gameargs

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"redstone/modes"
)

func TotalSystemMemoryMB() int {
	switch runtime.GOOS {
	case "linux":
		return totalMemoryLinux()
	case "windows":
		return totalMemoryWindows()
	case "darwin":
		return totalMemoryDarwin()
	default:
		return 0
	}
}

func totalMemoryLinux() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

func totalMemoryDarwin() int {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	bytesVal, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return int(bytesVal / 1024 / 1024)
}

func totalMemoryWindows() int {
	out, err := exec.Command("wmic", "ComputerSystem", "get", "TotalPhysicalMemory").Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "TotalPhysicalMemory") {
			continue
		}
		bytesVal, err := strconv.ParseInt(line, 10, 64)
		if err == nil {
			return int(bytesVal / 1024 / 1024)
		}
	}
	return 0
}

func ResolveMemoryMB(mode modes.MemoryMode, manualXmx, manualXms int) (xmxMB, xmsMB int) {
	total := TotalSystemMemoryMB()

	switch mode {
	case modes.MemoryModeManual:
		return manualXmx, manualXms
	case modes.MemoryModeMinimal:
		return 1536, 512
	case modes.MemoryModeMaximum:
		if total <= 0 {
			return 8192, 2048
		}
		return int(float64(total) * 0.75), 1024
	case modes.MemoryModeBalanced:
		if total <= 0 {
			return 4096, 1024
		}
		return int(float64(total) * 0.5), 1024
	case modes.MemoryModeAuto:
		fallthrough
	default:
		if total <= 0 {
			return 4096, 1024
		}
		val := int(float64(total) * 0.4)
		if val < 2048 {
			val = 2048
		}
		if val > 8192 {
			val = 8192
		}
		return val, 1024
	}
}
