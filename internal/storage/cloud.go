package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/config"
	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/types"
	"github.com/tis24dev/proxmox-backup/pkg/utils"
)

// CloudStorage implements the Storage interface for cloud storage using rclone
// All errors from cloud storage are NON-FATAL - they log warnings but don't abort the backup
// Uses comprehensible timeout names: CONNECTION (for remote check) and OPERATION (for upload/download)
type CloudStorage struct {
	config *config.Config
	logger *logging.Logger
	remote string
}

// NewCloudStorage creates a new cloud storage instance
func NewCloudStorage(cfg *config.Config, logger *logging.Logger) (*CloudStorage, error) {
	return &CloudStorage{
		config: cfg,
		logger: logger,
		remote: cfg.CloudRemote,
	}, nil
}

// Name returns the storage backend name
func (c *CloudStorage) Name() string {
	return "Cloud Storage (rclone)"
}

// Location returns the backup location type
func (c *CloudStorage) Location() BackupLocation {
	return LocationCloud
}

// IsEnabled returns true if cloud storage is configured
func (c *CloudStorage) IsEnabled() bool {
	return c.config.CloudEnabled && c.remote != ""
}

// IsCritical returns false because cloud storage is non-critical
// Failures in cloud storage should NOT abort the backup
func (c *CloudStorage) IsCritical() bool {
	return false
}

// DetectFilesystem checks if rclone is available and the remote is accessible
func (c *CloudStorage) DetectFilesystem(ctx context.Context) (*FilesystemInfo, error) {
	// Check if rclone is available
	if !c.hasRclone() {
		c.logger.Warning("WARNING: rclone not found in PATH - cloud backup will be skipped")
		c.logger.Warning("WARNING: Install rclone to enable cloud backups")
		return nil, &StorageError{
			Location:    LocationCloud,
			Operation:   "detect_filesystem",
			Path:        c.remote,
			Err:         fmt.Errorf("rclone command not found in PATH"),
			IsCritical:  false,
			Recoverable: true,
		}
	}

	// Check if remote is configured and accessible
	// Use CONNECTION timeout for this check (short timeout)
	c.logger.Info("Checking cloud remote accessibility: %s (timeout: %ds)",
		c.remote,
		c.config.RcloneTimeoutConnection)

	if err := c.checkRemoteAccessible(ctx); err != nil {
		c.logger.Warning("WARNING: Cloud remote %s is not accessible: %v", c.remote, err)
		c.logger.Warning("WARNING: Cloud backup will be skipped")
		c.logger.Warning("HINT: Check your rclone configuration with: rclone config show %s", c.remote)
		return nil, &StorageError{
			Location:    LocationCloud,
			Operation:   "check_remote",
			Path:        c.remote,
			Err:         fmt.Errorf("remote not accessible (connection timeout: %ds): %w", c.config.RcloneTimeoutConnection, err),
			IsCritical:  false,
			Recoverable: true,
		}
	}

	c.logger.Info("Cloud remote %s is accessible", c.remote)

	// Return minimal filesystem info (cloud doesn't have a real filesystem type)
	return &FilesystemInfo{
		Path:              c.remote,
		Type:              FilesystemType("rclone-" + c.remote),
		SupportsOwnership: false,
		IsNetworkFS:       true,
		MountPoint:        c.remote,
		Device:            "cloud",
	}, nil
}

// hasRclone checks if rclone command is available
func (c *CloudStorage) hasRclone() bool {
	_, err := exec.LookPath("rclone")
	return err == nil
}

