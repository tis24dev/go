package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/storage"
	"github.com/tis24dev/proxmox-backup/internal/types"
)

// StorageTarget rappresenta una destinazione esterna (es. storage secondario, cloud).
type StorageTarget interface {
	Sync(ctx context.Context, stats *BackupStats) error
}

// NotificationChannel rappresenta un canale di notifica (es. Telegram, email).
type NotificationChannel interface {
	Notify(ctx context.Context, stats *BackupStats) error
}

// RegisterStorageTarget aggiunge una destinazione da eseguire dopo il backup.
func (o *Orchestrator) RegisterStorageTarget(target StorageTarget) {
	if target == nil {
		return
	}
	o.storageTargets = append(o.storageTargets, target)
}

// RegisterNotificationChannel aggiunge un canale di notifica da eseguire dopo il backup.
func (o *Orchestrator) RegisterNotificationChannel(channel NotificationChannel) {
	if channel == nil {
		return
	}
	o.notificationChannels = append(o.notificationChannels, channel)
}

func (o *Orchestrator) dispatchPostBackup(ctx context.Context, stats *BackupStats) error {
	// Phase 1: Storage operations (critical - failures abort backup)
	for _, target := range o.storageTargets {
		if err := target.Sync(ctx, stats); err != nil {
			return &BackupError{
				Phase: "storage",
				Err:   fmt.Errorf("storage target failed: %w", err),
				Code:  types.ExitStorageError,
			}
		}
	}

	// Phase 2: Notifications (non-critical - failures don't abort backup)
	// Notification errors are logged but never propagated
	if len(o.notificationChannels) > 0 {
		fmt.Println()
		o.logStep(7, "Notifications - dispatching channels")
	}
	for _, channel := range o.notificationChannels {
		_ = channel.Notify(ctx, stats) // Ignore errors - notifications are non-critical
	}

	// Phase 3: Close log file and dispatch to storage/rotation
	fmt.Println()
	o.logStep(8, "Log file management - close, copy, and rotate")
	logFilePath := o.logger.GetLogFilePath()
	if logFilePath != "" {
		o.logger.Info("Closing log file: %s", logFilePath)
		if err := o.logger.CloseLogFile(); err != nil {
			o.logger.Warning("Failed to close log file: %v", err)
		} else {
			o.logger.Debug("Log file closed successfully")

			// Copy log to secondary and cloud storage
			if err := o.dispatchLogFile(ctx, logFilePath); err != nil {
				o.logger.Warning("Log file dispatch failed: %v", err)
			}

			// Rotate old log files
			if err := o.rotateLogFiles(ctx); err != nil {
				o.logger.Warning("Log rotation failed: %v", err)
			}
		}
	} else {
		o.logger.Debug("No log file to close (logging to stdout only)")
	}

	return nil
}

// dispatchLogFile copies the log file to secondary and cloud storage
func (o *Orchestrator) dispatchLogFile(ctx context.Context, logFilePath string) error {
	if o.cfg == nil {
		return nil
	}

	logFileName := filepath.Base(logFilePath)
	o.logger.Info("Dispatching log file: %s", logFileName)

	// Copy to secondary storage
	if o.cfg.SecondaryEnabled && o.cfg.SecondaryLogPath != "" {
		secondaryLogPath := filepath.Join(o.cfg.SecondaryLogPath, logFileName)
		o.logger.Debug("Copying log to secondary: %s", secondaryLogPath)

		if err := os.MkdirAll(o.cfg.SecondaryLogPath, 0755); err != nil {
			o.logger.Warning("Failed to create secondary log directory: %v", err)
		} else {
			if err := copyFile(logFilePath, secondaryLogPath); err != nil {
				o.logger.Warning("Failed to copy log to secondary: %v", err)
			} else {
				o.logger.Info("✓ Log copied to secondary: %s", secondaryLogPath)
			}
		}
	}

	// Copy to cloud storage
	if o.cfg.CloudEnabled {
		if cloudBase := strings.TrimSpace(o.cfg.CloudLogPath); cloudBase != "" {
			destination := buildCloudLogDestination(cloudBase, logFileName)
			o.logger.Debug("Copying log to cloud: %s", destination)

			if err := o.copyLogToCloud(ctx, logFilePath, destination); err != nil {
				o.logger.Warning("Failed to copy log to cloud: %v", err)
			} else {
				o.logger.Info("✓ Log copied to cloud: %s", destination)
			}
		}
	}

	return nil
}

// copyLogToCloud copies a log file to cloud storage using rclone
func (o *Orchestrator) copyLogToCloud(ctx context.Context, sourcePath, destPath string) error {
	if !strings.Contains(destPath, ":") {
		return fmt.Errorf("cloud log path must include an rclone remote (es. remote:/logs): %s", destPath)
	}

	client, err := storage.NewCloudStorage(o.cfg, o.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize cloud storage: %w", err)
	}

	return client.UploadToRemotePath(ctx, sourcePath, destPath, true)
}

