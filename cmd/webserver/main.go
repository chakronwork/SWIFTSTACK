package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chakronwork/webserver-stack/gui"
	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/db"
	"github.com/chakronwork/webserver-stack/internal/doctor"
	"github.com/chakronwork/webserver-stack/internal/download"
	"github.com/chakronwork/webserver-stack/internal/logger"
	"github.com/chakronwork/webserver-stack/internal/service"
	"github.com/chakronwork/webserver-stack/internal/site"
	"github.com/chakronwork/webserver-stack/internal/templates"
	"github.com/urfave/cli/v2"
)

const appVersion = "0.1.0-alpha"

func getAppDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return ".", err
	}
	return filepath.Dir(exe), nil
}

func main() {
	app := &cli.App{
		Name:    "webserver",
		Usage:   "Portable, XAMPP-like local PHP stack for Windows",
		Version: appVersion,
		Action: func(c *cli.Context) error {
			// Default when launched without flags/subcommands: open GUI Dashboard!
			return runGUI()
		},
		Commands: []*cli.Command{
			{
				Name:  "gui",
				Usage: "Open the web-based Control Dashboard",
				Action: func(c *cli.Context) error {
					return runGUI()
				},
			},
			{
				Name:  "init",
				Usage: "Download, verify components, generate configs, and prepare directory structure",
				Action: func(c *cli.Context) error {
					return runInit()
				},
			},
			{
				Name:      "start",
				Usage:     "Start all services or a specific service [nginx|php|mariadb]",
				ArgsUsage: "[service]",
				Action: func(c *cli.Context) error {
					return runStart(c.Args().First())
				},
			},
			{
				Name:      "stop",
				Usage:     "Stop all services or a specific service [nginx|php|mariadb]",
				ArgsUsage: "[service]",
				Action: func(c *cli.Context) error {
					return runStop(c.Args().First())
				},
			},
			{
				Name:      "restart",
				Usage:     "Restart all services or a specific service [nginx|php|mariadb]",
				ArgsUsage: "[service]",
				Action: func(c *cli.Context) error {
					return runRestart(c.Args().First())
				},
			},
			{
				Name:      "status",
				Usage:     "Show current status of all services",
				ArgsUsage: "[service]",
				Action: func(c *cli.Context) error {
					return runStatus(c.Args().First())
				},
			},
			{
				Name:  "site",
				Usage: "Manage virtual hosts (add, list, remove)",
				Subcommands: []*cli.Command{
					{
						Name:      "add",
						Usage:     "Add a new virtual host (e.g. webserver site add myapp.test --https)",
						ArgsUsage: "<name.test>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "php",
								Value: "8.4",
								Usage: "PHP version for this site (e.g. 8.4, 8.5)",
							},
							&cli.StringFlag{
								Name:  "root",
								Usage: "Document root path (defaults to www/<name>)",
							},
							&cli.BoolFlag{
								Name:  "https",
								Usage: "Enable HTTPS with local trusted mkcert SSL certificate",
							},
						},
						Action: func(c *cli.Context) error {
							return runSiteAdd(c.Args().First(), c.String("php"), c.String("root"), c.Bool("https"))
						},
					},
					{
						Name:  "list",
						Usage: "List all configured virtual hosts",
						Action: func(c *cli.Context) error {
							return runSiteList()
						},
					},
					{
						Name:      "remove",
						Usage:     "Remove a virtual host",
						ArgsUsage: "<name>",
						Action: func(c *cli.Context) error {
							return runSiteRemove(c.Args().First())
						},
					},
				},
			},
			{
				Name:  "doctor",
				Usage: "Diagnose ports, VC++ runtime, permissions, WebView2, and system health",
				Action: func(c *cli.Context) error {
					return runDoctor()
				},
			},
			{
				Name:  "db",
				Usage: "Database operations (password show/reset, backup, restore)",
				Subcommands: []*cli.Command{
					{
						Name:  "password",
						Usage: "Manage MariaDB root password [show|reset]",
						Subcommands: []*cli.Command{
							{
								Name:  "show",
								Usage: "Show current root password from .secrets",
								Action: func(c *cli.Context) error {
									return runDBPasswordShow()
								},
							},
							{
								Name:  "reset",
								Usage: "Reset MariaDB root password to a new random value",
								Action: func(c *cli.Context) error {
									return runDBPasswordReset()
								},
							},
						},
					},
				},
			},
			{
				Name:      "logs",
				Usage:     "Display service logs",
				ArgsUsage: "<service>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "f",
						Aliases: []string{"follow"},
						Usage:   "Follow log output",
					},
				},
				Action: func(c *cli.Context) error {
					return runLogs(c.Args().First(), c.Bool("f"))
				},
			},
			{
				Name:  "version",
				Usage: "Display version information",
				Action: func(c *cli.Context) error {
					fmt.Printf("webserver version %s\n", appVersion)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupConfig() (*config.AppConfig, error) {
	appDir, err := getAppDir()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(filepath.Join(appDir, "stack.toml")); err != nil {
		cwd, _ := os.Getwd()
		if _, err := os.Stat(filepath.Join(cwd, "stack.toml")); err == nil {
			appDir = cwd
		}
	}

	cfg, err := config.Load(appDir)
	if err != nil {
		return nil, err
	}

	if err := cfg.Paths.EnsureDirectories(); err != nil {
		return nil, err
	}

	if err := logger.Init(cfg.Paths.Logs, logger.LevelInfo); err != nil {
		logger.InitStdout(logger.LevelInfo)
	}

	return cfg, nil
}

func runGUI() error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}
	defer mgr.Close()

	return gui.StartDashboardServer(cfg, mgr, runInit)
}

