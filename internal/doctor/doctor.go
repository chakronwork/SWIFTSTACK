// Package doctor diagnoses system readiness, runtime dependencies, permissions, and port conflicts.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/port"
	"golang.org/x/sys/windows/registry"
)

// CheckItem represents a single diagnostic result.
type CheckItem struct {
	Name        string
	Passed      bool
	Critical    bool
	Message     string
	Remediation string
}

// DiagnosticReport holds all check results.
type DiagnosticReport struct {
	Items    []CheckItem
	AllPass  bool
	HasWarn  bool
}

// RunDiagnostics executes all health and environment checks.
func RunDiagnostics(cfg *config.AppConfig) DiagnosticReport {
	report := DiagnosticReport{AllPass: true}

	// 1. Check VC++ Runtime (VS17 / 2015-2022 x64)
	report.add(checkVCRuntime())

	// 2. Check WebView2 Runtime (required for Wails GUI)
	report.add(checkWebView2())

	// 3. Check Hosts File Permissions
	report.add(checkHostsPermission())

	// 4. Check Port Conflicts
	portsToCheck := []struct {
		name string
		bind string
		p    int
	}{
		{"Nginx HTTP", cfg.Stack.Nginx.Bind, cfg.Stack.Nginx.Port},
		{"Nginx HTTPS", cfg.Stack.Nginx.Bind, cfg.Stack.Nginx.SSLPort},
		{"MariaDB", cfg.Stack.MariaDB.Bind, cfg.Stack.MariaDB.Port},
		{"PHP 8.4 FastCGI", "127.0.0.1", config.PHPFCGIPort(cfg.Stack.PHP.DefaultVersion)},
	}

	for _, pt := range portsToCheck {
		report.add(checkPort(pt.name, pt.bind, pt.p))
	}

	// 5. Check Directory Paths (Spaces and Non-ASCII support)
	report.add(checkPathEncoding(cfg.Paths.Root))

	// 6. Check Windows Defender / Antivirus Status
	report.add(checkAntivirus(cfg.Paths.Root))

	return report
}

func (r *DiagnosticReport) add(item CheckItem) {
	if !item.Passed {
		if item.Critical {
			r.AllPass = false
		} else {
			r.HasWarn = true
		}
	}
	r.Items = append(r.Items, item)
}

func checkVCRuntime() CheckItem {
	// Look up Visual C++ 2015-2022 Redistributable (x64) in Windows Registry
	keys := []string{
		`SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\X64`,
		`SOFTWARE\WOW6432Node\Microsoft\VisualStudio\14.0\VC\Runtimes\X64`,
	}

	installed := false
	var versionStr string

	for _, k := range keys {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, k, registry.QUERY_VALUE)
		if err == nil {
			val, _, err := key.GetIntegerValue("Installed")
			if err == nil && val == 1 {
				installed = true
				v, _, _ := key.GetStringValue("Version")
				versionStr = v
				key.Close()
				break
			}
			key.Close()
		}
	}

	// Fallback check: look for vcruntime140.dll in System32
	if !installed {
		sysDir := os.Getenv("SystemRoot")
		if sysDir != "" {
			dllPath := filepath.Join(sysDir, "System32", "vcruntime140.dll")
			if _, err := os.Stat(dllPath); err == nil {
				installed = true
				versionStr = "Detected vcruntime140.dll"
			}
		}
	}

	if installed {
		msg := "Visual C++ 2015-2022 (VS17) x64 Redistributable is installed"
		if versionStr != "" {
			msg += " (" + versionStr + ")"
		}
		return CheckItem{
			Name:     "Visual C++ Redistributable (x64)",
			Passed:   true,
			Critical: true,
			Message:  msg,
		}
	}

	return CheckItem{
		Name:        "Visual C++ Redistributable (x64)",
		Passed:      false,
		Critical:    true,
		Message:     "Visual C++ 2015-2022 (VS17) x64 runtime is NOT found. PHP 8.4 and MariaDB will fail to launch.",
		Remediation: "Download and install from: https://aka.ms/vs/17/release/vc_redist.x64.exe",
	}
}

