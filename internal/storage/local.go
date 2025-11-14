package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/config"
	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/types"
)

// LocalStorage implements the Storage interface for local filesystem storage
type LocalStorage struct {
	config     *config.Config
	logger     *logging.Logger
	basePath   string
	fsDetector *FilesystemDetector
	fsInfo     *FilesystemInfo
}

// NewLocalStorage creates a new local storage instance
func NewLocalStorage(cfg *config.Config, logger *logging.Logger) (*LocalStorage, error) {
	return &LocalStorage{
		config:     cfg,
		logger:     logger,
		basePath:   cfg.BackupPath,
		fsDetector: NewFilesystemDetector(logger),
	}, nil
}

// Name returns the storage backend name
func (l *LocalStorage) Name() string {
	return "Local Storage"
}

// Location returns the backup location type
func (l *LocalStorage) Location() BackupLocation {
	return LocationPrimary
}

// IsEnabled returns true if local storage is configured
func (l *LocalStorage) IsEnabled() bool {
	return l.basePath != ""
}

// IsCritical returns true because local storage is critical
func (l *LocalStorage) IsCritical() bool {
	return true
}

// DetectFilesystem detects the filesystem type for the backup path
func (l *LocalStorage) DetectFilesystem(ctx context.Context) (*FilesystemInfo, error) {
	// Ensure directory exists
	if err := os.MkdirAll(l.basePath, 0700); err != nil {
		return nil, &StorageError{
			Location:   LocationPrimary,
			Operation:  "detect_filesystem",
			Path:       l.basePath,
			Err:        err,
			IsCritical: true,
		}
	}

	fsInfo, err := l.fsDetector.DetectFilesystem(ctx, l.basePath)
	if err != nil {
		return nil, &StorageError{
			Location:   LocationPrimary,
			Operation:  "detect_filesystem",
			Path:       l.basePath,
			Err:        err,
			IsCritical: true,
		}
	}

	l.fsInfo = fsInfo
	return fsInfo, nil
}

// Store stores a backup file to local storage
// For local storage, this mainly involves setting proper permissions
func (l *LocalStorage) Store(ctx context.Context, backupFile string, metadata *types.BackupMetadata) error {
	l.logger.Debug("Local storage: preparing to store %s", filepath.Base(backupFile))
	// Check context
	if err := ctx.Err(); err != nil {
		l.logger.Debug("Local storage: store aborted due to context cancellation")
		return err
	}

	// Verify file exists
	if _, err := os.Stat(backupFile); err != nil {
		l.logger.Debug("Local storage: source file %s not found", backupFile)
		return &StorageError{
			Location:   LocationPrimary,
			Operation:  "store",
			Path:       backupFile,
			Err:        fmt.Errorf("backup file not found: %w", err),
			IsCritical: true,
		}
	}

	// Set proper permissions on the backup file
	l.logger.Debug("Local storage: setting ownership/permissions on %s", filepath.Base(backupFile))
	if err := l.fsDetector.SetPermissions(ctx, backupFile, 0, 0, 0600, l.fsInfo); err != nil {
		l.logger.Warning("Failed to set permissions on %s: %v", backupFile, err)
		// Not critical - continue
	}

	l.logger.Debug("Backup stored successfully in local storage: %s", backupFile)

	if count := l.countBackups(ctx); count >= 0 {
		l.logger.Debug("Local storage: current backups detected after archive creation: %d", count)
	} else {
		l.logger.Debug("Local storage: unable to count backups after archive creation (see previous log for details)")
	}

	return nil
}

func (l *LocalStorage) countBackups(ctx context.Context) int {
	backups, err := l.List(ctx)
	if err != nil {
		l.logger.Debug("Local storage: failed to list backups for recount: %v", err)
		return -1
	}
	return len(backups)
}

// executeTarCommand executes a tar command
func (l *LocalStorage) executeTarCommand(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar command failed: %w: %s", err, string(output))
	}

	return nil
}

