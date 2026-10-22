// Package service manages the lifecycle of Nginx, PHP-CGI, and MariaDB processes.
// Uses Windows Job Objects to ensure child processes are terminated when the manager exits.
package service

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/logger"
	"github.com/chakronwork/webserver-stack/internal/port"
	"github.com/chakronwork/webserver-stack/internal/templates"
)

// State represents the running state of a service.
type State int

const (
	StateStopped State = iota
	StateStarting
	StateRunning
	StateStopping
	StateError
)

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Info holds status information about a service.
type Info struct {
	Name      string
	State     State
	PID       int
	Port      int
	StartedAt time.Time
	Error     string
}

// Uptime returns how long the service has been running.
func (i Info) Uptime() time.Duration {
	if i.State != StateRunning || i.StartedAt.IsZero() {
		return 0
	}
	return time.Since(i.StartedAt)
}

// Service represents a managed service (nginx, php-cgi, mariadb).
type Service struct {
	name      string
	state     State
	cmd       *exec.Cmd
	pid       int
	port      int
	startedAt time.Time
	lastError string
	mu        sync.RWMutex
}

// Manager orchestrates all services and the Windows Job Object.
type Manager struct {
	cfg      *config.AppConfig
	services map[string]*Service
	jobObj   jobObject // Windows Job Object handle
	mu       sync.RWMutex
}

// NewManager creates a new service manager.
func NewManager(cfg *config.AppConfig) (*Manager, error) {
	m := &Manager{
		cfg:      cfg,
		services: make(map[string]*Service),
	}

	// Create Windows Job Object
	job, err := createJobObject()
	if err != nil {
		return nil, fmt.Errorf("cannot create Job Object: %w", err)
	}
	m.jobObj = job

	// Register services
	m.services["nginx"] = &Service{
		name: "nginx",
		port: cfg.Stack.Nginx.Port,
	}

	phpVersion := cfg.Stack.PHP.DefaultVersion
	m.services["php"] = &Service{
		name: "php",
		port: config.PHPFCGIPort(phpVersion),
	}

	m.services["mariadb"] = &Service{
		name:    "mariadb",
		port:    cfg.Stack.MariaDB.Port,
	}

	return m, nil
}

// Close releases the Job Object.
func (m *Manager) Close() error {
	return m.jobObj.Close()
}

// StartAll starts all services in order: MariaDB, PHP, Nginx.
func (m *Manager) StartAll() error {
	log := logger.G()
	log.Info("Starting all services...")

	// Check ports before starting
	if err := m.checkPorts(); err != nil {
		return err
	}

	// Start in dependency order
	order := []string{"mariadb", "php", "nginx"}
	for _, name := range order {
		if err := m.Start(name); err != nil {
			// Stop already-started services on failure
			log.Error("Failed to start %s, stopping already started services: %v", name, err)
			m.StopAll()
			return err
		}
	}

	log.Info("All services started successfully")
	return nil
}

// StopAll stops all services in reverse order: Nginx, PHP, MariaDB.
func (m *Manager) StopAll() error {
	log := logger.G()
	log.Info("Stopping all services...")

	order := []string{"nginx", "php", "mariadb"}
	var lastErr error
	for _, name := range order {
		if err := m.Stop(name); err != nil {
			log.Error("Failed to stop %s: %v", name, err)
			lastErr = err
		}
	}

	if lastErr != nil {
		return fmt.Errorf("errors occurred while stopping services: %w", lastErr)
	}

	log.Info("All services stopped")
	return nil
}

