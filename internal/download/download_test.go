package download

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile failed: %v", err)
	}

	expected := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash mismatch: got %s, want %s", hash, expectedHex)
	}
}

func TestDownloadAndVerify(t *testing.T) {
	// Create test content
	content := []byte("test download content for SHA256 verification")
	expectedHash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expectedHash[:])

	// Start test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer ts.Close()

	dir := t.TempDir()

	// Test successful download with correct hash
	path, err := DownloadAndVerify(ts.URL+"/test.bin", expectedHex, dir, "test.bin")
	if err != nil {
		t.Fatalf("DownloadAndVerify failed: %v", err)
	}

	downloaded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read downloaded file: %v", err)
	}
	if string(downloaded) != string(content) {
		t.Error("downloaded content does not match")
	}

	// Test re-download skips if hash matches
	path2, err := DownloadAndVerify(ts.URL+"/test.bin", expectedHex, dir, "test.bin")
	if err != nil {
		t.Fatalf("DownloadAndVerify (cached) failed: %v", err)
	}
	if path2 != path {
		t.Error("expected same path for cached download")
	}
}

func TestDownloadAndVerifyBadHash(t *testing.T) {
	content := []byte("test content")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer ts.Close()

	dir := t.TempDir()

	_, err := DownloadAndVerify(ts.URL+"/test.bin", "0000000000000000000000000000000000000000000000000000000000000000", dir, "test.bin")
	if err == nil {
		t.Fatal("expected error for mismatched hash")
	}
	if !contains(err.Error(), "SHA256 mismatch") {
		t.Errorf("expected SHA256 mismatch error, got: %v", err)
	}
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()

	// Create a test zip file
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"mydir/file1.txt": "content of file 1",
		"mydir/sub/file2.txt": "content of file 2",
	})

	extractDir := filepath.Join(dir, "extracted")

	// Extract without strip
	err := ExtractZip(zipPath, extractDir, "")
	if err != nil {
		t.Fatalf("ExtractZip failed: %v", err)
	}

	// Verify files exist
	content1, err := os.ReadFile(filepath.Join(extractDir, "mydir", "file1.txt"))
	if err != nil {
		t.Fatalf("file1.txt not found: %v", err)
	}
	if string(content1) != "content of file 1" {
		t.Errorf("file1.txt content mismatch: %s", content1)
	}

	content2, err := os.ReadFile(filepath.Join(extractDir, "mydir", "sub", "file2.txt"))
	if err != nil {
		t.Fatalf("file2.txt not found: %v", err)
	}
	if string(content2) != "content of file 2" {
		t.Errorf("file2.txt content mismatch: %s", content2)
	}
}

func TestExtractZipWithStrip(t *testing.T) {
	dir := t.TempDir()

	// Create a test zip with a top-level directory to strip
	zipPath := filepath.Join(dir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"nginx-1.30.6/conf/nginx.conf":  "worker_processes 1;",
		"nginx-1.30.6/html/index.html": "<html></html>",
		"nginx-1.30.6/nginx.exe":       "fake exe",
	})

	extractDir := filepath.Join(dir, "nginx")

	err := ExtractZip(zipPath, extractDir, "nginx-1.30.6")
	if err != nil {
		t.Fatalf("ExtractZip with strip failed: %v", err)
	}

	// Files should be directly in extractDir, not under nginx-1.30.6/
	content, err := os.ReadFile(filepath.Join(extractDir, "conf", "nginx.conf"))
	if err != nil {
		t.Fatalf("nginx.conf not found after strip: %v", err)
	}
	if string(content) != "worker_processes 1;" {
		t.Errorf("nginx.conf content mismatch: %s", content)
	}

	if _, err := os.Stat(filepath.Join(extractDir, "nginx.exe")); err != nil {
		t.Errorf("nginx.exe not found after strip: %v", err)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

// createTestZip creates a zip file with the given file contents.
func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("cannot create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("cannot create zip entry %s: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("cannot write zip entry %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("cannot close zip writer: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
