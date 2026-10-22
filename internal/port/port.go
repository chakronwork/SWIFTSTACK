// Package port provides TCP port availability checking and conflict detection.
package port

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CheckResult holds the result of a port availability check.
type CheckResult struct {
	Port      int
	Available bool
	PID       int    // PID of the process using the port (0 if available)
	Process   string // Process name using the port
	Error     string // Human-readable error message
}

// Check tests whether a TCP port is available on the given bind address.
func Check(bind string, port int) CheckResult {
	result := CheckResult{Port: port}

	addr := fmt.Sprintf("%s:%d", bind, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is in use; try to find out who's using it
		result.Available = false
		pid, procName := findProcessOnPort(port)
		result.PID = pid
		result.Process = procName

		if pid > 0 {
			result.Error = fmt.Sprintf("Port %d is used by PID %d (%s). Change the port in stack.toml or stop the conflicting service.", port, pid, procName)
		} else {
			result.Error = fmt.Sprintf("Port %d is not available: %v. Change the port in stack.toml.", port, err)
		}
		return result
	}
	listener.Close()

	result.Available = true
	return result
}

// CheckMultiple checks a list of ports and returns results for each.
func CheckMultiple(bind string, ports []int) []CheckResult {
	results := make([]CheckResult, 0, len(ports))
	for _, p := range ports {
		results = append(results, Check(bind, p))
	}
	return results
}

// WaitForPort waits until a port becomes reachable (for health checking a started service).
func WaitForPort(bind string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("%s:%d", bind, port)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("port %d on %s did not become available within %v", port, bind, timeout)
}

// SuggestAlternative returns the next available port starting from the given one.
func SuggestAlternative(bind string, startPort int) int {
	for p := startPort + 1; p < startPort+100; p++ {
		result := Check(bind, p)
		if result.Available {
			return p
		}
	}
	return 0
}

// findProcessOnPort uses netstat to find the process holding a port (Windows-specific).
func findProcessOnPort(port int) (pid int, name string) {
	cmd := exec.Command("netstat", "-ano", "-p", "TCP")
	output, err := cmd.Output()
	if err != nil {
		return 0, ""
	}

	portStr := fmt.Sprintf(":%d ", port)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, portStr) {
			continue
		}
		if !strings.Contains(line, "LISTENING") {
			continue
		}

		// Extract PID from the last column
		fields := regexp.MustCompile(`\s+`).Split(line, -1)
		if len(fields) < 5 {
			continue
		}
		pidStr := fields[len(fields)-1]
		p, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		procName := getProcessName(p)
		return p, procName
	}

	return 0, ""
}

// getProcessName retrieves the process name for a given PID using tasklist.
func getProcessName(pid int) string {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	line := strings.TrimSpace(string(output))
	if line == "" || strings.Contains(line, "No tasks") {
		return "unknown"
	}

	// CSV format: "process.exe","PID","Session Name","Session#","Mem Usage"
	parts := strings.Split(line, ",")
	if len(parts) > 0 {
		name := strings.Trim(parts[0], "\"")
		return name
	}

	return "unknown"
}