func runInit() error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	log := logger.G()
	log.Info("Initializing webserver stack in %s ...", cfg.Paths.Root)

	// Ensure vhosts directory
	vhostsDir := filepath.Join(cfg.Paths.Conf, "vhosts")
	os.MkdirAll(vhostsDir, 0755)

	// Ensure welcome page in www/
	defaultIndex := filepath.Join(cfg.Paths.WWW, "index.php")
	if _, err := os.Stat(defaultIndex); os.IsNotExist(err) {
		welcomePage := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Welcome to webserver</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; }
        .card { background: #1e293b; padding: 2rem 3rem; border-radius: 12px; box-shadow: 0 10px 25px rgba(0,0,0,0.5); text-align: center; max-width: 500px; }
        h1 { color: #38bdf8; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { display: inline-block; padding: 0.25rem 0.75rem; background: #0284c7; color: white; border-radius: 9999px; font-weight: bold; margin-bottom: 1rem; }
        a { color: #38bdf8; text-decoration: none; font-weight: 500; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">PHP <?php echo PHP_VERSION; ?></div>
        <h1>webserver is running!</h1>
        <p>Portable, lightweight & secure local PHP stack for Windows.</p>
        <p><a href="phpinfo.php">View phpinfo()</a></p>
    </div>
</body>
</html>`
		os.WriteFile(defaultIndex, []byte(welcomePage), 0644)
		os.WriteFile(filepath.Join(cfg.Paths.WWW, "phpinfo.php"), []byte("<?php phpinfo(); ?>"), 0644)
	}

	// 1. Download & Extract Nginx
	log.Info("Step 1/5: Checking Nginx...")
	nginxZip, err := download.DownloadAndVerify(
		cfg.Versions.Nginx.URL,
		cfg.Versions.Nginx.SHA256,
		cfg.Paths.Tmp,
		"nginx.zip",
	)
	if err != nil {
		return fmt.Errorf("failed downloading Nginx: %w", err)
	}
	if err := download.ExtractZip(nginxZip, cfg.Paths.NginxDir(), cfg.Versions.Nginx.Strip); err != nil {
		return fmt.Errorf("failed extracting Nginx: %w", err)
	}

	// 2. Download & Extract PHP 8.4
	log.Info("Step 2/6: Checking PHP 8.4...")
	phpVer, ok := cfg.Versions.PHP["8.4"]
	if !ok {
		return fmt.Errorf("PHP 8.4 version info not found in versions.toml")
	}
	phpZip, err := download.DownloadAndVerify(
		phpVer.URL,
		phpVer.SHA256,
		cfg.Paths.Tmp,
		"php-8.4.zip",
	)
	if err != nil {
		return fmt.Errorf("failed downloading PHP 8.4: %w", err)
	}
	if err := download.ExtractZip(phpZip, cfg.Paths.PHPDir("8.4"), phpVer.Strip); err != nil {
		return fmt.Errorf("failed extracting PHP 8.4: %w", err)
	}

	// 3. Download & Extract MariaDB
	log.Info("Step 3/6: Checking MariaDB 11.4 LTS...")
	mariadbZip, err := download.DownloadAndVerify(
		cfg.Versions.MariaDB.URL,
		cfg.Versions.MariaDB.SHA256,
		cfg.Paths.Tmp,
		"mariadb.zip",
	)
	if err != nil {
		return fmt.Errorf("failed downloading MariaDB: %w", err)
	}
	if err := download.ExtractZip(mariadbZip, filepath.Join(cfg.Paths.Bin, "mariadb"), cfg.Versions.MariaDB.Strip); err != nil {
		return fmt.Errorf("failed extracting MariaDB: %w", err)
	}

	// 4. Download & Extract phpMyAdmin (if enabled)
	if cfg.Stack.PhpMyAdmin.Enabled {
		log.Info("Step 4/6: Checking phpMyAdmin...")
		pmaZip, err := download.DownloadAndVerify(
			cfg.Versions.PhpMyAdmin.URL,
			cfg.Versions.PhpMyAdmin.SHA256,
			cfg.Paths.Tmp,
			"phpmyadmin.zip",
		)
		if err == nil {
			pmaDest := filepath.Join(cfg.Paths.WWW, "phpmyadmin")
			_ = download.ExtractZip(pmaZip, pmaDest, cfg.Versions.PhpMyAdmin.Strip)
		}
	}

	// 5. Download mkcert
	log.Info("Step 5/6: Checking mkcert...")
	mkcertPath, err := download.DownloadAndVerify(
		cfg.Versions.Mkcert.URL,
		cfg.Versions.Mkcert.SHA256,
		cfg.Paths.Tmp,
		"mkcert.exe",
	)
	if err == nil {
		_ = download.CopySingleFile(mkcertPath, filepath.Join(cfg.Paths.Bin, "mkcert"), "mkcert.exe")
	}

	// 6. Render service configurations
	log.Info("Step 6/6: Generating configuration files...")
	tmplDir := filepath.Join(cfg.Paths.Root, "templates")
	if _, err := os.Stat(tmplDir); err == nil {
		err = templates.RenderAll(cfg, tmplDir)
	} else {
		err = templates.RenderAllEmbedded(cfg)
	}
	if err != nil {
		return fmt.Errorf("failed rendering configs: %w", err)
	}

	// Copy nginx mime.types if missing in conf/
	origMime := filepath.Join(cfg.Paths.NginxDir(), "conf", "mime.types")
	targetMime := filepath.Join(cfg.Paths.Conf, "mime.types")
	if _, err := os.Stat(targetMime); os.IsNotExist(err) {
		if content, err := os.ReadFile(origMime); err == nil {
			os.WriteFile(targetMime, content, 0644)
		}
	}
	origFastcgi := filepath.Join(cfg.Paths.NginxDir(), "conf", "fastcgi_params")
	targetFastcgi := filepath.Join(cfg.Paths.Conf, "fastcgi_params")
	if _, err := os.Stat(targetFastcgi); os.IsNotExist(err) {
		if content, err := os.ReadFile(origFastcgi); err == nil {
			os.WriteFile(targetFastcgi, content, 0644)
		}
	}

	// 4. Initialize Database and secure root credentials
	log.Info("Step 4/5: Initializing database & security...")
	if err := db.EnsureSecureGitignore(cfg.Paths.Data); err != nil {
		log.Warn("Failed creating .gitignore in data/: %v", err)
	}

	// Bootstrap MariaDB if binary is present
	if _, err := os.Stat(filepath.Join(cfg.Paths.MariaDBBinDir(), "mariadb-install-db.exe")); err == nil {
		if err := db.InitializeDatabase(cfg); err != nil {
			log.Warn("Database init warning: %v", err)
		}
	} else {
		pwd, isNew, _ := db.GetOrCreateRootPassword(cfg.Paths.Data)
		if isNew {
			log.Info("Generated secure MariaDB root password (saved in data/.secrets)")
		}
		_ = pwd
	}

	log.Info("Step 5/5: Finalizing configuration...")
	log.Info("Stack initialized successfully! Run 'webserver start' to start the stack.")
	return nil
}

func runStart(svcName string) error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}
	defer mgr.Close()

	if svcName != "" {
		return mgr.Start(svcName)
	}

	if err := mgr.StartAll(); err != nil {
		return err
	}

	fmt.Printf("\nWebserver is running! Access your site at: http://%s:%d/\n", cfg.Stack.Nginx.Bind, cfg.Stack.Nginx.Port)
	fmt.Println("Press Ctrl+C to stop services and exit...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nStopping all services...")
	return mgr.StopAll()
}

func runStop(svcName string) error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}
	defer mgr.Close()

	if svcName != "" {
		return mgr.Stop(svcName)
	}
	return mgr.StopAll()
}

func runRestart(svcName string) error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}
	defer mgr.Close()

	if svcName != "" {
		return mgr.Restart(svcName)
	}

	if err := mgr.StopAll(); err != nil {
		return err
	}
	return mgr.StartAll()
}

func runStatus(svcName string) error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	mgr, err := service.NewManager(cfg)
	if err != nil {
		return err
	}
	defer mgr.Close()

	if svcName != "" {
		info, err := mgr.ServiceStatus(svcName)
		if err != nil {
			return err
		}
		printServiceInfo(info)
		return nil
	}

	statuses := mgr.Status()
	fmt.Printf("%-12s %-10s %-8s %-8s %s\n", "SERVICE", "STATUS", "PID", "PORT", "ERROR")
	fmt.Println(strings.Repeat("-", 50))
	for _, name := range []string{"nginx", "php", "mariadb"} {
		info := statuses[name]
		pidStr := "-"
		if info.PID > 0 {
			pidStr = fmt.Sprintf("%d", info.PID)
		}
		fmt.Printf("%-12s %-10s %-8s %-8d %s\n", info.Name, info.State.String(), pidStr, info.Port, info.Error)
	}
	return nil
}

func printServiceInfo(info service.Info) {
	fmt.Printf("Service: %s\n", info.Name)
	fmt.Printf("State:   %s\n", info.State.String())
	fmt.Printf("PID:     %d\n", info.PID)
	fmt.Printf("Port:    %d\n", info.Port)
	if info.Error != "" {
		fmt.Printf("Error:   %s\n", info.Error)
	}
}

func runLogs(svcName string, follow bool) error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	if svcName == "" {
		svcName = "webserver"
	}

	var logFile string
	switch svcName {
	case "nginx":
		logFile = filepath.Join(cfg.Paths.Logs, "nginx-error.log")
	case "nginx-access":
		logFile = filepath.Join(cfg.Paths.Logs, "nginx-access.log")
	case "php":
		logFile = filepath.Join(cfg.Paths.Logs, "php-error.log")
	case "mariadb":
		logFile = filepath.Join(cfg.Paths.Logs, "mariadb-error.log")
	default:
		logFile = filepath.Join(cfg.Paths.Logs, svcName+".log")
	}

	file, err := os.Open(logFile)
	if err != nil {
		return fmt.Errorf("cannot open log file %s: %w", logFile, err)
	}
	defer file.Close()

	if !follow {
		_, err = io.Copy(os.Stdout, file)
		return err
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			break
		}
		fmt.Print(line)
	}
	return nil
}

func runDoctor() error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	fmt.Println("Diagnosing Webserver Environment...")
	fmt.Println(strings.Repeat("=", 60))

	report := doctor.RunDiagnostics(cfg)
	for _, it := range report.Items {
		icon := "[OK]"
		if !it.Passed {
			if it.Critical {
				icon = "[FAIL]"
			} else {
				icon = "[WARN]"
			}
		}

		fmt.Printf("%-7s %s\n", icon, it.Name)
		fmt.Printf("        %s\n", it.Message)
		if it.Remediation != "" && !it.Passed {
			fmt.Printf("        -> Action: %s\n", it.Remediation)
		}
	}
	fmt.Println(strings.Repeat("=", 60))

	if report.AllPass {
		fmt.Println("Result: All critical checks passed! System is ready to run.")
	} else {
		fmt.Println("Result: Some critical issues were found. Please resolve actions above.")
	}
	return nil
}

func runDBPasswordShow() error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	pwd, isNew, err := db.GetOrCreateRootPassword(cfg.Paths.Data)
	if err != nil {
		return fmt.Errorf("could not read database secrets: %w", err)
	}

	if isNew {
		fmt.Println("A new root password was generated.")
	}
	fmt.Printf("MariaDB Root Password: %s\n", pwd)
	fmt.Println("(Stored locally in data/.secrets - keep confidential)")
	return nil
}

func runDBPasswordReset() error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	newPwd, err := db.GenerateRandomPassword(24)
	if err != nil {
		return err
	}

	if err := db.ResetRootPassword(cfg.Paths.Data, newPwd); err != nil {
		return fmt.Errorf("failed to update .secrets: %w", err)
	}

	fmt.Printf("MariaDB Root Password reset successfully to: %s\n", newPwd)
	fmt.Println("Note: If MariaDB is currently running, restart it to ensure all internal caches align.")
	return nil
}

func runSiteAdd(name, phpVer, root string, https bool) error {
	if name == "" {
		return fmt.Errorf("site domain name is required. Example: webserver site add myapp.test")
	}

	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	s, err := site.Add(cfg, site.AddSiteOptions{
		Name:       name,
		PHPVersion: phpVer,
		Root:       root,
		HTTPS:      https,
	})
	if err != nil {
		return err
	}

	proto := "http"
	port := cfg.Stack.Nginx.Port
	if s.HTTPS {
		proto = "https"
		port = cfg.Stack.Nginx.SSLPort
	}

	fmt.Printf("\nSite created successfully!\n")
	fmt.Printf("  URL:      %s://%s:%d/\n", proto, s.Name, port)
	fmt.Printf("  Root:     %s\n", s.Root)
	fmt.Printf("  PHP:      %s (FastCGI 127.0.0.1:%d)\n", s.PHPVersion, s.FCGIPort)
	fmt.Printf("  HTTPS:    %t\n\n", s.HTTPS)
	return nil
}

func runSiteList() error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	sites, err := site.List(cfg)
	if err != nil {
		return err
	}

	fmt.Println("Configured Virtual Hosts:")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("  localhost (default site)")
	for _, s := range sites {
		fmt.Printf("  %s\n", s)
	}
	return nil
}

func runSiteRemove(name string) error {
	if name == "" {
		return fmt.Errorf("site domain name is required")
	}

	cfg, err := setupConfig()
	if err != nil {
		return err
	}

	return site.Remove(cfg, name)
}

