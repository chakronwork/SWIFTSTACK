// Package config loads and manages the stack.toml and versions.toml configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
)

// StackConfig represents the main stack.toml configuration.
type StackConfig struct {
	General    GeneralConfig    `toml:"general"`
	Nginx      NginxConfig      `toml:"nginx"`
	PHP        PHPConfig        `toml:"php"`
	MariaDB    MariaDBConfig    `toml:"mariadb"`
	PhpMyAdmin PhpMyAdminConfig `toml:"phpmyadmin"`
}

// GeneralConfig holds general settings.
type GeneralConfig struct {
	Profile  string `toml:"profile"`
	Language string `toml:"language"`
}

// NginxConfig holds nginx-specific settings.
type NginxConfig struct {
	Port    int    `toml:"port"`
	SSLPort int    `toml:"ssl_port"`
	Bind    string `toml:"bind"`
}

// PHPConfig holds PHP-specific settings.
type PHPConfig struct {
	DefaultVersion   string `toml:"default_version"`
	MemoryLimit      string `toml:"memory_limit"`
	UploadMaxFilesize string `toml:"upload_max_filesize"`
	PostMaxSize      string `toml:"post_max_size"`
	MaxExecutionTime int    `toml:"max_execution_time"`
	DisplayErrors    bool   `toml:"display_errors"`
	Xdebug           bool   `toml:"xdebug"`
}

// MariaDBConfig holds MariaDB-specific settings.
type MariaDBConfig struct {
	Port int    `toml:"port"`
	Bind string `toml:"bind"`
}

// PhpMyAdminConfig holds phpMyAdmin-specific settings.
type PhpMyAdminConfig struct {
	Enabled bool `toml:"enabled"`
}

// VersionsConfig represents the versions.toml file.
type VersionsConfig struct {
	Nginx      ComponentVersion            `toml:"nginx"`
	PHP        map[string]ComponentVersion `toml:"php"`
	MariaDB    ComponentVersion            `toml:"mariadb"`
	PhpMyAdmin ComponentVersion            `toml:"phpmyadmin"`
	Composer   ComponentVersion            `toml:"composer"`
	Mkcert     ComponentVersion            `toml:"mkcert"`
}

// ComponentVersion describes a downloadable component.
type ComponentVersion struct {
	Version string `toml:"version"`
	URL     string `toml:"url"`
	SHA256  string `toml:"sha256"`
	Strip   string `toml:"strip"`
}

// Paths holds all resolved directory paths for the installation.
type Paths struct {
	Root     string // Install directory (where webserver.exe lives)
	Bin      string // bin/
	Conf     string // conf/
	WWW      string // www/
	Data     string // data/
	DataSQL  string // data/mysql/
	Logs     string // logs/
	Tmp      string // tmp/
	Sessions string // sessions/
	Backups  string // backups/
	Licenses string // licenses/
}

var (
	globalConfig *AppConfig
	configMu     sync.RWMutex
)

// AppConfig combines all configuration and paths.
type AppConfig struct {
	Stack    StackConfig
	Versions VersionsConfig
	Paths    Paths
}

// ResolvePaths computes all paths relative to the given root directory.
func ResolvePaths(root string) Paths {
	return Paths{
		Root:     root,
		Bin:      filepath.Join(root, "bin"),
		Conf:     filepath.Join(root, "conf"),
		WWW:      filepath.Join(root, "www"),
		Data:     filepath.Join(root, "data"),
		DataSQL:  filepath.Join(root, "data", "mysql"),
		Logs:     filepath.Join(root, "logs"),
		Tmp:      filepath.Join(root, "tmp"),
		Sessions: filepath.Join(root, "sessions"),
		Backups:  filepath.Join(root, "backups"),
		Licenses: filepath.Join(root, "licenses"),
	}
}

// EnsureDirectories creates all required directories if they don't exist.
func (p *Paths) EnsureDirectories() error {
	dirs := []string{
		p.Bin, p.Conf, p.WWW, p.Data, p.DataSQL,
		p.Logs, p.Tmp, p.Sessions, p.Backups, p.Licenses,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create directory %s: %w", dir, err)
		}
	}
	return nil
}

