package hosts

import (
	"strings"
	"testing"
)

func TestHasEntryParser(t *testing.T) {
	mockHosts := `
# Sample hosts
127.0.0.1 localhost
::1 localhost
127.0.0.1 myapp.test # webserver-stack-vhost: myapp.test
192.168.1.1 router.local
`
	domain := "myapp.test"
	found := false

	lines := strings.Split(mockHosts, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ip := fields[0]
			if ip == "127.0.0.1" || ip == "::1" {
				for _, d := range fields[1:] {
					if strings.EqualFold(d, domain) {
						found = true
						break
					}
				}
			}
		}
	}

	if !found {
		t.Errorf("expected to find %s in mock hosts file", domain)
	}
}
