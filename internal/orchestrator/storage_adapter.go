package orchestrator

import (
	"context"
	"fmt"

	"github.com/tis24dev/proxmox-backup/internal/config"
	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/storage"
	"github.com/tis24dev/proxmox-backup/internal/types"
)

// StorageAdapter adapts a storage.Storage backend to the StorageTarget interface
type StorageAdapter struct {
	backend      storage.Storage
	logger       *logging.Logger
	config       *config.Config // Main configuration for retention policy
	fsInfo       *storage.FilesystemInfo
	initialStats *storage.StorageStats
}

// NewStorageAdapter creates a new storage adapter
func NewStorageAdapter(backend storage.Storage, logger *logging.Logger, cfg *config.Config) *StorageAdapter {
	return &StorageAdapter{
		backend: backend,
		logger:  logger,
		config:  cfg,
	}
}

// SetFilesystemInfo preloads filesystem info detected earlier.
func (s *StorageAdapter) SetFilesystemInfo(info *storage.FilesystemInfo) {
	if info != nil {
		s.fsInfo = info
	}
}

// SetInitialStats caches storage stats gathered during initialization.
func (s *StorageAdapter) SetInitialStats(stats *storage.StorageStats) {
	s.initialStats = stats
}

// Sync implements the StorageTarget interface
// It performs filesystem detection, stores the backup, and applies retention
func (s *StorageAdapter) Sync(ctx context.Context, stats *BackupStats) error {
	// Check if backend is enabled
	if !s.backend.IsEnabled() {
		s.logger.Debug("%s is disabled, skipping", s.backend.Name())
		return nil
	}

	s.logger.Debug("Starting %s operations...", s.backend.Name())

	// Step 1: Detect filesystem and log in real-time
	var err error
	fsInfo := s.fsInfo
	if fsInfo == nil {
		fsInfo, err = s.backend.DetectFilesystem(ctx)
		if err != nil {
			if s.backend.IsCritical() {
				return fmt.Errorf("%s filesystem detection failed (CRITICAL): %w", s.backend.Name(), err)
			}
			s.logger.Warning("WARNING: %s filesystem detection failed: %v", s.backend.Name(), err)
			s.logger.Warning("WARNING: %s operations will be skipped", s.backend.Name())
			return nil
		}
		s.fsInfo = fsInfo
	}

	// Step 2: Prepare backup metadata
	metadata := &types.BackupMetadata{
		BackupFile:  stats.ArchivePath,
		Timestamp:   stats.StartTime,
		Size:        stats.ArchiveSize,
		Checksum:    stats.Checksum,
		ProxmoxType: stats.ProxmoxType,
		Compression: stats.Compression,
		Version:     stats.Version,
	}

	// Step 3: Store backup
	s.logger.Info("%s: Storing backup...", s.backend.Name())
	hasWarnings := false
	if err := s.backend.Store(ctx, stats.ArchivePath, metadata); err != nil {
		// Check if error is critical
		if s.backend.IsCritical() {
			return fmt.Errorf("%s store operation failed (CRITICAL): %w", s.backend.Name(), err)
		}

		// Non-critical error - log warning and continue
		s.logger.Warning("WARNING: %s store operation failed: %v", s.backend.Name(), err)
		s.logger.Warning("WARNING: Backup was not saved to %s", s.backend.Name())
		hasWarnings = true
		// Don't return error - continue with retention
	} else {
		s.logger.Info("✓ %s: Backup stored successfully", s.backend.Name())
	}

	// Step 4: Apply retention policy
	retentionConfig := storage.NewRetentionConfigFromConfig(s.config, s.backend.Location())
	if retentionConfig.MaxBackups > 0 || retentionConfig.Policy == "gfs" {
		if retentionConfig.Policy == "gfs" {
			s.logger.Info("%s: Applying GFS retention policy...", s.backend.Name())
		} else {
			s.logger.Info("%s: Applying retention policy...", s.backend.Name())
		}
		s.logRetentionPolicyDetails(retentionConfig)

		s.logCurrentBackupCount()
		deleted, err := s.backend.ApplyRetention(ctx, retentionConfig)
		if err != nil {
			// Check if error is critical
			if s.backend.IsCritical() {
				return fmt.Errorf("%s retention failed (CRITICAL): %w", s.backend.Name(), err)
			}

			// Non-critical error - log warning and continue
			s.logger.Warning("WARNING: %s retention failed: %v", s.backend.Name(), err)
			hasWarnings = true
		} else if deleted > 0 {
			s.logger.Info("✓ %s: Deleted %d old backups", s.backend.Name(), deleted)
		}
	}

	// Step 5: Get and log statistics
	storageStats, err := s.backend.GetStats(ctx)
	if err != nil {
		s.logger.Debug("%s: Failed to get statistics: %v", s.backend.Name(), err)
	} else {
		s.logger.Info("%s statistics:", s.backend.Name())
		s.logger.Info("  Total backups: %d", storageStats.TotalBackups)
		if storageStats.TotalSize > 0 {
			s.logger.Info("  Total size: %s", formatBytes(storageStats.TotalSize))
		}
		if fsInfo != nil {
			s.logger.Info("  Filesystem: %s", fsInfo.Type)
		}

		if stats != nil {
			s.applyStorageStats(storageStats, stats)
		}
	}

	if hasWarnings {
		s.logger.Warning("✗ %s operations completed with warnings", s.backend.Name())
	} else {
		s.logger.Info("✓ %s operations completed", s.backend.Name())
	}
	return nil
}

