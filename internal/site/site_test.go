package site

import (
	"strings"
	"testing"

	"github.com/chakronwork/webserver-stack/internal/templates"
)

func TestRenderVhost(t *testing.T) {
	s := &Site{
		Name:       "myshop.test",
		Root:       "C:/webserver/www/myshop",
		PHPVersion: "8.4",
		FCGIPort:   9084,
		Port:       80,
		SSLPort:    443,
		HTTPS:      true,
		CertFile:   "C:/webserver/conf/ssl/myshop.test.crt",
		KeyFile:    "C:/webserver/conf/ssl/myshop.test.key",
	}

	out, err := RenderVhost(templates.VhostConfTemplate, s)
	if err != nil {
		t.Fatalf("RenderVhost failed: %v", err)
	}

	// Must contain both HTTP and HTTPS blocks
	expected := []string{
		"server_name  myshop.test;",
		"listen       127.0.0.1:80;",
		"listen       127.0.0.1:443 ssl;",
		"fastcgi_pass   127.0.0.1:9084;",
		`root         "C:/webserver/www/myshop";`,
		`ssl_certificate      "C:/webserver/conf/ssl/myshop.test.crt";`,
	}

	for _, exp := range expected {
		if !strings.Contains(out, exp) {
			t.Errorf("vhost config missing expected directive: %s", exp)
		}
	}
}

func TestRenderVhostHTTPOnly(t *testing.T) {
	s := &Site{
		Name:       "blog.test",
		Root:       "C:/webserver/www/blog",
		PHPVersion: "8.5",
		FCGIPort:   9085,
		Port:       8080,
		HTTPS:      false,
	}

	out, err := RenderVhost(templates.VhostConfTemplate, s)
	if err != nil {
		t.Fatalf("RenderVhost failed: %v", err)
	}

	if strings.Contains(out, "ssl_certificate") {
		t.Error("HTTP-only site should not include ssl_certificate")
	}
	if !strings.Contains(out, "fastcgi_pass   127.0.0.1:9085;") {
		t.Error("expected PHP 8.5 FastCGI port 9085")
	}
}
