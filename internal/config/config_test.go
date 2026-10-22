package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temp directory with config files
	dir := t.TempDir()

	stackContent := `
[general]
profile = "dev"
language = "en"

[nginx]
port = 8080
ssl_port = 8443
bind = "127.0.0.1"

[php]
default_version = "8.4"
memory_limit = "128M"
upload_max_filesize = "32M"
post_max_size = "32M"
max_execution_time = 120
display_errors = true
xdebug = false

[mariadb]
port = 3307
bind = "127.0.0.1"

[phpmyadmin]
enabled = true
`
	versionsContent := `
[nginx]
version = "1.30.6"
url = "https://nginx.org/download/nginx-1.30.6.zip"
sha256 = "abc123"
strip = "nginx-1.30.6"

[php."8.4"]
version = "8.4.27"
url = "https://example.com/php.zip"
sha256 = "def456"
strip = ""

[mariadb]
version = "11.4.13"
url = "https://example.com/mariadb.zip"
sha256 = "ghi789"
strip = "mariadb-11.4.13-winx64"

[phpmyadmin]
version = "5.2.3"
url = "https://example.com/pma.zip"
sha256 = "jkl012"
strip = "phpMyAdmin-5.2.3-all-languages"

[composer]
version = "2.10.3"
url = "https://example.com/composer.phar"
sha256 = "mno345"
strip = ""

[mkcert]
version = "1.4.4"
url = "https://example.com/mkcert.exe"
sha256 = "pqr678"
strip = ""
`
	if err := os.WriteFile(filepath.Join(dir, "stack.toml"), []byte(stackContent), 0644); err != nil {
		t.Fatalf("failed to write stack.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions.toml"), []byte(versionsContent), 0644); err != nil {
		t.Fatalf("failed to write versions.toml: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Test stack config
	if cfg.Stack.Nginx.Port != 8080 {
		t.Errorf("expected nginx port 8080, got %d", cfg.Stack.Nginx.Port)
	}
	if cfg.Stack.Nginx.SSLPort != 8443 {
		t.Errorf("expected nginx ssl port 8443, got %d", cfg.Stack.Nginx.SSLPort)
	}
	if cfg.Stack.Nginx.Bind != "127.0.0.1" {
		t.Errorf("expected nginx bind 127.0.0.1, got %s", cfg.Stack.Nginx.Bind)
	}
	if cfg.Stack.PHP.DefaultVersion != "8.4" {
		t.Errorf("expected php default version 8.4, got %s", cfg.Stack.PHP.DefaultVersion)
	}
	if cfg.Stack.PHP.MemoryLimit != "128M" {
		t.Errorf("expected php memory_limit 128M, got %s", cfg.Stack.PHP.MemoryLimit)
	}
	if cfg.Stack.MariaDB.Port != 3307 {
		t.Errorf("expected mariadb port 3307, got %d", cfg.Stack.MariaDB.Port)
	}
	if !cfg.Stack.IsDevProfile() {
		t.Error("expected dev profile")
	}

	// Test versions config
	if cfg.Versions.Nginx.Version != "1.30.6" {
		t.Errorf("expected nginx version 1.30.6, got %s", cfg.Versions.Nginx.Version)
	}
	phpVer, ok := cfg.Versions.PHP["8.4"]
	if !ok {
		t.Fatal("PHP 8.4 version not found")
	}
	if phpVer.Version != "8.4.27" {
		t.Errorf("expected PHP version 8.4.27, got %s", phpVer.Version)
	}
	if cfg.Versions.MariaDB.Version != "11.4.13" {
		t.Errorf("expected MariaDB version 11.4.13, got %s", cfg.Versions.MariaDB.Version)
	}

	// Test paths
	if cfg.Paths.Root != dir {
		t.Errorf("expected root %s, got %s", dir, cfg.Paths.Root)
	}
}

func TestApplyDefaults(t *testing.T) {
	s := &StackConfig{}
	applyDefaults(s)

	if s.General.Profile != "dev" {
		t.Errorf("expected default profile 'dev', got '%s'", s.General.Profile)
	}
	if s.Nginx.Port != 80 {
		t.Errorf("expected default nginx port 80, got %d", s.Nginx.Port)
	}
	if s.MariaDB.Port != 3306 {
		t.Errorf("expected default mariadb port 3306, got %d", s.MariaDB.Port)
	}
	if s.PHP.MemoryLimit != "256M" {
		t.Errorf("expected default memory_limit 256M, got %s", s.PHP.MemoryLimit)
	}
}

func TestPHPFCGIPort(t *testing.T) {
	tests := []struct {
		version  string
		expected int
	}{
		{"8.4", 9084},
		{"8.5", 9085},
		{"8.3", 9083},
	}
	for _, tt := range tests {
		got := PHPFCGIPort(tt.version)
		if got != tt.expected {
			t.Errorf("PHPFCGIPort(%s) = %d, want %d", tt.version, got, tt.expected)
		}
	}
}

func TestResolvePaths(t *testing.T) {
	root := `C:\webserver`
	p := ResolvePaths(root)

	if p.Bin != filepath.Join(root, "bin") {
		t.Errorf("expected bin path %s, got %s", filepath.Join(root, "bin"), p.Bin)
	}
	if p.DataSQL != filepath.Join(root, "data", "mysql") {
		t.Errorf("expected data/mysql path, got %s", p.DataSQL)
	}
}

func TestEnsureDirectories(t *testing.T) {
	dir := t.TempDir()
	p := ResolvePaths(dir)

	if err := p.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}

	// Verify directories were created
	dirs := []string{p.Bin, p.Conf, p.WWW, p.Data, p.DataSQL, p.Logs, p.Tmp, p.Sessions, p.Backups, p.Licenses}
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %s was not created: %v", d, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}