// List returns all backups in local storage
func (l *LocalStorage) List(ctx context.Context) ([]*types.BackupMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Find all backup files (legacy "proxmox-backup-*.tar.*" or new "*-backup-*.tar*")
	globPatterns := []string{
		filepath.Join(l.basePath, "proxmox-backup-*.tar.*"), // Legacy Bash naming
		filepath.Join(l.basePath, "*-backup-*.tar*"),        // Go pipeline naming (+ bundle)
	}

	var matches []string
	seen := make(map[string]struct{})
	for _, pattern := range globPatterns {
		patternMatches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, &StorageError{
				Location:   LocationPrimary,
				Operation:  "list",
				Path:       l.basePath,
				Err:        err,
				IsCritical: true,
			}
		}
		for _, match := range patternMatches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			matches = append(matches, match)
		}
	}

	var backups []*types.BackupMetadata

	// Filter and parse backup files
	for _, match := range matches {
		// Skip associated files (.sha256, .metadata)
		if strings.HasSuffix(match, ".sha256") ||
			strings.HasSuffix(match, ".metadata") {
			continue
		}

		// Parse metadata if available
		metadata, err := l.loadMetadata(match)
		if err != nil {
			l.logger.Warning("Failed to load metadata for %s: %v", match, err)
			// Create minimal metadata from filename
			metadata = &types.BackupMetadata{
				BackupFile: match,
			}
		}

		backups = append(backups, metadata)
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// loadMetadata loads metadata for a backup file
func (l *LocalStorage) loadMetadata(backupFile string) (*types.BackupMetadata, error) {
	metadataFile := backupFile + ".metadata"
	if !l.config.BundleAssociatedFiles {
		if _, err := os.Stat(metadataFile); err != nil {
			return nil, err
		}
	}

	// TODO: Implement metadata loading from JSON file
	// For now, return minimal metadata
	stat, err := os.Stat(backupFile)
	if err != nil {
		return nil, err
	}

	return &types.BackupMetadata{
		BackupFile: backupFile,
		Timestamp:  stat.ModTime(),
		Size:       stat.Size(),
	}, nil
}

// Delete removes a backup file and its associated files
func (l *LocalStorage) Delete(ctx context.Context, backupFile string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	l.logger.Debug("Local storage: deleting backup %s", filepath.Base(backupFile))

	// List of files to delete
	filesToDelete := []string{backupFile}

	// If bundling is enabled, only delete the bundle
	if l.config.BundleAssociatedFiles {
		bundleFile := backupFile + ".bundle.tar"
		if _, err := os.Stat(bundleFile); err == nil {
			filesToDelete = []string{bundleFile}
		}
	} else {
		// Delete associated files
		associatedFiles := []string{
			backupFile + ".sha256",
			backupFile + ".metadata",
			backupFile + ".metadata.sha256",
		}

		for _, f := range associatedFiles {
			if _, err := os.Stat(f); err == nil {
				filesToDelete = append(filesToDelete, f)
			}
		}
	}

	// Delete all files
	for _, f := range filesToDelete {
		l.logger.Debug("Local storage: removing file %s", f)
		if err := os.Remove(f); err != nil {
			l.logger.Warning("Failed to remove %s: %v", f, err)
			// Continue with other files
		}
	}

	l.logger.Debug("Local storage: deleted backup and associated files: %s", filepath.Base(backupFile))
	return nil
}

// ApplyRetention removes old backups according to retention policy
// Supports both simple (count-based) and GFS (time-distributed) policies
func (l *LocalStorage) ApplyRetention(ctx context.Context, config RetentionConfig) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	// List all backups
	l.logger.Debug("Local storage: listing backups for retention policy '%s'", config.Policy)
	backups, err := l.List(ctx)
	if err != nil {
		return 0, &StorageError{
			Location:   LocationPrimary,
			Operation:  "apply_retention",
			Path:       l.basePath,
			Err:        err,
			IsCritical: true,
		}
	}

	if len(backups) == 0 {
		l.logger.Debug("Local storage: no backups to apply retention")
		return 0, nil
	}

	// Apply appropriate retention policy
	if config.Policy == "gfs" {
		return l.applyGFSRetention(ctx, backups, config)
	}
	return l.applySimpleRetention(ctx, backups, config.MaxBackups)
}

