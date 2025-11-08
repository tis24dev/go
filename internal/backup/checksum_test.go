package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/types"
)

func TestGenerateAndVerifyChecksum(t *testing.T) {
	logger := logging.New(types.LogLevelDebug, false)
	ctx := context.Background()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("checksum-test-content")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	checksum, err := GenerateChecksum(ctx, logger, filePath)
	if err != nil {
		t.Fatalf("GenerateChecksum failed: %v", err)
	}
	if checksum == "" {
		t.Fatal("checksum should not be empty")
	}

	ok, err := VerifyChecksum(ctx, logger, filePath, checksum)
	if err != nil {
		t.Fatalf("VerifyChecksum failed: %v", err)
	}
	if !ok {
		t.Fatal("expected checksum verification to succeed")
	}

	// Modify file and ensure verification fails
	if err := os.WriteFile(filePath, []byte("modified"), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	ok, err = VerifyChecksum(ctx, logger, filePath, checksum)
	if err != nil {
		t.Fatalf("VerifyChecksum after modification failed: %v", err)
	}
	if ok {
		t.Fatal("expected checksum verification to fail after modification")
	}
}

func TestCreateAndLoadManifest(t *testing.T) {
	logger := logging.New(types.LogLevelInfo, false)
	ctx := context.Background()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	manifest := &Manifest{
		ArchivePath:      "/opt/proxmox-backup/backup/test.tar.xz",
		ArchiveSize:      1024,
		SHA256:           "abc123",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
		CompressionType:  "xz",
		CompressionLevel: 6,
		ProxmoxType:      "pbs",
		Hostname:         "test-host",
	}

	if err := CreateManifest(ctx, logger, manifest, manifestPath); err != nil {
		t.Fatalf("CreateManifest failed: %v", err)
	}

	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if loaded.ArchivePath != manifest.ArchivePath {
		t.Errorf("ArchivePath mismatch: got %s, want %s", loaded.ArchivePath, manifest.ArchivePath)
	}
	if loaded.SHA256 != manifest.SHA256 {
		t.Errorf("SHA256 mismatch: got %s, want %s", loaded.SHA256, manifest.SHA256)
	}
	if loaded.CompressionType != manifest.CompressionType {
		t.Errorf("CompressionType mismatch: got %s, want %s", loaded.CompressionType, manifest.CompressionType)
	}
	if loaded.CompressionLevel != manifest.CompressionLevel {
		t.Errorf("CompressionLevel mismatch: got %d, want %d", loaded.CompressionLevel, manifest.CompressionLevel)
	}
	if loaded.Hostname != manifest.Hostname {
		t.Errorf("Hostname mismatch: got %s, want %s", loaded.Hostname, manifest.Hostname)
	}
}