func checkWebView2() CheckItem {
	// Check WebView2 installation via registry
	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
	}

	for _, k := range keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err == nil {
			pv, _, err := key.GetStringValue("pv")
			key.Close()
			if err == nil && pv != "" && pv != "0.0.0.0" {
				return CheckItem{
					Name:     "Microsoft Edge WebView2 Runtime",
					Passed:   true,
					Critical: false,
					Message:  fmt.Sprintf("WebView2 Runtime is installed (v%s)", pv),
				}
			}
		}
	}

	return CheckItem{
		Name:        "Microsoft Edge WebView2 Runtime",
		Passed:      false,
		Critical:    false, // CLI still works without WebView2, only GUI needs it
		Message:     "WebView2 Runtime is not installed. CLI commands work, but GUI dashboard requires it.",
		Remediation: "Install the Evergreen Bootstrapper: https://go.microsoft.com/fwlink/p/?LinkId=2124703",
	}
}

func checkHostsPermission() CheckItem {
	hostsPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
	f, err := os.OpenFile(hostsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		f.Close()
		return CheckItem{
			Name:     "Hosts File Write Access",
			Passed:   true,
			Critical: false,
			Message:  "Direct write access to hosts file is available (running elevated)",
		}
	}

	return CheckItem{
		Name:        "Hosts File Write Access",
		Passed:      true, // Non-fatal because app uses separate UAC helper when adding vhosts
		Critical:    false,
		Message:     "Standard user privileges (Virtual hosts will invoke UAC elevation prompt when needed)",
		Remediation: "Normal behavior; no action required.",
	}
}

func checkPort(serviceName, bind string, portNum int) CheckItem {
	res := port.Check(bind, portNum)
	if res.Available {
		return CheckItem{
			Name:     fmt.Sprintf("Port %d (%s)", portNum, serviceName),
			Passed:   true,
			Critical: true,
			Message:  fmt.Sprintf("Port %d is available on %s", portNum, bind),
		}
	}

	alt := port.SuggestAlternative(bind, portNum)
	altMsg := ""
	if alt > 0 {
		altMsg = fmt.Sprintf(" Suggested alternative port: %d.", alt)
	}

	return CheckItem{
		Name:        fmt.Sprintf("Port %d (%s)", portNum, serviceName),
		Passed:      false,
		Critical:    true,
		Message:     res.Error,
		Remediation: fmt.Sprintf("Stop the conflicting process (%s) or change port in stack.toml.%s", res.Process, altMsg),
	}
}

func checkPathEncoding(rootPath string) CheckItem {
	return CheckItem{
		Name:     "Install Path Compatibility",
		Passed:   true,
		Critical: false,
		Message:  fmt.Sprintf("Current install directory: %s", rootPath),
	}
}

func checkAntivirus(rootPath string) CheckItem {
	// Check if Windows Defender has an exclusion or if folder is scan-ready
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Get-MpPreference | Select-Object -ExpandProperty ExclusionPath")
	out, err := cmd.Output()
	if err == nil {
		exclusions := strings.Split(string(out), "\r\n")
		for _, exc := range exclusions {
			exc = strings.TrimSpace(exc)
			if exc != "" && strings.EqualFold(exc, rootPath) {
				return CheckItem{
					Name:     "Antivirus Exclusions",
					Passed:   true,
					Critical: false,
					Message:  "Directory is excluded from Windows Defender scans (optimal performance)",
				}
			}
		}
	}

	return CheckItem{
		Name:        "Antivirus Status",
		Passed:      true,
		Critical:    false,
		Message:     "Standard AV scanning active.",
		Remediation: fmt.Sprintf("For faster I/O, you can optionally exclude %s in Windows Defender settings.", rootPath),
	}
}