// checkRemoteAccessible checks if the rclone remote is accessible
// Uses RCLONE_TIMEOUT_CONNECTION (short timeout for connection check)
func (c *CloudStorage) checkRemoteAccessible(ctx context.Context) error {
	// Create a context with connection timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(c.config.RcloneTimeoutConnection)*time.Second)
	defer cancel()

	// Try to list the root of the remote (quick check)
	args := []string{"rclone", "lsd", c.remote + ":", "--max-depth", "1"}

	c.logger.Debug("Running: %s", strings.Join(args, " "))

	cmd := exec.CommandContext(timeoutCtx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("connection timeout (%ds) - remote did not respond in time", c.config.RcloneTimeoutConnection)
		}
		return fmt.Errorf("rclone check failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// Store uploads a backup file to cloud storage using rclone
func (c *CloudStorage) Store(ctx context.Context, backupFile string, metadata *types.BackupMetadata) error {
	c.logger.Debug("Cloud storage: preparing to upload %s", filepath.Base(backupFile))
	// Check context
	if err := ctx.Err(); err != nil {
		c.logger.Debug("Cloud storage: store aborted due to context cancellation")
		return err
	}

	// Verify source file exists
	stat, err := os.Stat(backupFile)
	if err != nil {
		c.logger.Debug("Cloud storage: source file %s not found", backupFile)
		c.logger.Warning("WARNING: Cloud storage - backup file not found: %s: %v", backupFile, err)
		return &StorageError{
			Location:    LocationCloud,
			Operation:   "store",
			Path:        backupFile,
			Err:         fmt.Errorf("source file not found: %w", err),
			IsCritical:  false,
			Recoverable: false,
		}
	}

	filename := filepath.Base(backupFile)
	remoteFile := c.remote + ":" + filename

	c.logger.Info("Uploading backup to cloud storage: %s (%s) -> %s (timeout: %ds)",
		filename,
		utils.FormatBytes(stat.Size()),
		c.remote,
		c.config.RcloneTimeoutOperation)
	c.logger.Debug("Cloud storage: upload retries=%d threads=%d bwlimit=%s",
		c.config.RcloneRetries, c.config.RcloneTransfers, c.config.RcloneBandwidthLimit)

	// Use OPERATION timeout for upload (long timeout)
	uploadCtx, cancel := context.WithTimeout(ctx, time.Duration(c.config.RcloneTimeoutOperation)*time.Second)
	defer cancel()

	// Upload using rclone with retries
    if err := c.uploadWithRetry(uploadCtx, backupFile, remoteFile); err != nil {
        c.logger.Warning("WARNING: Cloud Storage: File upload failed for %s: %v", filename, err)
        c.logger.Warning("WARNING: Cloud Storage: Backup not saved to %s", c.remote)
        return &StorageError{
			Location:    LocationCloud,
			Operation:   "upload",
			Path:        backupFile,
			Err:         err,
			IsCritical:  false,
			Recoverable: true,
		}
	}

	// Verify upload
    c.logger.Info("Verifying cloud upload: %s", filename)
    verified, err := c.VerifyUpload(ctx, backupFile, remoteFile)
    if err != nil || !verified {
        c.logger.Warning("WARNING: Cloud Storage: Upload verification failed for %s: %v", filename, err)
        c.logger.Warning("WARNING: File was uploaded but could not be verified - it may be corrupt")
        return &StorageError{
			Location:    LocationCloud,
			Operation:   "verify",
			Path:        backupFile,
			Err:         fmt.Errorf("verification failed: %w", err),
			IsCritical:  false,
			Recoverable: true,
		}
	}

	// Upload associated files if not bundled
	if !c.config.BundleAssociatedFiles {
		associatedFiles := []string{
			backupFile + ".sha256",
			backupFile + ".metadata",
			backupFile + ".metadata.sha256",
		}

		for _, srcFile := range associatedFiles {
			if _, err := os.Stat(srcFile); err != nil {
				continue // Skip if doesn't exist
			}

			remoteAssocFile := c.remote + ":" + filepath.Base(srcFile)
			uploadCtx2, cancel2 := context.WithTimeout(ctx, time.Duration(c.config.RcloneTimeoutOperation)*time.Second)
			defer cancel2()

            if err := c.uploadWithRetry(uploadCtx2, srcFile, remoteAssocFile); err != nil {
                c.logger.Warning("WARNING: Cloud Storage: Failed to upload associated file %s: %v",
                    filepath.Base(srcFile), err)
                // Continue with other files
            }
		}
	} else {
		// Upload bundle file
		bundleFile := backupFile + ".bundle.tar"
		if _, err := os.Stat(bundleFile); err == nil {
			remoteBundle := c.remote + ":" + filepath.Base(bundleFile)
			uploadCtx3, cancel3 := context.WithTimeout(ctx, time.Duration(c.config.RcloneTimeoutOperation)*time.Second)
			defer cancel3()

            if err := c.uploadWithRetry(uploadCtx3, bundleFile, remoteBundle); err != nil {
                c.logger.Warning("WARNING: Cloud Storage: Failed to upload bundle %s: %v",
                    filepath.Base(bundleFile), err)
            }
        }
    }

    c.logger.Debug("✓ Cloud Storage: File uploaded")

	if count := c.countBackups(ctx); count >= 0 {
		c.logger.Debug("Cloud storage: current backups detected after upload: %d", count)
	} else {
		c.logger.Debug("Cloud storage: unable to count backups after upload (see previous log for details)")
	}

	return nil
}

func (c *CloudStorage) countBackups(ctx context.Context) int {
	backups, err := c.List(ctx)
	if err != nil {
		c.logger.Debug("Cloud storage: failed to list backups for recount: %v", err)
		return -1
	}
	return len(backups)
}

// uploadWithRetry uploads a file with automatic retry on failure
func (c *CloudStorage) uploadWithRetry(ctx context.Context, localFile, remoteFile string) error {
	var lastErr error

	for attempt := 1; attempt <= c.config.RcloneRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if attempt > 1 {
			c.logger.Info("Upload retry attempt %d/%d for %s",
				attempt,
				c.config.RcloneRetries,
				filepath.Base(localFile))
		}

		err := c.rcloneCopy(ctx, localFile, remoteFile)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			c.logger.Warning("Upload attempt %d/%d failed: operation timeout (%ds exceeded)",
				attempt,
				c.config.RcloneRetries,
				c.config.RcloneTimeoutOperation)
		} else {
			c.logger.Warning("Upload attempt %d/%d failed: %v",
				attempt,
				c.config.RcloneRetries,
				err)
		}

		// Don't retry if we've run out of time
		if ctx.Err() != nil {
			break
		}

		// Wait before retry (exponential backoff)
		if attempt < c.config.RcloneRetries {
			waitTime := time.Duration(attempt*2) * time.Second
			c.logger.Debug("Waiting %v before retry...", waitTime)
			time.Sleep(waitTime)
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("upload failed: operation timeout (%ds exceeded) after %d attempts",
			c.config.RcloneTimeoutOperation,
			c.config.RcloneRetries)
	}

	return fmt.Errorf("upload failed after %d attempts: %w",
		c.config.RcloneRetries,
		lastErr)
}

// rcloneCopy executes rclone copy command
func (c *CloudStorage) rcloneCopy(ctx context.Context, localFile, remoteFile string) error {
	args := []string{"rclone", "copy"}

	// Add bandwidth limit if configured
	if c.config.RcloneBandwidthLimit != "" {
		args = append(args, "--bwlimit", c.config.RcloneBandwidthLimit)
	}

	// Add parallel transfers
	if c.config.RcloneTransfers > 0 {
		args = append(args, "--transfers", fmt.Sprintf("%d", c.config.RcloneTransfers))
	}

	// Add progress and stats
	args = append(args, "--progress", "--stats", "10s")

	// Add source and destination
	args = append(args, localFile, filepath.Dir(remoteFile)+":")

	c.logger.Debug("Running: %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("rclone operation timeout")
		}
		return fmt.Errorf("rclone copy failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// VerifyUpload verifies that a file was successfully uploaded to cloud storage
// Uses two methods: primary (rclone lsl) and alternative (rclone ls + grep)
func (c *CloudStorage) VerifyUpload(ctx context.Context, localFile, remoteFile string) (bool, error) {
	// Get local file info
	localStat, err := os.Stat(localFile)
	if err != nil {
		return false, fmt.Errorf("cannot stat local file: %w", err)
	}

	filename := filepath.Base(remoteFile)

	// Use primary verification method by default
	if c.config.RcloneVerifyMethod != "alternative" {
		return c.verifyPrimary(ctx, remoteFile, localStat.Size(), filename)
	}

	// Use alternative verification method
	return c.verifyAlternative(ctx, remoteFile, localStat.Size(), filename)
}

// verifyPrimary uses 'rclone lsl' to verify upload (primary method)
func (c *CloudStorage) verifyPrimary(ctx context.Context, remoteFile string, expectedSize int64, filename string) (bool, error) {
	args := []string{"rclone", "lsl", remoteFile}

	c.logger.Debug("Verification (primary): %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		c.logger.Debug("Primary verification failed, trying alternative method: %v", err)
		return c.verifyAlternative(ctx, remoteFile, expectedSize, filename)
	}

	// Parse lsl output: size filename
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return false, fmt.Errorf("empty lsl output - file may not exist")
	}

	// lsl format: "SIZE DATE TIME FILENAME"
	fields := strings.Fields(outputStr)
	if len(fields) < 4 {
		return false, fmt.Errorf("unexpected lsl output format: %s", outputStr)
	}

	// First field should be size
	var remoteSize int64
	if _, err := fmt.Sscanf(fields[0], "%d", &remoteSize); err != nil {
		return false, fmt.Errorf("cannot parse remote file size: %w", err)
	}

	if remoteSize != expectedSize {
		return false, fmt.Errorf("size mismatch: local=%d remote=%d", expectedSize, remoteSize)
	}

	c.logger.Debug("Verification successful: %s (%s)", filename, utils.FormatBytes(remoteSize))
	return true, nil
}

// verifyAlternative uses 'rclone ls | grep' to verify upload (alternative method)
func (c *CloudStorage) verifyAlternative(ctx context.Context, remoteFile string, expectedSize int64, filename string) (bool, error) {
	// List files in remote directory
	remoteDir := filepath.Dir(remoteFile)
	args := []string{"rclone", "ls", remoteDir + ":"}

	c.logger.Debug("Verification (alternative): %s | grep %s", strings.Join(args, " "), filename)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return false, fmt.Errorf("rclone ls failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	// Search for filename in output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// ls format: "SIZE FILENAME"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Check if this is our file
		if fields[1] != filename {
			continue
		}

		// Parse size
		var remoteSize int64
		if _, err := fmt.Sscanf(fields[0], "%d", &remoteSize); err != nil {
			continue
		}

		if remoteSize != expectedSize {
			return false, fmt.Errorf("size mismatch: local=%d remote=%d", expectedSize, remoteSize)
		}

		c.logger.Debug("Verification successful (alternative): %s (%s)", filename, utils.FormatBytes(remoteSize))
		return true, nil
	}

	return false, fmt.Errorf("file not found in rclone ls output")
}

// List returns all backups in cloud storage
func (c *CloudStorage) List(ctx context.Context) ([]*types.BackupMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// List files in remote
	args := []string{"rclone", "lsl", c.remote + ":"}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		c.logger.Warning("WARNING: Cloud storage - failed to list backups: %v", err)
		return nil, &StorageError{
			Location:    LocationCloud,
			Operation:   "list",
			Path:        c.remote,
			Err:         fmt.Errorf("rclone lsl failed: %w", err),
			IsCritical:  false,
			Recoverable: true,
		}
	}

	var backups []*types.BackupMetadata

	// Parse lsl output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// lsl format: "SIZE DATE TIME FILENAME"
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		filename := fields[3]

		// Only include backup files (legacy `proxmox-backup-*` or Go `*-backup-*`)
		isNewName := strings.Contains(filename, "-backup-")
		isLegacy := strings.HasPrefix(filename, "proxmox-backup-")
		if !(isLegacy || isNewName) {
			continue
		}

		if !strings.Contains(filename, ".tar") {
			continue
		}

		// Skip associated files
		if strings.HasSuffix(filename, ".sha256") ||
			strings.HasSuffix(filename, ".metadata") {
			continue
		}

		// Parse size
		var size int64
		fmt.Sscanf(fields[0], "%d", &size)

		// Parse timestamp (DATE TIME)
		timestamp, _ := time.Parse("2006-01-02 15:04:05", fields[1]+" "+fields[2])

		backups = append(backups, &types.BackupMetadata{
			BackupFile: filename,
			Timestamp:  timestamp,
			Size:       size,
		})
	}

	// Sort by timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// Delete removes a backup file from cloud storage
func (c *CloudStorage) Delete(ctx context.Context, backupFile string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	filename := filepath.Base(backupFile)
	remoteFile := c.remote + ":" + filename

	c.logger.Debug("Deleting cloud backup: %s", filename)

	// List of files to delete
	filesToDelete := []string{remoteFile}

	// If bundling is enabled, only delete the bundle
	if c.config.BundleAssociatedFiles {
		bundleFile := c.remote + ":" + filename + ".bundle.tar"
		filesToDelete = []string{bundleFile}
	} else {
		// Delete associated files
		associatedFiles := []string{
			remoteFile + ".sha256",
			remoteFile + ".metadata",
			remoteFile + ".metadata.sha256",
		}

		for _, f := range associatedFiles {
			filesToDelete = append(filesToDelete, f)
		}
	}

	// Delete all files
	for _, f := range filesToDelete {
		args := []string{"rclone", "deletefile", f}

		c.logger.Debug("Running: %s", strings.Join(args, " "))

		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			c.logger.Warning("WARNING: Cloud storage - failed to delete %s: %v: %s",
				filepath.Base(f),
				err,
				strings.TrimSpace(string(output)))
			// Continue with other files
		}
	}

	c.logger.Info("Deleted cloud backup: %s", filename)
	return nil
}