// Start starts a single service by name.
func (m *Manager) Start(name string) error {
	log := logger.G()
	svc, ok := m.services[name]
	if !ok {
		return fmt.Errorf("unknown service: %s", name)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.state == StateRunning {
		log.Info("%s is already running (PID %d)", name, svc.pid)
		return nil
	}

	svc.state = StateStarting
	log.Info("Starting %s...", name)

	var err error
	switch name {
	case "nginx":
		err = m.startNginx(svc)
	case "php":
		err = m.startPHP(svc)
	case "mariadb":
		err = m.startMariaDB(svc)
	default:
		err = fmt.Errorf("unknown service: %s", name)
	}

	if err != nil {
		svc.state = StateError
		svc.lastError = err.Error()
		return fmt.Errorf("failed to start %s: %w", name, err)
	}

	svc.state = StateRunning
	svc.startedAt = time.Now()
	log.Info("%s started (PID %d, port %d)", name, svc.pid, svc.port)
	return nil
}

// Stop stops a single service by name.
func (m *Manager) Stop(name string) error {
	log := logger.G()
	svc, ok := m.services[name]
	if !ok {
		return fmt.Errorf("unknown service: %s", name)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.state != StateRunning && svc.state != StateError {
		log.Info("%s is not running", name)
		return nil
	}

	svc.state = StateStopping
	log.Info("Stopping %s (PID %d)...", name, svc.pid)

	var err error
	switch name {
	case "nginx":
		err = m.stopNginx(svc)
	case "php":
		err = m.stopPHP(svc)
	case "mariadb":
		err = m.stopMariaDB(svc)
	}

	if err != nil {
		log.Warn("Graceful stop failed for %s, killing process: %v", name, err)
		if svc.cmd != nil && svc.cmd.Process != nil {
			svc.cmd.Process.Kill()
		}
	}

	svc.state = StateStopped
	svc.pid = 0
	svc.cmd = nil
	log.Info("%s stopped", name)
	return nil
}

// Restart stops and starts a service.
func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}
	return m.Start(name)
}

// Status returns status info for all services.
func (m *Manager) Status() map[string]Info {
	result := make(map[string]Info)
	for name, svc := range m.services {
		svc.mu.RLock()
		result[name] = Info{
			Name:      name,
			State:     svc.state,
			PID:       svc.pid,
			Port:      svc.port,
			StartedAt: svc.startedAt,
			Error:     svc.lastError,
		}
		svc.mu.RUnlock()
	}
	return result
}

// ServiceStatus returns status info for a single service.
func (m *Manager) ServiceStatus(name string) (Info, error) {
	svc, ok := m.services[name]
	if !ok {
		return Info{}, fmt.Errorf("unknown service: %s. Valid services: nginx, php, mariadb", name)
	}

	svc.mu.RLock()
	defer svc.mu.RUnlock()

	return Info{
		Name:      name,
		State:     svc.state,
		PID:       svc.pid,
		Port:      svc.port,
		StartedAt: svc.startedAt,
		Error:     svc.lastError,
	}, nil
}

