package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chakronwork/webserver-stack/internal/config"
)

func testConfig() *config.AppConfig {
	return &config.AppConfig{
		Stack: config.StackConfig{
			General: config.GeneralConfig{
				Profile:  "dev",
				Language: "en",
			},
			Nginx: config.NginxConfig{
				Port:    8080,
				SSLPort: 8443,
				Bind:    "127.0.0.1",
			},
			PHP: config.PHPConfig{
				DefaultVersion:    "8.4",
				MemoryLimit:       "256M",
				UploadMaxFilesize: "64M",
				PostMaxSize:       "64M",
				MaxExecutionTime:  300,
				DisplayErrors:     true,
				Xdebug:            false,
			},
			MariaDB: config.MariaDBConfig{
				Port: 3306,
				Bind: "127.0.0.1",
			},
			PhpMyAdmin: config.PhpMyAdminConfig{
				Enabled: true,
			},
		},
		Paths: config.ResolvePaths(`C:\webserver`),
	}
}

func TestBuildRenderData(t *testing.T) {
	cfg := testConfig()
	data := BuildRenderData(cfg)

	if data.NginxRoot != "C:/webserver" {
		t.Errorf("expected NginxRoot C:/webserver, got %s", data.NginxRoot)
	}
	if data.PHP.FCGIPort != 9084 {
		t.Errorf("expected FCGIPort 9084, got %d", data.PHP.FCGIPort)
	}
	if data.PHP.DisplayErrors != "On" {
		t.Errorf("expected DisplayErrors On in dev mode, got %s", data.PHP.DisplayErrors)
	}
}

func TestBuildRenderDataProd(t *testing.T) {
	cfg := testConfig()
	cfg.Stack.General.Profile = "prod"
	data := BuildRenderData(cfg)

	if data.PHP.DisplayErrors != "Off" {
		t.Errorf("expected DisplayErrors Off in prod mode, got %s", data.PHP.DisplayErrors)
	}
}

func TestRenderNginxConf(t *testing.T) {
	cfg := testConfig()
	data := BuildRenderData(cfg)

	result, err := RenderToString("nginx.conf", NginxConfTemplate, data)
	if err != nil {
		t.Fatalf("RenderToString failed: %v", err)
	}

	// Check key directives are present
	checks := []string{
		"127.0.0.1:8080",
		"fastcgi_pass   127.0.0.1:9084",
		"C:/webserver/www",
		"C:/webserver/logs/nginx-error.log",
		"client_max_body_size 64M",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("nginx.conf missing expected content: %s", check)
		}
	}

	// Should NOT contain backslashes in paths
	if strings.Contains(result, `C:\webserver`) {
		t.Error("nginx.conf should use forward slashes, not backslashes")
	}
}

func TestRenderPHPIni(t *testing.T) {
	cfg := testConfig()
	data := BuildRenderData(cfg)

	result, err := RenderToString("php.ini", PHPIniTemplate, data)
	if err != nil {
		t.Fatalf("RenderToString failed: %v", err)
	}

	checks := []string{
		"memory_limit = 256M",
		"upload_max_filesize = 64M",
		"max_execution_time = 300",
		"display_errors = On",
		"expose_php = Off",
		"extension=mysqli",
		"extension=pdo_mysql",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("php.ini missing expected content: %s", check)
		}
	}
}

func TestRenderMyIni(t *testing.T) {
	cfg := testConfig()
	data := BuildRenderData(cfg)

	result, err := RenderToString("my.ini", MyIniTemplate, data)
	if err != nil {
		t.Fatalf("RenderToString failed: %v", err)
	}

	checks := []string{
		"port = 3306",
		"bind-address = 127.0.0.1",
		"skip-name-resolve",
		"utf8mb4",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("my.ini missing expected content: %s", check)
		}
	}
}

func TestRenderToFile(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "test.conf")

	tmpl := "server { listen {{.Port}}; }"
	data := struct{ Port int }{8080}

	err := RenderToFile("test", tmpl, destPath, data)
	if err != nil {
		t.Fatalf("RenderToFile failed: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("cannot read rendered file: %v", err)
	}

	expected := "server { listen 8080; }"
	if string(content) != expected {
		t.Errorf("rendered content mismatch: got %s, want %s", string(content), expected)
	}
}

func TestToForwardSlash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`C:\webserver\bin\nginx`, "C:/webserver/bin/nginx"},
		{`C:\Users\ทดสอบ\dev`, "C:/Users/ทดสอบ/dev"},
		{"already/forward", "already/forward"},
		{`mixed/path\here`, "mixed/path/here"},
	}
	for _, tt := range tests {
		got := ToForwardSlash(tt.input)
		if got != tt.expected {
			t.Errorf("ToForwardSlash(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}