// applyGFSRetention applies GFS (Grandfather-Father-Son) retention policy
func (l *LocalStorage) applyGFSRetention(ctx context.Context, backups []*types.BackupMetadata, config RetentionConfig) (int, error) {
	l.logger.Info("Applying GFS retention policy (daily=%d, weekly=%d, monthly=%d, yearly=%d)",
		config.Daily, config.Weekly, config.Monthly, config.Yearly)

	// Classify backups according to GFS scheme
	classification := ClassifyBackupsGFS(backups, config)

	// Get statistics
	stats := GetRetentionStats(classification)
	l.logger.Info("GFS classification → daily: %d/%d, weekly: %d/%d, monthly: %d/%d, yearly: %d/%d, to_delete: %d",
		stats[CategoryDaily], config.Daily,
		stats[CategoryWeekly], config.Weekly,
		stats[CategoryMonthly], config.Monthly,
		stats[CategoryYearly], config.Yearly,
		stats[CategoryDelete])

	// Delete backups marked for deletion
	deleted := 0
	for backup, category := range classification {
		if category != CategoryDelete {
			continue
		}

		if err := ctx.Err(); err != nil {
			return deleted, err
		}

		l.logger.Debug("Deleting old backup: %s (created: %s)",
			filepath.Base(backup.BackupFile),
			backup.Timestamp.Format("2006-01-02 15:04:05"))

		if err := l.Delete(ctx, backup.BackupFile); err != nil {
			l.logger.Warning("Failed to delete %s: %v", backup.BackupFile, err)
			continue
		}

		deleted++
	}

	remaining := len(backups) - deleted
	l.logger.Info("Local storage retention applied: deleted %d backups, %d remaining", deleted, remaining)

	return deleted, nil
}

// applySimpleRetention applies simple count-based retention policy
func (l *LocalStorage) applySimpleRetention(ctx context.Context, backups []*types.BackupMetadata, maxBackups int) (int, error) {
	if maxBackups <= 0 {
		l.logger.Debug("Retention disabled for local storage (maxBackups = %d)", maxBackups)
		return 0, nil
	}

	totalBackups := len(backups)
	if totalBackups <= maxBackups {
		l.logger.Debug("Local storage: %d backups (within retention limit of %d)", totalBackups, maxBackups)
		return 0, nil
	}

	// Calculate how many to delete
	toDelete := totalBackups - maxBackups
	l.logger.Info("Applying simple retention policy: %d backups found, limit is %d, deleting %d oldest",
		totalBackups, maxBackups, toDelete)
	l.logger.Info("Simple retention → current: %d, limit: %d, to_delete: %d",
		totalBackups, maxBackups, toDelete)

	// Delete oldest backups (already sorted newest first)
	deleted := 0
	for i := totalBackups - 1; i >= maxBackups; i-- {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}

		backup := backups[i]
		l.logger.Debug("Deleting old backup: %s (created: %s)",
			filepath.Base(backup.BackupFile),
			backup.Timestamp.Format("2006-01-02 15:04:05"))

		if err := l.Delete(ctx, backup.BackupFile); err != nil {
			l.logger.Warning("Failed to delete %s: %v", backup.BackupFile, err)
			continue
		}

		deleted++
	}

	remaining := totalBackups - deleted
	l.logger.Debug("Local storage retention applied: deleted %d backups, %d remaining", deleted, remaining)

	return deleted, nil
}

// VerifyUpload is not applicable for local storage
func (l *LocalStorage) VerifyUpload(ctx context.Context, localFile, remoteFile string) (bool, error) {
	return true, nil
}

// GetStats returns storage statistics
func (l *LocalStorage) GetStats(ctx context.Context) (*StorageStats, error) {
	backups, err := l.List(ctx)
	if err != nil {
		return nil, err
	}

	stats := &StorageStats{
		TotalBackups: len(backups),
	}

	if l.fsInfo != nil {
		stats.FilesystemType = l.fsInfo.Type
	}

	var totalSize int64
	var oldest, newest *time.Time

	for _, backup := range backups {
		totalSize += backup.Size

		if oldest == nil || backup.Timestamp.Before(*oldest) {
			t := backup.Timestamp
			oldest = &t
		}
		if newest == nil || backup.Timestamp.After(*newest) {
			t := backup.Timestamp
			newest = &t
		}
	}

	stats.TotalSize = totalSize
	stats.OldestBackup = oldest
	stats.NewestBackup = newest

	// Get available/total space using statfs
	var stat syscall.Statfs_t
	if err := syscall.Statfs(l.basePath, &stat); err == nil {
		available := int64(stat.Bavail) * int64(stat.Bsize)
		total := int64(stat.Blocks) * int64(stat.Bsize)
		if available < 0 {
			available = 0
		}
		if total < 0 {
			total = 0
		}
		stats.AvailableSpace = available
		stats.TotalSpace = total
		used := total - available
		if used < 0 {
			used = 0
		}
		stats.UsedSpace = used
	}

	return stats, nil
}
