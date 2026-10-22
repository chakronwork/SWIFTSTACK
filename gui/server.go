package gui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/chakronwork/webserver-stack/internal/config"
	"github.com/chakronwork/webserver-stack/internal/db"
	"github.com/chakronwork/webserver-stack/internal/doctor"
	"github.com/chakronwork/webserver-stack/internal/logger"
	"github.com/chakronwork/webserver-stack/internal/service"
	"github.com/chakronwork/webserver-stack/internal/site"
)

//go:embed frontend/index.html
var DashboardHTML []byte

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func sendJSON(w http.ResponseWriter, status int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// StartDashboardServer launches the local GUI dashboard web UI on localhost.
func StartDashboardServer(cfg *config.AppConfig, mgr *service.Manager, runInitFunc func() error) error {
	log := logger.G()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to bind GUI port: %w", err)
	}
	guiPort := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()

	// Serve Frontend Dashboard
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(DashboardHTML)
	})

	// API: Get Status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		statuses := mgr.Status()
		res := map[string]interface{}{
			"nginx": map[string]interface{}{
				"state": statuses["nginx"].State.String(),
				"port":  statuses["nginx"].Port,
				"pid":   statuses["nginx"].PID,
				"error": statuses["nginx"].Error,
			},
			"php": map[string]interface{}{
				"state": statuses["php"].State.String(),
				"port":  statuses["php"].Port,
				"pid":   statuses["php"].PID,
				"error": statuses["php"].Error,
			},
			"mariadb": map[string]interface{}{
				"state": statuses["mariadb"].State.String(),
				"port":  statuses["mariadb"].Port,
				"pid":   statuses["mariadb"].PID,
				"error": statuses["mariadb"].Error,
			},
		}
		sendJSON(w, http.StatusOK, APIResponse{Success: true, Data: res})
	})

	// API: Control Services
	mux.HandleFunc("/api/control", func(w http.ResponseWriter, r *http.Request) {
		serviceName := r.URL.Query().Get("service")
		action := r.URL.Query().Get("action")

		var err error
		switch action {
		case "start":
			err = mgr.Start(serviceName)
		case "stop":
			err = mgr.Stop(serviceName)
		case "restart":
			err = mgr.Restart(serviceName)
		case "start_all":
			err = mgr.StartAll()
		case "stop_all":
			err = mgr.StopAll()
		default:
			err = fmt.Errorf("unknown action: %s", action)
		}

		if err != nil {
			sendJSON(w, http.StatusOK, APIResponse{Success: false, Message: err.Error()})
			return
		}
		sendJSON(w, http.StatusOK, APIResponse{Success: true, Message: "Action completed successfully"})
	})

	// API: Run Init (Download components & setup)
	mux.HandleFunc("/api/init", func(w http.ResponseWriter, r *http.Request) {
		if runInitFunc == nil {
			sendJSON(w, http.StatusOK, APIResponse{Success: false, Message: "Init function not configured"})
			return
		}
		go func() {
			if err := runInitFunc(); err != nil {
				log.Error("Background init error: %v", err)
			}
		}()
		sendJSON(w, http.StatusOK, APIResponse{Success: true, Message: "Initialization started in background. Watch terminal for progress."})
	})

	// API: Database Password
	mux.HandleFunc("/api/db/password", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "reset" {
			newPwd, err := db.GenerateRandomPassword(24)
			if err != nil {
				sendJSON(w, http.StatusOK, APIResponse{Success: false, Message: err.Error()})
				return
			}
			if err := db.ResetRootPassword(cfg.Paths.Data, newPwd); err != nil {
				sendJSON(w, http.StatusOK, APIResponse{Success: false, Message: err.Error()})
				return
			}
			sendJSON(w, http.StatusOK, APIResponse{Success: true, Data: newPwd})
			return
		}

		pwd, _, err := db.GetOrCreateRootPassword(cfg.Paths.Data)
		if err != nil {
			sendJSON(w, http.StatusOK, APIResponse{Success: false, Message: err.Error()})
			return
		}
		sendJSON(w, http.StatusOK, APIResponse{Success: true, Data: pwd})
	})

	// API: Sites List
	mux.HandleFunc("/api/sites", func(w http.ResponseWriter, r *http.Request) {
		sites, err := site.List(cfg)
		if err != nil {
			sendJSON(w, http.StatusOK, APIResponse{Success: false, Message: err.Error()})
			return
		}
		sendJSON(w, http.StatusOK, APIResponse{Success: true, Data: sites})
	})

	// API: Doctor Diagnostics
	mux.HandleFunc("/api/doctor", func(w http.ResponseWriter, r *http.Request) {
		report := doctor.RunDiagnostics(cfg)
		sendJSON(w, http.StatusOK, APIResponse{Success: true, Data: report})
	})

	dashboardURL := fmt.Sprintf("http://127.0.0.1:%d", guiPort)
	log.Info("SWIFTSTACK Dashboard running at %s", dashboardURL)
	fmt.Printf("\n============================================================\n")
	fmt.Printf("🚀 SWIFTSTACK Control Dashboard: %s\n", dashboardURL)
	fmt.Printf("============================================================\n\n")

	// Open browser automatically
	go func() {
		time.Sleep(400 * time.Millisecond)
		_ = exec.Command("cmd.exe", "/c", "start", dashboardURL).Start()
	}()

	server := &http.Server{Handler: mux}
	return server.Serve(listener)
}