func (s *StorageAdapter) logCurrentBackupCount() {
	listable, ok := s.backend.(interface {
		List(context.Context) ([]*types.BackupMetadata, error)
	})
	if !ok {
		return
	}

	backups, err := listable.List(context.Background())
	if err != nil {
		s.logger.Debug("%s: Unable to count backups prior to retention: %v", s.backend.Name(), err)
		return
	}
	s.logger.Debug("%s: Current backups detected: %d", s.backend.Name(), len(backups))
}

func (s *StorageAdapter) logRetentionPolicyDetails(cfg storage.RetentionConfig) {
	if s.logger == nil {
		return
	}
	if cfg.Policy == "gfs" {
		s.logger.Info("  Policy: GFS (daily=%d, weekly=%d, monthly=%d, yearly=%d)",
			cfg.Daily, cfg.Weekly, cfg.Monthly, cfg.Yearly)
		return
	}
	if cfg.MaxBackups > 0 {
		s.logger.Info("  Policy: simple (keep %d newest)", cfg.MaxBackups)
	} else {
		s.logger.Info("  Policy: simple (disabled)")
	}
}

// formatBytes formats bytes in human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (s *StorageAdapter) applyStorageStats(storageStats *storage.StorageStats, stats *BackupStats) {
	if storageStats == nil || stats == nil {
		return
	}

	switch s.backend.Location() {
	case storage.LocationPrimary:
		stats.LocalBackups = storageStats.TotalBackups
		stats.LocalFreeSpace = clampInt64ToUint64(storageStats.AvailableSpace)
		stats.LocalTotalSpace = clampInt64ToUint64(storageStats.TotalSpace)
	case storage.LocationSecondary:
		if !stats.SecondaryEnabled {
			stats.SecondaryEnabled = true
		}
		stats.SecondaryBackups = storageStats.TotalBackups
		stats.SecondaryFreeSpace = clampInt64ToUint64(storageStats.AvailableSpace)
		stats.SecondaryTotalSpace = clampInt64ToUint64(storageStats.TotalSpace)
	case storage.LocationCloud:
		if !stats.CloudEnabled {
			stats.CloudEnabled = true
		}
		stats.CloudBackups = storageStats.TotalBackups
	}
}

func clampInt64ToUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
