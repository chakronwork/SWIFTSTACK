package doctor

import (
	"testing"

	"github.com/chakronwork/webserver-stack/internal/config"
)

func TestRunDiagnostics(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.AppConfig{
		Stack: config.StackConfig{
			Nginx: config.NginxConfig{
				Port:    18080,
				SSLPort: 18443,
				Bind:    "127.0.0.1",
			},
			MariaDB: config.MariaDBConfig{
				Port: 13306,
				Bind: "127.0.0.1",
			},
			PHP: config.PHPConfig{
				DefaultVersion: "8.4",
			},
		},
		Paths: config.ResolvePaths(tempDir),
	}

	report := RunDiagnostics(cfg)
	if len(report.Items) == 0 {
		t.Fatal("expected diagnostic items, got 0")
	}

	foundVC := false
	for _, it := range report.Items {
		if it.Name == "Visual C++ Redistributable (x64)" {
			foundVC = true
			break
		}
	}

	if !foundVC {
		t.Error("Visual C++ Redistributable check was missing from report")
	}
}