// checkPorts verifies all required ports are available.
func (m *Manager) checkPorts() error {
	log := logger.G()
	portsToCheck := []struct {
		name string
		bind string
		port int
	}{
		{"nginx", m.cfg.Stack.Nginx.Bind, m.cfg.Stack.Nginx.Port},
		{"nginx-ssl", m.cfg.Stack.Nginx.Bind, m.cfg.Stack.Nginx.SSLPort},
		{"php-fcgi", "127.0.0.1", config.PHPFCGIPort(m.cfg.Stack.PHP.DefaultVersion)},
		{"mariadb", m.cfg.Stack.MariaDB.Bind, m.cfg.Stack.MariaDB.Port},
	}

	var errors []string
	for _, p := range portsToCheck {
		result := port.Check(p.bind, p.port)
		if !result.Available {
			alt := port.SuggestAlternative(p.bind, p.port)
			suggestion := ""
			if alt > 0 {
				suggestion = fmt.Sprintf(" Suggested alternative: %d.", alt)
			}
			msg := fmt.Sprintf("  %s: %s%s", p.name, result.Error, suggestion)
			errors = append(errors, msg)
			log.Error(msg)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("port conflicts detected:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// startNginx starts the nginx process.
func (m *Manager) startNginx(svc *Service) error {
	nginxExe := m.cfg.Paths.NginxBinPath()
	if _, err := os.Stat(nginxExe); err != nil {
		return fmt.Errorf("nginx not found at %s. Run 'webserver init' first", nginxExe)
	}

	confPath := filepath.Join(m.cfg.Paths.Conf, "nginx.conf")
	if _, err := os.Stat(confPath); err != nil {
		return fmt.Errorf("nginx.conf not found at %s. Run 'webserver init' first", confPath)
	}

	// Ensure nginx temp directories exist
	tempDirs := []string{"client_body", "proxy", "fastcgi", "uwsgi", "scgi"}
	for _, d := range tempDirs {
		os.MkdirAll(filepath.Join(m.cfg.Paths.Tmp, d), 0755)
	}

	// Use forward slashes for nginx paths
	confFwd := templates.ToForwardSlash(confPath)
	prefixFwd := templates.ToForwardSlash(m.cfg.Paths.NginxDir())

	cmd := exec.Command(nginxExe, "-c", confFwd, "-p", prefixFwd)
	cmd.Dir = m.cfg.Paths.NginxDir()
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := m.startProcess(cmd); err != nil {
		return err
	}

	svc.cmd = cmd
	svc.pid = cmd.Process.Pid

	// Health check: wait for HTTP response
	if err := m.healthCheckHTTP(m.cfg.Stack.Nginx.Bind, m.cfg.Stack.Nginx.Port, 10*time.Second); err != nil {
		return fmt.Errorf("nginx started but health check failed: %w", err)
	}

	return nil
}

// stopNginx gracefully stops nginx using nginx -s quit.
func (m *Manager) stopNginx(svc *Service) error {
	nginxExe := m.cfg.Paths.NginxBinPath()
	confFwd := templates.ToForwardSlash(filepath.Join(m.cfg.Paths.Conf, "nginx.conf"))
	prefixFwd := templates.ToForwardSlash(m.cfg.Paths.NginxDir())

	cmd := exec.Command(nginxExe, "-s", "quit", "-c", confFwd, "-p", prefixFwd)
	cmd.Dir = m.cfg.Paths.NginxDir()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nginx -s quit failed: %w", err)
	}

	// Wait for the process to actually exit
	if svc.cmd != nil && svc.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- svc.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			return fmt.Errorf("nginx did not exit within 5 seconds after quit signal")
		}
	}

	return nil
}

// startPHP starts php-cgi as a FastCGI server.
func (m *Manager) startPHP(svc *Service) error {
	phpVersion := m.cfg.Stack.PHP.DefaultVersion
	phpExe := m.cfg.Paths.PHPBinPath(phpVersion)
	if _, err := os.Stat(phpExe); err != nil {
		return fmt.Errorf("php-cgi not found at %s. Run 'webserver init' first", phpExe)
	}

	phpIni := filepath.Join(m.cfg.Paths.Conf, "php.ini")

	fcgiPort := config.PHPFCGIPort(phpVersion)
	bindAddr := fmt.Sprintf("127.0.0.1:%d", fcgiPort)

	cmd := exec.Command(phpExe, "-b", bindAddr, "-c", phpIni)
	cmd.Dir = m.cfg.Paths.PHPDir(phpVersion)
	cmd.Env = append(os.Environ(),
		"PHP_FCGI_MAX_REQUESTS=10000",
		"PHP_FCGI_CHILDREN=4",
	)

	if err := m.startProcess(cmd); err != nil {
		return err
	}

	svc.cmd = cmd
	svc.pid = cmd.Process.Pid
	svc.port = fcgiPort

	// Health check: wait for TCP port
	if err := port.WaitForPort("127.0.0.1", fcgiPort, 10*time.Second); err != nil {
		return fmt.Errorf("php-cgi started but port %d not available: %w", fcgiPort, err)
	}

	return nil
}

// stopPHP terminates the php-cgi process.
func (m *Manager) stopPHP(svc *Service) error {
	// php-cgi has no graceful shutdown mechanism; just kill it
	if svc.cmd != nil && svc.cmd.Process != nil {
		return svc.cmd.Process.Kill()
	}
	return nil
}

// startMariaDB starts mariadbd.
func (m *Manager) startMariaDB(svc *Service) error {
	mariadbExe := filepath.Join(m.cfg.Paths.MariaDBBinDir(), "mariadbd.exe")
	if _, err := os.Stat(mariadbExe); err != nil {
		mariadbExe = filepath.Join(m.cfg.Paths.MariaDBBinDir(), "mysqld.exe")
		if _, err := os.Stat(mariadbExe); err != nil {
			return fmt.Errorf("MariaDB binary not found. Please click '⚡ Download / Init Stack' on dashboard or run 'webserver init' first")
		}
	}

	myIni := filepath.Join(m.cfg.Paths.Conf, "my.ini")
	if _, err := os.Stat(myIni); err != nil {
		return fmt.Errorf("my.ini not found at %s. Run 'webserver init' first", myIni)
	}

	cmd := exec.Command(mariadbExe, fmt.Sprintf("--defaults-file=%s", myIni), "--console")
	cmd.Dir = m.cfg.Paths.MariaDBBinDir()

	if err := m.startProcess(cmd); err != nil {
		return err
	}

	svc.cmd = cmd
	svc.pid = cmd.Process.Pid

	// Health check: wait for TCP port
	if err := port.WaitForPort(m.cfg.Stack.MariaDB.Bind, m.cfg.Stack.MariaDB.Port, 30*time.Second); err != nil {
		return fmt.Errorf("mariadb started but port %d not available: %w", m.cfg.Stack.MariaDB.Port, err)
	}

	return nil
}

// stopMariaDB gracefully stops MariaDB using mariadb-admin shutdown.
func (m *Manager) stopMariaDB(svc *Service) error {
	adminExe := filepath.Join(m.cfg.Paths.MariaDBBinDir(), "mariadb-admin.exe")
	if _, err := os.Stat(adminExe); err != nil {
		// Fall back to mysqladmin.exe
		adminExe = filepath.Join(m.cfg.Paths.MariaDBBinDir(), "mysqladmin.exe")
	}

	// Read the root password from secrets
	password := readSecret(m.cfg.Paths.Data)

	args := []string{"-u", "root", "shutdown"}
	if password != "" {
		args = []string{"-u", "root", fmt.Sprintf("-p%s", password), "shutdown"}
	}

	cmd := exec.Command(adminExe, args...)
	cmd.Dir = m.cfg.Paths.MariaDBBinDir()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mariadb-admin shutdown failed: %w", err)
	}

	// Wait for process to exit
	if svc.cmd != nil && svc.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- svc.cmd.Wait()
		}()

		select {
		case <-done:
		case <-time.After(15 * time.Second):
			return fmt.Errorf("mariadb did not exit within 15 seconds after shutdown")
		}
	}

	return nil
}

