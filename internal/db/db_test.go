package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRandomPassword(t *testing.T) {
	pwd1, err := GenerateRandomPassword(24)
	if err != nil {
		t.Fatalf("GenerateRandomPassword failed: %v", err)
	}
	if len(pwd1) != 24 {
		t.Errorf("expected password length 24, got %d", len(pwd1))
	}

	pwd2, err := GenerateRandomPassword(24)
	if err != nil {
		t.Fatalf("GenerateRandomPassword failed: %v", err)
	}

	if pwd1 == pwd2 {
		t.Error("two randomly generated passwords should not be identical")
	}
}

func TestGetOrCreateRootPassword(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create first time
	pwd, isNew, err := GetOrCreateRootPassword(tempDir)
	if err != nil {
		t.Fatalf("GetOrCreateRootPassword failed: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true on initial run")
	}
	if pwd == "" {
		t.Error("password must not be empty")
	}

	// 2. Fetch existing
	pwd2, isNew2, err := GetOrCreateRootPassword(tempDir)
	if err != nil {
		t.Fatalf("subsequent GetOrCreateRootPassword failed: %v", err)
	}
	if isNew2 {
		t.Error("expected isNew=false on second run")
	}
	if pwd != pwd2 {
		t.Errorf("passwords mismatch: %s vs %s", pwd, pwd2)
	}

	// 3. Reset password
	newPwd := "new-test-secret-12345"
	if err := ResetRootPassword(tempDir, newPwd); err != nil {
		t.Fatalf("ResetRootPassword failed: %v", err)
	}

	pwd3, _, _ := GetOrCreateRootPassword(tempDir)
	if pwd3 != newPwd {
		t.Errorf("expected %s, got %s", newPwd, pwd3)
	}
}

func TestEnsureSecureGitignore(t *testing.T) {
	tempDir := t.TempDir()
	if err := EnsureSecureGitignore(tempDir); err != nil {
		t.Fatalf("EnsureSecureGitignore failed: %v", err)
	}

	content, err := filepath.Glob(filepath.Join(tempDir, ".gitignore"))
	if err != nil || len(content) == 0 {
		t.Fatal(".gitignore file was not created")
	}
	_ = strings.Contains
}