// rotateLogFiles removes old log files based on retention policies
func (o *Orchestrator) rotateLogFiles(ctx context.Context) error {
	if o.cfg == nil {
		return nil
	}

	o.logger.Info("Rotating log files...")

	// Rotate local logs (use same retention as backups)
	if err := o.rotateLogsInPath(o.cfg.LogPath, o.cfg.MaxLocalBackups); err != nil {
		o.logger.Warning("Failed to rotate local logs: %v", err)
	} else {
		o.logger.Debug("✓ Local logs rotated (max: %d)", o.cfg.MaxLocalBackups)
	}

	// Rotate secondary logs (use same retention as backups)
	if o.cfg.SecondaryEnabled && o.cfg.SecondaryLogPath != "" {
		if err := o.rotateLogsInPath(o.cfg.SecondaryLogPath, o.cfg.MaxSecondaryBackups); err != nil {
			o.logger.Warning("Failed to rotate secondary logs: %v", err)
		} else {
			o.logger.Debug("✓ Secondary logs rotated (max: %d)", o.cfg.MaxSecondaryBackups)
		}
	}

	// Rotate cloud logs (use same retention as backups)
	if o.cfg.CloudEnabled && o.cfg.CloudLogPath != "" {
		if err := o.rotateCloudLogs(ctx, o.cfg.CloudLogPath, o.cfg.MaxCloudBackups); err != nil {
			o.logger.Warning("Failed to rotate cloud logs: %v", err)
		} else {
			o.logger.Debug("✓ Cloud logs rotated (max: %d)", o.cfg.MaxCloudBackups)
		}
	}

	return nil
}

// rotateLogsInPath removes old log files in a local directory
func (o *Orchestrator) rotateLogsInPath(logPath string, maxLogs int) error {
	if maxLogs <= 0 {
		return nil
	}

	// Find all log files
	pattern := filepath.Join(logPath, "backup-*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob failed: %w", err)
	}

	if len(matches) <= maxLogs {
		o.logger.Debug("Log rotation not needed: %d logs <= %d max", len(matches), maxLogs)
		return nil
	}

	// Sort by modification time (oldest first)
	sort.Slice(matches, func(i, j int) bool {
		infoI, errI := os.Stat(matches[i])
		infoJ, errJ := os.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Remove oldest logs
	toRemove := len(matches) - maxLogs
	for i := 0; i < toRemove; i++ {
		logFile := matches[i]
		o.logger.Debug("Removing old log: %s", filepath.Base(logFile))
		if err := os.Remove(logFile); err != nil {
			o.logger.Warning("Failed to remove %s: %v", filepath.Base(logFile), err)
		}
	}

	o.logger.Info("Removed %d old log file(s) from %s", toRemove, logPath)
	return nil
}

// rotateCloudLogs removes old log files from cloud storage
func (o *Orchestrator) rotateCloudLogs(ctx context.Context, cloudLogPath string, maxLogs int) error {
	if maxLogs <= 0 {
		return nil
	}

	// List log files in cloud storage
	args := []string{
		"rclone", "lsf",
		cloudLogPath,
		"--files-only",
		"--format", "pt", // path and time
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone lsf failed: %w (output: %s)", err, output)
	}

	lines := strings.Split(string(output), "\n")
	var logFiles []struct {
		path    string
		modTime time.Time
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse format: "path;time"
		parts := strings.Split(line, ";")
		if len(parts) != 2 {
			continue
		}

		path := parts[0]
		if !strings.HasPrefix(path, "backup-") || !strings.HasSuffix(path, ".log") {
			continue
		}

		timeStr := parts[1]
		modTime, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			o.logger.Debug("Failed to parse time for %s: %v", path, err)
			continue
		}

		logFiles = append(logFiles, struct {
			path    string
			modTime time.Time
		}{path: path, modTime: modTime})
	}

	if len(logFiles) <= maxLogs {
		o.logger.Debug("Cloud log rotation not needed: %d logs <= %d max", len(logFiles), maxLogs)
		return nil
	}

	// Sort by modification time (oldest first)
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].modTime.Before(logFiles[j].modTime)
	})

	// Remove oldest logs
	toRemove := len(logFiles) - maxLogs
	for i := 0; i < toRemove; i++ {
		logFile := logFiles[i].path
		cloudPath := buildCloudLogDestination(cloudLogPath, logFile)
		o.logger.Debug("Removing old cloud log: %s", logFile)

		deleteArgs := []string{"rclone", "delete", cloudPath}
		deleteCmd := exec.CommandContext(ctx, deleteArgs[0], deleteArgs[1:]...)
		if err := deleteCmd.Run(); err != nil {
			o.logger.Warning("Failed to remove cloud log %s: %v", logFile, err)
		}
	}

	o.logger.Info("Removed %d old log file(s) from cloud storage", toRemove)
	return nil
}

func buildCloudLogDestination(basePath, fileName string) string {
	base := strings.TrimSpace(basePath)
	if base == "" {
		return fileName
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, ":") {
		return base + fileName
	}
	if strings.Contains(base, ":") {
		return base + "/" + fileName
	}
	return filepath.Join(base, fileName)
}
