// Package site manages virtual host creation, multi-PHP routing, and Nginx validation with rollback.
package site

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/hosts"
	"github.com/chakronwork/webserver-stack/internal/logger"
	"github.com/chakronwork/webserver-stack/internal/templates"
	"github.com/chakronwork/webserver-stack/internal/tls"
)

// Site represents a configured virtual host.
type Site struct {
	Name       string `json:"name"`
	Root       string `json:"root"`
	PHPVersion string `json:"php_version"`
	FCGIPort   int    `json:"fcgi_port"`
	Port       int    `json:"port"`
	SSLPort    int    `json:"ssl_port"`
	HTTPS      bool   `json:"https"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
}

// AddSiteOptions contains parameters for adding a new site.
type AddSiteOptions struct {
	Name       string
	Root       string
	PHPVersion string
	HTTPS      bool
}

// Add creates a new virtual host, generates certs if HTTPS is requested, and updates Nginx safely.
func Add(cfg *config.AppConfig, opts AddSiteOptions) (*Site, error) {
	log := logger.G()

	if opts.Name == "" {
		return nil, fmt.Errorf("site domain name is required (e.g. myapp.test)")
	}

	if opts.PHPVersion == "" {
		opts.PHPVersion = cfg.Stack.PHP.DefaultVersion
	}

	// Default root to www/<name>
	if opts.Root == "" {
		opts.Root = filepath.Join(cfg.Paths.WWW, opts.Name)
	}

	// Ensure document root directory exists
	if err := os.MkdirAll(opts.Root, 0755); err != nil {
		return nil, fmt.Errorf("cannot create project root %s: %w", opts.Root, err)
	}

	// Ensure a basic index.php exists if directory is empty
	indexFile := filepath.Join(opts.Root, "index.php")
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		content := fmt.Sprintf("<?php\necho '<h1>Welcome to %s</h1>';\necho '<p>Powered by PHP ' . PHP_VERSION . ' on webserver stack</p>';\n", opts.Name)
		os.WriteFile(indexFile, []byte(content), 0644)
	}

	fcgiPort := config.PHPFCGIPort(opts.PHPVersion)

	site := &Site{
		Name:       opts.Name,
		Root:       templates.ToForwardSlash(opts.Root),
		PHPVersion: opts.PHPVersion,
		FCGIPort:   fcgiPort,
		Port:       cfg.Stack.Nginx.Port,
		SSLPort:    cfg.Stack.Nginx.SSLPort,
		HTTPS:      opts.HTTPS,
	}

	if opts.HTTPS {
		// Ensure Root CA and generate certificate
		_ = tls.EnsureRootCA(cfg)
		certs, err := tls.GenerateCert(cfg, opts.Name)
		if err != nil {
			log.Warn("Failed generating SSL cert: %v, continuing with HTTP only", err)
			site.HTTPS = false
		} else {
			site.CertFile = certs.CertFile
			site.KeyFile = certs.KeyFile
		}
	}

	// 1. Render vhost config
	vhostDir := filepath.Join(cfg.Paths.Conf, "vhosts")
	os.MkdirAll(vhostDir, 0755)
	targetConf := filepath.Join(vhostDir, opts.Name+".conf")
	backupConf := targetConf + ".bak"

	// Backup existing if any
	if _, err := os.Stat(targetConf); err == nil {
		content, _ := os.ReadFile(targetConf)
		os.WriteFile(backupConf, content, 0644)
	}

	tmplPath := filepath.Join(cfg.Paths.Root, "templates", "vhost.conf.tmpl")
	var tmplContent string
	if c, err := os.ReadFile(tmplPath); err == nil {
		tmplContent = string(c)
	} else {
		tmplContent = templates.VhostConfTemplate
	}

	rendered, err := RenderVhost(tmplContent, site)
	if err != nil {
		return nil, fmt.Errorf("failed to render vhost template: %w", err)
	}

	if err := os.WriteFile(targetConf, []byte(rendered), 0644); err != nil {
		return nil, fmt.Errorf("failed to write vhost config: %w", err)
	}

	// 2. Validate Nginx config test (nginx -t)
	if err := TestNginxConfig(cfg); err != nil {
		log.Error("Nginx configuration test failed! Rolling back vhost config...")
		// Rollback
		if _, bErr := os.Stat(backupConf); bErr == nil {
			bContent, _ := os.ReadFile(backupConf)
			os.WriteFile(targetConf, bContent, 0644)
			os.Remove(backupConf)
		} else {
			os.Remove(targetConf)
		}
		return nil, fmt.Errorf("nginx -t test failed (changes reverted): %w", err)
	}
	os.Remove(backupConf)

	// 3. Add to Windows hosts file
	if err := hosts.RunElevatedHelper("add", opts.Name); err != nil {
		log.Warn("Could not auto-add %s to hosts file (UAC cancelled or denied): %v", opts.Name, err)
		log.Warn("Please add '127.0.0.1 %s' to C:\\Windows\\System32\\drivers\\etc\\hosts manually", opts.Name)
	}

	// 4. Reload Nginx
	_ = ReloadNginx(cfg)

	log.Info("Site %s created successfully!", opts.Name)
	return site, nil
}

// Remove deletes a virtual host and updates hosts and Nginx safely.
func Remove(cfg *config.AppConfig, name string) error {
	log := logger.G()
	targetConf := filepath.Join(cfg.Paths.Conf, "vhosts", name+".conf")

	if _, err := os.Stat(targetConf); os.IsNotExist(err) {
		return fmt.Errorf("site %s does not exist", name)
	}

	backupConf := targetConf + ".bak"
	content, _ := os.ReadFile(targetConf)
	os.WriteFile(backupConf, content, 0644)
	os.Remove(targetConf)

	// Validate Nginx
	if err := TestNginxConfig(cfg); err != nil {
		log.Error("Nginx config test failed after removing site! Rolling back...")
		os.WriteFile(targetConf, content, 0644)
		os.Remove(backupConf)
		return fmt.Errorf("failed to remove site safely: %w", err)
	}
	os.Remove(backupConf)

	// Remove from hosts
	_ = hosts.RunElevatedHelper("remove", name)

	// Reload Nginx
	_ = ReloadNginx(cfg)

	log.Info("Site %s removed successfully", name)
	return nil
}

// List returns all configured virtual hosts.
func List(cfg *config.AppConfig) ([]string, error) {
	vhostDir := filepath.Join(cfg.Paths.Conf, "vhosts")
	files, err := filepath.Glob(filepath.Join(vhostDir, "*.conf"))
	if err != nil {
		return nil, err
	}

	var sites []string
	for _, f := range files {
		base := filepath.Base(f)
		sites = append(sites, strings.TrimSuffix(base, ".conf"))
	}
	return sites, nil
}

// RenderVhost renders the vhost template with the given site data.
func RenderVhost(tmplContent string, site *Site) (string, error) {
	tmpl, err := template.New("vhost").Parse(tmplContent)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, site); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// TestNginxConfig runs 'nginx -t' to verify the configuration syntax.
func TestNginxConfig(cfg *config.AppConfig) error {
	nginxExe := cfg.Paths.NginxBinPath()
	if _, err := os.Stat(nginxExe); os.IsNotExist(err) {
		return nil // skip if nginx is not yet installed (e.g. during dev tests)
	}

	confFwd := templates.ToForwardSlash(filepath.Join(cfg.Paths.Conf, "nginx.conf"))
	prefixFwd := templates.ToForwardSlash(cfg.Paths.NginxDir())

	cmd := exec.Command(nginxExe, "-t", "-c", confFwd, "-p", prefixFwd)
	cmd.Dir = cfg.Paths.NginxDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("syntax check failed: %w\n%s", err, string(out))
	}
	return nil
}

// ReloadNginx sends the reload signal (nginx -s reload).
func ReloadNginx(cfg *config.AppConfig) error {
	nginxExe := cfg.Paths.NginxBinPath()
	if _, err := os.Stat(nginxExe); os.IsNotExist(err) {
		return nil
	}

	confFwd := templates.ToForwardSlash(filepath.Join(cfg.Paths.Conf, "nginx.conf"))
	prefixFwd := templates.ToForwardSlash(cfg.Paths.NginxDir())

	cmd := exec.Command(nginxExe, "-s", "reload", "-c", confFwd, "-p", prefixFwd)
	cmd.Dir = cfg.Paths.NginxDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	return cmd.Run()
}