// startProcess starts a process and assigns it to the Job Object.
func (m *Manager) startProcess(cmd *exec.Cmd) error {
	// Set process creation flags for Windows (CREATE_SUSPENDED not needed,
	// but we do need CREATE_NEW_PROCESS_GROUP to prevent Ctrl+C propagation)
	setProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start process: %w", err)
	}

	// Assign to Job Object
	if err := m.jobObj.Assign(cmd.Process); err != nil {
		logger.G().Warn("Cannot assign PID %d to Job Object: %v (orphan protection may not work)", cmd.Process.Pid, err)
	}

	return nil
}

// healthCheckHTTP performs an HTTP health check.
func (m *Manager) healthCheckHTTP(bind string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://%s:%d/", bind, port)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}

	return fmt.Errorf("HTTP health check failed for %s after %v", url, timeout)
}

// healthCheckTCP performs a TCP health check.
func (m *Manager) healthCheckTCP(bind string, portNum int, timeout time.Duration) error {
	return port.WaitForPort(bind, portNum, timeout)
}

// readSecret reads the MariaDB root password from data/.secrets.
func readSecret(dataDir string) string {
	secretFile := filepath.Join(dataDir, ".secrets")
	data, err := os.ReadFile(secretFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MARIADB_ROOT_PASSWORD=") {
			return strings.TrimPrefix(line, "MARIADB_ROOT_PASSWORD=")
		}
	}
	return ""
}

// Suppress unused import warning for net package
var _ = net.Dial
