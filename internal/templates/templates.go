// Package templates renders service configuration files from Go templates.
package templates

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/logger"
)

// RenderData holds all data available to templates.
type RenderData struct {
	Stack    config.StackConfig
	Paths    config.Paths
	PHP      PHPRenderData
	Sites    []SiteData
	// ForwardSlash versions of paths for nginx (which requires forward slashes)
	NginxRoot     string
	NginxConf     string
	NginxLogs     string
	NginxWWW      string
	NginxSessions string
	NginxTmp      string
}

// PHPRenderData holds PHP-specific template data.
type PHPRenderData struct {
	Version       string
	FCGIPort      int
	ExtDir        string // forward-slash path to ext dir
	TmpDir        string // forward-slash path to tmp dir
	SessionDir    string // forward-slash path to session dir
	DisplayErrors string // "On" or "Off"
}

// SiteData represents a virtual host site.
type SiteData struct {
	Name       string
	Root       string // forward-slash path
	PHPVersion string
	FCGIPort   int
	HTTPS      bool
	CertFile   string // forward-slash path
	KeyFile    string // forward-slash path
}

// toForwardSlash converts Windows backslash paths to forward slashes (required by nginx).
func toForwardSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// ToForwardSlash is the exported version for use by other packages.
func ToForwardSlash(path string) string {
	return toForwardSlash(path)
}

// BuildRenderData constructs RenderData from the app configuration.
func BuildRenderData(cfg *config.AppConfig) RenderData {
	displayErrors := "Off"
	if cfg.Stack.IsDevProfile() && cfg.Stack.PHP.DisplayErrors {
		displayErrors = "On"
	}

	phpVersion := cfg.Stack.PHP.DefaultVersion
	phpDir := cfg.Paths.PHPDir(phpVersion)

	rd := RenderData{
		Stack:         cfg.Stack,
		Paths:         cfg.Paths,
		NginxRoot:     toForwardSlash(cfg.Paths.Root),
		NginxConf:     toForwardSlash(cfg.Paths.Conf),
		NginxLogs:     toForwardSlash(cfg.Paths.Logs),
		NginxWWW:      toForwardSlash(cfg.Paths.WWW),
		NginxSessions: toForwardSlash(cfg.Paths.Sessions),
		NginxTmp:      toForwardSlash(cfg.Paths.Tmp),
		PHP: PHPRenderData{
			Version:       phpVersion,
			FCGIPort:      config.PHPFCGIPort(phpVersion),
			ExtDir:        toForwardSlash(filepath.Join(phpDir, "ext")),
			TmpDir:        toForwardSlash(cfg.Paths.Tmp),
			SessionDir:    toForwardSlash(cfg.Paths.Sessions),
			DisplayErrors: displayErrors,
		},
	}

	return rd
}

// RenderToFile renders a template string to a file.
func RenderToFile(tmplName, tmplContent, destPath string, data interface{}) error {
	log := logger.G()

	funcMap := template.FuncMap{
		"fwd": toForwardSlash,
	}

	tmpl, err := template.New(tmplName).Funcs(funcMap).Parse(tmplContent)
	if err != nil {
		return fmt.Errorf("template parse error in %s: %w", tmplName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("template execution error in %s: %w", tmplName, err)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", destPath, err)
	}

	if err := os.WriteFile(destPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("cannot write %s: %w", destPath, err)
	}

	log.Debug("Rendered %s", destPath)
	return nil
}

// RenderToString renders a template string and returns the result.
func RenderToString(tmplName, tmplContent string, data interface{}) (string, error) {
	funcMap := template.FuncMap{
		"fwd": toForwardSlash,
	}

	tmpl, err := template.New(tmplName).Funcs(funcMap).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("template parse error in %s: %w", tmplName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution error in %s: %w", tmplName, err)
	}

	return buf.String(), nil
}

// RenderAll generates all config files from templates.
func RenderAll(cfg *config.AppConfig, tmplDir string) error {
	log := logger.G()
	log.Info("Rendering configuration files...")

	data := BuildRenderData(cfg)

	// Define template files to render
	templateFiles := []struct {
		templateName string
		outputPath   string
	}{
		{"nginx.conf.tmpl", filepath.Join(cfg.Paths.Conf, "nginx.conf")},
		{"php.ini.tmpl", filepath.Join(cfg.Paths.Conf, "php.ini")},
		{"my.ini.tmpl", filepath.Join(cfg.Paths.Conf, "my.ini")},
	}

	for _, tf := range templateFiles {
		tmplPath := filepath.Join(tmplDir, tf.templateName)
		content, err := os.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("cannot read template %s: %w", tmplPath, err)
		}

		if err := RenderToFile(tf.templateName, string(content), tf.outputPath, data); err != nil {
			return err
		}
	}

	log.Info("Configuration files rendered successfully")
	return nil
}

