package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/logging"
)

// Manifest represents backup archive metadata with checksums
type Manifest struct {
	ArchivePath      string    `json:"archive_path"`
	ArchiveSize      int64     `json:"archive_size"`
	SHA256           string    `json:"sha256"`
	CreatedAt        time.Time `json:"created_at"`
	CompressionType  string    `json:"compression_type"`
	CompressionLevel int       `json:"compression_level"`
	CompressionMode  string    `json:"compression_mode,omitempty"`
	ProxmoxType      string    `json:"proxmox_type"`
	ProxmoxTargets   []string  `json:"proxmox_targets,omitempty"`
	ProxmoxVersion   string    `json:"proxmox_version,omitempty"`
	Hostname         string    `json:"hostname"`
	ScriptVersion    string    `json:"script_version,omitempty"`
	EncryptionMode   string    `json:"encryption_mode,omitempty"`
}

// GenerateChecksum calculates SHA256 checksum of a file
func GenerateChecksum(ctx context.Context, logger *logging.Logger, filePath string) (string, error) {
	logger.Debug("Generating SHA256 checksum for: %s", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()

	// Copy file to hash in chunks with context checking
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		n, err := file.Read(buf)
		if n > 0 {
			if _, err := hash.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("failed to write to hash: %w", err)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	logger.Debug("Generated checksum: %s", checksum)
	return checksum, nil
}

// CreateManifest creates a manifest file with archive metadata and checksum
func CreateManifest(ctx context.Context, logger *logging.Logger, manifest *Manifest, outputPath string) error {
	logger.Debug("Creating manifest file: %s", outputPath)

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Marshal manifest to JSON with indentation
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Write manifest file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	logger.Debug("Manifest created successfully")
	return nil
}

// VerifyChecksum verifies a file against a manifest's checksum
func VerifyChecksum(ctx context.Context, logger *logging.Logger, filePath, expectedChecksum string) (bool, error) {
	logger.Debug("Verifying checksum for: %s", filePath)

	actualChecksum, err := GenerateChecksum(ctx, logger, filePath)
	if err != nil {
		return false, fmt.Errorf("failed to generate checksum: %w", err)
	}

	matches := actualChecksum == expectedChecksum
	if matches {
		logger.Debug("Checksum verification passed")
	} else {
		logger.Warning("Checksum mismatch! Expected: %s, Got: %s", expectedChecksum, actualChecksum)
	}

	return matches, nil
}

// LoadManifest loads a manifest from a JSON file
func LoadManifest(manifestPath string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &manifest, nil
}
