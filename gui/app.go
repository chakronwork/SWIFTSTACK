// Package gui integrates the internal stack components with the desktop UI.
package gui

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/db"
	"github.com/chakronwork/webserver-stack/internal/logger"
	"github.com/chakronwork/webserver-stack/internal/service"
	"github.com/chakronwork/webserver-stack/internal/site"
)

// App struct provides bindings for the GUI window.
type App struct {
	ctx context.Context
	cfg *config.AppConfig
	mgr *service.Manager
}

// NewApp creates a new App application struct.
func NewApp(cfg *config.AppConfig, mgr *service.Manager) *App {
	return &App{
		cfg: cfg,
		mgr: mgr,
	}
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// GetServicesStatus returns running information for all services.
func (a *App) GetServicesStatus() map[string]service.Info {
	if a.mgr != nil {
		return a.mgr.Status()
	}
	return nil
}

// ControlService starts, stops, or restarts a service.
func (a *App) ControlService(name, action string) error {
	if a.mgr == nil {
		return fmt.Errorf("service manager not ready")
	}

	switch action {
	case "start":
		return a.mgr.Start(name)
	case "stop":
		return a.mgr.Stop(name)
	case "restart":
		return a.mgr.Restart(name)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

// StartAll starts all services.
func (a *App) StartAll() error {
	if a.mgr != nil {
		return a.mgr.StartAll()
	}
	return nil
}

// StopAll stops all services.
func (a *App) StopAll() error {
	if a.mgr != nil {
		return a.mgr.StopAll()
	}
	return nil
}

// GetDBPassword retrieves the current MariaDB root password.
func (a *App) GetDBPassword() (string, error) {
	pwd, _, err := db.GetOrCreateRootPassword(a.cfg.Paths.Data)
	return pwd, err
}

// ResetDBPassword regenerates the root password.
func (a *App) ResetDBPassword() (string, error) {
	newPwd, err := db.GenerateRandomPassword(24)
	if err != nil {
		return "", err
	}
	err = db.ResetRootPassword(a.cfg.Paths.Data, newPwd)
	return newPwd, err
}

// ListSites returns all registered virtual host domain names.
func (a *App) ListSites() ([]string, error) {
	return site.List(a.cfg)
}

// AddSite creates a new virtual host from the GUI.
func (a *App) AddSite(name, phpVersion string, https bool) error {
	_, err := site.Add(a.cfg, site.AddSiteOptions{
		Name:       name,
		PHPVersion: phpVersion,
		HTTPS:      https,
	})
	return err
}

// RemoveSite deletes an existing virtual host.
func (a *App) RemoveSite(name string) error {
	return site.Remove(a.cfg, name)
}

// OpenPathInExplorer opens a folder or file in Windows Explorer.
func (a *App) OpenPathInExplorer(subPath string) error {
	target := filepath.Join(a.cfg.Paths.Root, subPath)
	return exec.Command("explorer.exe", target).Start()
}

// Suppress unused imports
var _ = logger.G