// RenderAllEmbedded generates all config files from embedded template strings.
func RenderAllEmbedded(cfg *config.AppConfig) error {
	log := logger.G()
	log.Info("Rendering configuration files...")

	data := BuildRenderData(cfg)

	templates := []struct {
		name    string
		content string
		output  string
	}{
		{"nginx.conf", NginxConfTemplate, filepath.Join(cfg.Paths.Conf, "nginx.conf")},
		{"php.ini", PHPIniTemplate, filepath.Join(cfg.Paths.Conf, "php.ini")},
		{"my.ini", MyIniTemplate, filepath.Join(cfg.Paths.Conf, "my.ini")},
	}

	for _, t := range templates {
		if err := RenderToFile(t.name, t.content, t.output, data); err != nil {
			return err
		}
	}

	log.Info("Configuration files rendered successfully")
	return nil
}

// Embedded template strings — these are also stored as .tmpl files in the templates/ directory.

// NginxConfTemplate is the nginx.conf template.
var NginxConfTemplate = `# Generated by webserver stack — do not edit manually.
# Modify stack.toml and run 'webserver init' or 'webserver restart nginx' to regenerate.

worker_processes  auto;
error_log  "{{.NginxLogs}}/nginx-error.log" warn;
pid        "{{.NginxTmp}}/nginx.pid";

events {
    worker_connections  1024;
}

http {
    include       mime.types;
    default_type  application/octet-stream;

    log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
                      '$status $body_bytes_sent "$http_referer" '
                      '"$http_user_agent"';

    access_log  "{{.NginxLogs}}/nginx-access.log"  main;

    sendfile           on;
    keepalive_timeout  65;
    client_max_body_size {{.Stack.PHP.PostMaxSize}};

    # Temp paths
    client_body_temp_path "{{.NginxTmp}}/client_body";
    proxy_temp_path       "{{.NginxTmp}}/proxy";
    fastcgi_temp_path     "{{.NginxTmp}}/fastcgi";
    uwsgi_temp_path       "{{.NginxTmp}}/uwsgi";
    scgi_temp_path        "{{.NginxTmp}}/scgi";

    # Default server
    server {
        listen       {{.Stack.Nginx.Bind}}:{{.Stack.Nginx.Port}};
        server_name  localhost;
        root         "{{.NginxWWW}}";
        index        index.php index.html index.htm;

        location / {
            try_files $uri $uri/ /index.php$is_args$args;
        }

        location ~ \.php$ {
            fastcgi_pass   {{.Stack.Nginx.Bind}}:{{.PHP.FCGIPort}};
            fastcgi_index  index.php;
            fastcgi_param  SCRIPT_FILENAME $document_root$fastcgi_script_name;
            include        fastcgi_params;
        }

        # Deny access to . files (e.g., .htaccess, .git)
        location ~ /\. {
            deny all;
        }
    }

    # Include virtual host configs
    include "{{.NginxConf}}/vhosts/*.conf";
}
`

// VhostConfTemplate is the virtual host configuration template.
var VhostConfTemplate = `# Generated by webserver stack — virtual host for {{.Name}}
server {
    listen       127.0.0.1:{{.Port}};
    server_name  {{.Name}};
    root         "{{.Root}}";
    index        index.php index.html index.htm;

    location / {
        try_files $uri $uri/ /index.php$is_args$args;
    }

    location ~ \.php$ {
        fastcgi_pass   127.0.0.1:{{.FCGIPort}};
        fastcgi_index  index.php;
        fastcgi_param  SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include        fastcgi_params;
    }

    location ~ /\. {
        deny all;
    }
}
{{if .HTTPS}}
server {
    listen       127.0.0.1:{{.SSLPort}} ssl;
    server_name  {{.Name}};
    root         "{{.Root}}";
    index        index.php index.html index.htm;

    ssl_certificate      "{{.CertFile}}";
    ssl_certificate_key  "{{.KeyFile}}";

    location / {
        try_files $uri $uri/ /index.php$is_args$args;
    }

    location ~ \.php$ {
        fastcgi_pass   127.0.0.1:{{.FCGIPort}};
        fastcgi_index  index.php;
        fastcgi_param  SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include        fastcgi_params;
    }

    location ~ /\. {
        deny all;
    }
}
{{end}}
`

