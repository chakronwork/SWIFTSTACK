package port

import (
	"testing"
	"net"
)

func TestCheckAvailablePort(t *testing.T) {
	// Find a free port by letting the OS assign one
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	result := Check("127.0.0.1", port)
	if !result.Available {
		t.Errorf("expected port %d to be available, but it wasn't: %s", port, result.Error)
	}
}

func TestCheckOccupiedPort(t *testing.T) {
	// Occupy a port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	result := Check("127.0.0.1", port)
	if result.Available {
		t.Errorf("expected port %d to be occupied, but it was available", port)
	}
	if result.Error == "" {
		t.Error("expected an error message for occupied port")
	}
}

func TestSuggestAlternative(t *testing.T) {
	// Occupy a port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	alt := SuggestAlternative("127.0.0.1", port)
	if alt == 0 {
		t.Error("expected a suggested alternative port")
	}
	if alt <= port {
		t.Errorf("expected alternative port > %d, got %d", port, alt)
	}
}

func TestCheckMultiple(t *testing.T) {
	// Find two free ports
	l1, _ := net.Listen("tcp", "127.0.0.1:0")
	p1 := l1.Addr().(*net.TCPAddr).Port
	l1.Close()

	l2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l2.Close()
	p2 := l2.Addr().(*net.TCPAddr).Port

	results := CheckMultiple("127.0.0.1", []int{p1, p2})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Available {
		t.Errorf("port %d should be available", p1)
	}
	if results[1].Available {
		t.Errorf("port %d should be occupied", p2)
	}
}
