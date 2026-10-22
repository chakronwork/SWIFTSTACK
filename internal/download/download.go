// Package download handles downloading, SHA256 verification, and extraction of components.
package download

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chakronwork/webserver-stack/internal/logger"
)

// Component represents a downloadable component.
type Component struct {
	Name    string // e.g. "nginx", "php-8.4", "mariadb"
	URL     string
	SHA256  string
	DestDir string // final directory where extracted files go
	Strip   string // top-level directory to strip from the zip
}

// Result holds the outcome of a download operation.
type Result struct {
	Component string
	Success   bool
	Error     error
	FilePath  string
}

// DownloadAndVerify downloads a file from url, verifies its SHA256 hash, and returns the path to the downloaded file.
func DownloadAndVerify(url, expectedSHA256, destDir, filename string) (string, error) {
	log := logger.G()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create download directory %s: %w", destDir, err)
	}

	destPath := filepath.Join(destDir, filename)

	// Check if file already exists with correct hash
	if existingHash, err := hashFile(destPath); err == nil {
		if strings.EqualFold(existingHash, expectedSHA256) {
			log.Info("Component %s already downloaded and verified", filename)
			return destPath, nil
		}
		log.Warn("Existing file %s has incorrect hash, re-downloading", filename)
	}

	log.Info("Downloading %s ...", url)

	// Create a temporary file for downloading
	tmpPath := destPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("cannot create temp file %s: %w", tmpPath, err)
	}

	// Download with progress
	client := &http.Client{
		Timeout: 30 * time.Minute, // Large files may take a while on slow connections
	}

	resp, err := client.Get(url)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("download failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("download failed for %s: HTTP %d %s", url, resp.StatusCode, resp.Status)
	}

	// Hash while downloading
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	totalBytes := resp.ContentLength
	downloaded := int64(0)
	buf := make([]byte, 32*1024)
	lastReport := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := writer.Write(buf[:n])
			if writeErr != nil {
				tmpFile.Close()
				os.Remove(tmpPath)
				return "", fmt.Errorf("write error while downloading %s: %w", url, writeErr)
			}
			downloaded += int64(n)

			// Report progress every 2 seconds
			if time.Since(lastReport) > 2*time.Second {
				if totalBytes > 0 {
					pct := float64(downloaded) / float64(totalBytes) * 100
					log.Info("  %s: %.1f%% (%s / %s)",
						filename, pct,
						formatBytes(downloaded), formatBytes(totalBytes))
				} else {
					log.Info("  %s: %s downloaded", filename, formatBytes(downloaded))
				}
				lastReport = time.Now()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("read error while downloading %s: %w", url, readErr)
		}
	}

	tmpFile.Close()

	// Verify SHA256
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedSHA256) {
		// If the expected hash is a placeholder, warn but don't fail
		if expectedSHA256 == "VERIFY_AFTER_DOWNLOAD" {
			log.Warn("SHA256 verification skipped for %s (placeholder hash). Actual: %s", filename, actualHash)
			log.Warn("Update versions.toml with: sha256 = \"%s\"", actualHash)
		} else {
			os.Remove(tmpPath)
			return "", fmt.Errorf(
				"SHA256 mismatch for %s:\n  expected: %s\n  actual:   %s\n"+
					"The file may be corrupted or tampered with. Delete the download cache and try again.",
				filename, expectedSHA256, actualHash)
		}
	} else {
		log.Info("SHA256 verified: %s", filename)
	}

	// Rename temp file to final destination
	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("cannot rename %s to %s: %w", tmpPath, destPath, err)
	}

	log.Info("Downloaded %s (%s)", filename, formatBytes(downloaded))
	return destPath, nil
}

// ExtractZip extracts a zip file to destDir, optionally stripping a top-level directory.
func ExtractZip(zipPath, destDir, strip string) error {
	log := logger.G()
	log.Info("Extracting %s to %s ...", filepath.Base(zipPath), destDir)

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("cannot open zip %s: %w", zipPath, err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("cannot create destination %s: %w", destDir, err)
	}

	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)

		// Strip top-level directory if specified
		if strip != "" {
			prefix := strip + "/"
			if !strings.HasPrefix(name, prefix) {
				// Also handle case where strip matches exactly (the dir entry itself)
				if name == strip || name == strip+"/" {
					continue
				}
				continue
			}
			name = strings.TrimPrefix(name, prefix)
		}

		if name == "" {
			continue
		}

		// Convert to OS path
		destPath := filepath.Join(destDir, filepath.FromSlash(name))

		// Security: prevent zip slip
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			log.Warn("Skipping potentially unsafe path in zip: %s", f.Name)
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("cannot create directory for %s: %w", destPath, err)
		}

		// Extract file
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("cannot open zip entry %s: %w", f.Name, err)
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("cannot create file %s: %w", destPath, err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("cannot extract %s: %w", f.Name, err)
		}
	}

	log.Info("Extracted %s", filepath.Base(zipPath))
	return nil
}

// CopySingleFile downloads a single file (not a zip) and places it in destDir with the given name.
func CopySingleFile(srcPath, destDir, destName string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", destDir, err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("cannot open source %s: %w", srcPath, err)
	}
	defer src.Close()

	destPath := filepath.Join(destDir, destName)
	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("cannot create destination %s: %w", destPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("cannot copy file: %w", err)
	}

	return nil
}

// hashFile computes the SHA256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile computes and returns the SHA256 hash of a file (exported for use by other packages).
func HashFile(path string) (string, error) {
	return hashFile(path)
}

// formatBytes formats bytes into human-readable string.
func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