// PHPIniTemplate is the php.ini template.
var PHPIniTemplate = `; Generated by webserver stack — do not edit manually.
; Modify stack.toml and run 'webserver init' or 'webserver restart php' to regenerate.

[PHP]
engine = On
short_open_tag = Off
precision = 14
output_buffering = 4096
zlib.output_compression = Off
implicit_flush = Off
serialize_precision = -1
disable_functions =
disable_classes =
zend.enable_gc = On
expose_php = Off

; Error handling
display_errors = {{.PHP.DisplayErrors}}
display_startup_errors = {{.PHP.DisplayErrors}}
log_errors = On
log_errors_max_len = 1024
error_log = "{{fwd .Paths.Logs}}/php-error.log"
error_reporting = E_ALL

; Resource limits
max_execution_time = {{.Stack.PHP.MaxExecutionTime}}
max_input_time = 60
memory_limit = {{.Stack.PHP.MemoryLimit}}

; File uploads
file_uploads = On
upload_max_filesize = {{.Stack.PHP.UploadMaxFilesize}}
max_file_uploads = 20
post_max_size = {{.Stack.PHP.PostMaxSize}}

; Paths
extension_dir = "{{.PHP.ExtDir}}"
sys_temp_dir = "{{.PHP.TmpDir}}"

; Session
session.save_handler = files
session.save_path = "{{.PHP.SessionDir}}"
session.use_strict_mode = 1
session.use_cookies = 1
session.use_only_cookies = 1
session.cookie_httponly = 1
session.cookie_samesite = "Lax"
session.gc_maxlifetime = 1440

; Date
date.timezone = Asia/Bangkok

; Extensions
extension=bz2
extension=curl
extension=fileinfo
extension=gd
extension=gettext
extension=intl
extension=mbstring
extension=exif
extension=mysqli
extension=openssl
extension=pdo_mysql
extension=sodium
extension=zip

; OPcache
zend_extension=opcache
opcache.enable=1
opcache.enable_cli=0
opcache.memory_consumption=128
opcache.interned_strings_buffer=8
opcache.max_accelerated_files=10000
opcache.validate_timestamps=1
opcache.revalidate_freq=2

; cURL CA bundle (use the one bundled with PHP)
curl.cainfo = "{{.PHP.ExtDir}}/../ssl/cacert.pem"
openssl.cafile = "{{.PHP.ExtDir}}/../ssl/cacert.pem"
`

// MyIniTemplate is the MariaDB my.ini template.
var MyIniTemplate = `# Generated by webserver stack — do not edit manually.
# Modify stack.toml and run 'webserver init' or 'webserver restart mariadb' to regenerate.

[mysqld]
port = {{.Stack.MariaDB.Port}}
bind-address = {{.Stack.MariaDB.Bind}}
datadir = "{{fwd .Paths.DataSQL}}"
tmpdir = "{{fwd .Paths.Tmp}}"
log-error = "{{fwd .Paths.Logs}}/mariadb-error.log"
general-log-file = "{{fwd .Paths.Logs}}/mariadb-general.log"
slow-query-log-file = "{{fwd .Paths.Logs}}/mariadb-slow.log"

# Skip networking on non-localhost (already bound to 127.0.0.1)
skip-name-resolve

# InnoDB settings
innodb_buffer_pool_size = 128M
innodb_log_file_size = 48M
innodb_file_per_table = 1
innodb_flush_log_at_trx_commit = 1
innodb_flush_method = normal

# Character set
character-set-server = utf8mb4
collation-server = utf8mb4_unicode_ci

# Limits
max_connections = 100
max_allowed_packet = 64M
table_open_cache = 256
sort_buffer_size = 512K
read_buffer_size = 256K

[client]
port = {{.Stack.MariaDB.Port}}
default-character-set = utf8mb4
`