// ApplyRetention removes old backups according to retention policy
// Uses batched deletion (20 files per batch with 1s pause) to avoid API rate limits
func (c *CloudStorage) ApplyRetention(ctx context.Context, maxBackups int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if maxBackups <= 0 {
		c.logger.Debug("Retention disabled for cloud storage (maxBackups = %d)", maxBackups)
		return 0, nil
	}

	// List all backups
	c.logger.Debug("Cloud storage: listing backups prior to retention (limit=%d)", maxBackups)
	backups, err := c.List(ctx)
	if err != nil {
		c.logger.Warning("WARNING: Cloud storage - failed to list backups for retention: %v", err)
		return 0, &StorageError{
			Location:    LocationCloud,
			Operation:   "apply_retention",
			Path:        c.remote,
			Err:         err,
			IsCritical:  false,
			Recoverable: true,
		}
	}

	totalBackups := len(backups)
	if totalBackups <= maxBackups {
		c.logger.Debug("Cloud storage: %d backups (within retention limit of %d)", totalBackups, maxBackups)
		return 0, nil
	}

	// Calculate how many to delete
	toDelete := totalBackups - maxBackups
	c.logger.Info("Cloud storage: %d backups found, retention limit is %d, deleting %d oldest backups",
		totalBackups, maxBackups, toDelete)

	// Delete oldest backups in batches (to avoid API rate limits)
	deleted := 0
	batchSize := c.config.CloudBatchSize
	batchPause := time.Duration(c.config.CloudBatchPause) * time.Second

	for i := totalBackups - 1; i >= maxBackups; i-- {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}

		backup := backups[i]
		c.logger.Debug("Deleting old cloud backup: %s (created: %s)",
			backup.BackupFile,
			backup.Timestamp.Format("2006-01-02 15:04:05"),
		)

		if err := c.Delete(ctx, backup.BackupFile); err != nil {
			c.logger.Warning("WARNING: Cloud storage - failed to delete %s: %v", backup.BackupFile, err)
			continue
		}

		deleted++

		// Pause after each batch to avoid API rate limits
		if deleted%batchSize == 0 && i > maxBackups {
			c.logger.Debug("Batch of %d deletions completed, pausing for %v to avoid API rate limits",
				batchSize,
				batchPause)
			time.Sleep(batchPause)
		}
	}

	remaining := totalBackups - deleted
	if recount, err := c.List(ctx); err == nil {
		remaining = len(recount)
	} else {
		c.logger.Debug("Cloud storage: failed to recount backups after retention: %v", err)
	}

	c.logger.Debug("Cloud storage retention applied: deleted %d old backups, %d remaining",
		deleted, remaining)

	return deleted, nil
}

// GetStats returns storage statistics
func (c *CloudStorage) GetStats(ctx context.Context) (*StorageStats, error) {
	backups, err := c.List(ctx)
	if err != nil {
		c.logger.Warning("WARNING: Cloud storage - failed to get stats: %v", err)
		return &StorageStats{}, nil // Return empty stats, not an error
	}

	stats := &StorageStats{
		TotalBackups:   len(backups),
		FilesystemType: FilesystemType("rclone-" + c.remote),
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

	return stats, nil
}