// Load reads stack.toml and versions.toml from the given root directory.
func Load(root string) (*AppConfig, error) {
	cfg := &AppConfig{
		Paths: ResolvePaths(root),
	}

	// Load stack.toml
	stackPath := filepath.Join(root, "stack.toml")
	if _, err := os.Stat(stackPath); err != nil {
		return nil, fmt.Errorf("stack.toml not found in %s: %w", root, err)
	}
	if _, err := toml.DecodeFile(stackPath, &cfg.Stack); err != nil {
		return nil, fmt.Errorf("failed to parse stack.toml: %w", err)
	}

	// Apply defaults
	applyDefaults(&cfg.Stack)

	// Load versions.toml
	versionsPath := filepath.Join(root, "versions.toml")
	if _, err := os.Stat(versionsPath); err != nil {
		return nil, fmt.Errorf("versions.toml not found in %s: %w", root, err)
	}
	if _, err := toml.DecodeFile(versionsPath, &cfg.Versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions.toml: %w", err)
	}

	configMu.Lock()
	globalConfig = cfg
	configMu.Unlock()

	return cfg, nil
}

// Get returns the globally loaded configuration. Panics if Load has not been called.
func Get() *AppConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	if globalConfig == nil {
		panic("config.Load() must be called before config.Get()")
	}
	return globalConfig
}

// applyDefaults sets missing values to sensible defaults.
func applyDefaults(s *StackConfig) {
	if s.General.Profile == "" {
		s.General.Profile = "dev"
	}
	if s.General.Language == "" {
		s.General.Language = "en"
	}
	if s.Nginx.Port == 0 {
		s.Nginx.Port = 80
	}
	if s.Nginx.SSLPort == 0 {
		s.Nginx.SSLPort = 443
	}
	if s.Nginx.Bind == "" {
		s.Nginx.Bind = "127.0.0.1"
	}
	if s.PHP.DefaultVersion == "" {
		s.PHP.DefaultVersion = "8.4"
	}
	if s.PHP.MemoryLimit == "" {
		s.PHP.MemoryLimit = "256M"
	}
	if s.PHP.UploadMaxFilesize == "" {
		s.PHP.UploadMaxFilesize = "64M"
	}
	if s.PHP.PostMaxSize == "" {
		s.PHP.PostMaxSize = "64M"
	}
	if s.PHP.MaxExecutionTime == 0 {
		s.PHP.MaxExecutionTime = 300
	}
	if s.MariaDB.Port == 0 {
		s.MariaDB.Port = 3306
	}
	if s.MariaDB.Bind == "" {
		s.MariaDB.Bind = "127.0.0.1"
	}
}

// IsDevProfile returns true if the current profile is "dev".
func (s *StackConfig) IsDevProfile() bool {
	return s.General.Profile == "dev"
}

// PHPFCGIPort returns the FastCGI port for a given PHP version string like "8.4".
// Convention: 9084 for 8.4, 9085 for 8.5, etc.
func PHPFCGIPort(version string) int {
	// Parse minor version from string like "8.4"
	var major, minor int
	fmt.Sscanf(version, "%d.%d", &major, &minor)
	return 9000 + major*10 + minor
}

// NginxBinPath returns the path to nginx.exe.
func (p *Paths) NginxBinPath() string {
	return filepath.Join(p.Bin, "nginx", "nginx.exe")
}

// NginxDir returns the nginx directory (needed as prefix for nginx -p).
func (p *Paths) NginxDir() string {
	return filepath.Join(p.Bin, "nginx")
}

// PHPBinPath returns the path to php-cgi.exe for a given version.
func (p *Paths) PHPBinPath(version string) string {
	return filepath.Join(p.Bin, "php", version, "php-cgi.exe")
}

// PHPDir returns the PHP directory for a given version.
func (p *Paths) PHPDir(version string) string {
	return filepath.Join(p.Bin, "php", version)
}

// MariaDBBinDir returns the path to the MariaDB bin directory.
func (p *Paths) MariaDBBinDir() string {
	return filepath.Join(p.Bin, "mariadb", "bin")
}

// MariaDBDataDir returns the path to the MariaDB data directory.
func (p *Paths) MariaDBDataDir() string {
	return filepath.Join(p.Data, "mysql")
}

// ComposerPath returns the path to composer.phar.
func (p *Paths) ComposerPath() string {
	return filepath.Join(p.Bin, "composer", "composer.phar")
}

// MkcertPath returns the path to mkcert.exe.
func (p *Paths) MkcertPath() string {
	return filepath.Join(p.Bin, "mkcert", "mkcert.exe")
}
