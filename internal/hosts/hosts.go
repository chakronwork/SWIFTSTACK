// Package hosts manages additions and removals of local domains in the Windows hosts file.
package hosts

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const markerPrefix = "# webserver-stack-vhost: "

// GetHostsPath returns the standard Windows hosts file path.
func GetHostsPath() string {
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	return filepath.Join(sysRoot, "System32", "drivers", "etc", "hosts")
}

// HasEntry checks if a domain already points to 127.0.0.1 in the hosts file.
func HasEntry(domain string) (bool, error) {
	file, err := os.Open(GetHostsPath())
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ip := fields[0]
			for _, d := range fields[1:] {
				if (ip == "127.0.0.1" || ip == "::1") && strings.EqualFold(d, domain) {
					return true, nil
				}
			}
		}
	}
	return false, scanner.Err()
}

// AddEntryDirect adds a domain directly to the hosts file if writable.
func AddEntryDirect(domain string) error {
	exists, err := HasEntry(domain)
	if err == nil && exists {
		return nil
	}

	f, err := os.OpenFile(GetHostsPath(), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := fmt.Sprintf("\n127.0.0.1 %s %s%s\n", domain, markerPrefix, domain)
	_, err = f.WriteString(entry)
	return err
}

// RemoveEntryDirect removes a domain from the hosts file if writable.
func RemoveEntryDirect(domain string) error {
	path := GetHostsPath()
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	changed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		isTarget := false
		if len(fields) >= 2 {
			ip := fields[0]
			if ip == "127.0.0.1" || ip == "::1" {
				for _, d := range fields[1:] {
					if strings.EqualFold(d, domain) {
						isTarget = true
						break
					}
				}
			}
		}

		if isTarget {
			changed = true
			continue
		}
		newLines = append(newLines, line)
	}

	if changed {
		return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644)
	}
	return nil
}

// RunElevatedHelper invokes a minimal UAC prompt to add or remove hosts entries.
func RunElevatedHelper(action, domain string) error {
	// First try direct write
	var err error
	if action == "add" {
		err = AddEntryDirect(domain)
	} else if action == "remove" {
		err = RemoveEntryDirect(domain)
	}

	if err == nil {
		return nil
	}

	// Direct write failed (normal for standard user). Invoke minimal UAC helper via PowerShell Start-Process -Verb RunAs
	hostsPath := GetHostsPath()
	var psScript string
	if action == "add" {
		psScript = fmt.Sprintf(`Add-Content -Path '%s' -Value '127.0.0.1 %s # webserver-stack-vhost: %s'`, hostsPath, domain, domain)
	} else {
		psScript = fmt.Sprintf(`$c = Get-Content -Path '%s' | Where-Object { $_ -notmatch '(?i)\b%s\b' }; Set-Content -Path '%s' -Value $c`, hostsPath, domain, hostsPath)
	}

	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command",
		fmt.Sprintf(`Start-Process powershell -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList "-NoProfile -Command %s"`, psScript))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	return cmd.Run()
}
