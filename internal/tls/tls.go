// Package tls handles local Certificate Authority (CA) installation and per-site SSL certificates via mkcert.
package tls

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/logger"
	"github.com/chakronwork/webserver-stack/internal/templates"
)

// CertResult holds the paths to the generated certificate and private key.
type CertResult struct {
	CertFile string
	KeyFile  string
}

// EnsureRootCA installs the mkcert local CA if not already installed.
func EnsureRootCA(cfg *config.AppConfig) error {
	log := logger.G()
	mkcertExe := cfg.Paths.MkcertPath()

	if _, err := os.Stat(mkcertExe); err != nil {
		return fmt.Errorf("mkcert executable not found at %s. Please run 'webserver init'", mkcertExe)
	}

	// Check if CAROOT exists
	cmd := exec.Command(mkcertExe, "-CAROOT")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err == nil {
		caDir := string(out)
		if _, err := os.Stat(filepath.Join(caDir, "rootCA.pem")); err == nil {
			return nil // already installed
		}
	}

	log.Info("Installing local Root CA using mkcert (may prompt for UAC)...")
	// Run mkcert -install with elevation helper
	installCmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command",
		fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList '-install' -Verb RunAs -Wait -WindowStyle Hidden`, mkcertExe))
	installCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	return installCmd.Run()
}

// GenerateCert creates a signed certificate and private key for the given domain.
func GenerateCert(cfg *config.AppConfig, domain string) (*CertResult, error) {
	log := logger.G()
	mkcertExe := cfg.Paths.MkcertPath()

	if _, err := os.Stat(mkcertExe); err != nil {
		return nil, fmt.Errorf("mkcert executable not found at %s", mkcertExe)
	}

	sslDir := filepath.Join(cfg.Paths.Conf, "ssl")
	if err := os.MkdirAll(sslDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create ssl dir: %w", err)
	}

	certFile := filepath.Join(sslDir, domain+".crt")
	keyFile := filepath.Join(sslDir, domain+".key")

	// If already generated, return existing paths
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return &CertResult{
				CertFile: templates.ToForwardSlash(certFile),
				KeyFile:  templates.ToForwardSlash(keyFile),
			}, nil
		}
	}

	log.Info("Generating trusted SSL certificate for %s ...", domain)

	cmd := exec.Command(mkcertExe,
		"-cert-file", certFile,
		"-key-file", keyFile,
		domain,
		"*."+domain,
		"127.0.0.1",
		"::1",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate for %s: %w\nOutput: %s", domain, err, string(out))
	}

	return &CertResult{
		CertFile: templates.ToForwardSlash(certFile),
		KeyFile:  templates.ToForwardSlash(keyFile),
	}, nil
}
