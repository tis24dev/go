package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/backup"
	"github.com/tis24dev/proxmox-backup/internal/checks"
	"github.com/tis24dev/proxmox-backup/internal/config"
	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/types"
)

// BashExecutor handles execution of bash scripts
type BashExecutor struct {
	logger         *logging.Logger
	scriptPath     string // Base path to bash scripts
	dryRun         bool
	defaultTimeout time.Duration // Default timeout for script execution
}

// NewBashExecutor creates a new BashExecutor
func NewBashExecutor(logger *logging.Logger, scriptPath string, dryRun bool) *BashExecutor {
	return &BashExecutor{
		logger:         logger,
		scriptPath:     scriptPath,
		dryRun:         dryRun,
		defaultTimeout: 30 * time.Minute, // 30 minutes default timeout
	}
}

// SetDefaultTimeout sets the default timeout for script execution
func (b *BashExecutor) SetDefaultTimeout(timeout time.Duration) {
	b.defaultTimeout = timeout
}

// ExecuteScript executes a bash script with the given arguments
func (b *BashExecutor) ExecuteScript(scriptName string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.defaultTimeout)
	defer cancel()
	return b.ExecuteScriptWithContext(ctx, scriptName, args...)
}

// ExecuteScriptWithContext executes a bash script with the given context and arguments
func (b *BashExecutor) ExecuteScriptWithContext(ctx context.Context, scriptName string, args ...string) (string, error) {
	scriptPath := filepath.Join(b.scriptPath, scriptName)

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", fmt.Errorf("script not found: %s", scriptPath)
	}

	if b.dryRun {
		b.logger.Info("[DRY RUN] Would execute: %s %s", scriptPath, strings.Join(args, " "))
		return "[dry-run]", nil
	}

	b.logger.Debug("Executing bash script: %s %s", scriptPath, strings.Join(args, " "))

	// Create command with context
	cmd := exec.CommandContext(ctx, "/bin/bash", append([]string{scriptPath}, args...)...)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Log output
	if stdout.Len() > 0 {
		b.logger.Debug("Script stdout: %s", stdout.String())
	}
	if stderr.Len() > 0 {
		b.logger.Debug("Script stderr: %s", stderr.String())
	}

	if err != nil {
		// Check if context was canceled
		if ctx.Err() == context.DeadlineExceeded {
			return stdout.String(), fmt.Errorf("script execution timeout after %v: %s", b.defaultTimeout, scriptPath)
		}
		if ctx.Err() == context.Canceled {
			return stdout.String(), fmt.Errorf("script execution canceled: %s", scriptPath)
		}
		return stdout.String(), fmt.Errorf("script execution failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

// ExecuteScriptWithEnv executes a bash script with custom environment variables
func (b *BashExecutor) ExecuteScriptWithEnv(scriptName string, env map[string]string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.defaultTimeout)
	defer cancel()
	return b.ExecuteScriptWithEnvContext(ctx, scriptName, env, args...)
}

// ExecuteScriptWithEnvContext executes a bash script with context and custom environment variables
func (b *BashExecutor) ExecuteScriptWithEnvContext(ctx context.Context, scriptName string, env map[string]string, args ...string) (string, error) {
	scriptPath := filepath.Join(b.scriptPath, scriptName)

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", fmt.Errorf("script not found: %s", scriptPath)
	}

	if b.dryRun {
		b.logger.Info("[DRY RUN] Would execute: %s %s with env: %v", scriptPath, strings.Join(args, " "), env)
		return "[dry-run]", nil
	}

	b.logger.Debug("Executing bash script with env: %s %s", scriptPath, strings.Join(args, " "))

	// Create command with context
	cmd := exec.CommandContext(ctx, "/bin/bash", append([]string{scriptPath}, args...)...)

	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Log output
	if stdout.Len() > 0 {
		b.logger.Debug("Script stdout: %s", stdout.String())
	}
	if stderr.Len() > 0 {
		b.logger.Debug("Script stderr: %s", stderr.String())
	}

	if err != nil {
		// Check if context was canceled
		if ctx.Err() == context.DeadlineExceeded {
			return stdout.String(), fmt.Errorf("script execution timeout after %v: %s", b.defaultTimeout, scriptPath)
		}
		if ctx.Err() == context.Canceled {
			return stdout.String(), fmt.Errorf("script execution canceled: %s", scriptPath)
		}
		return stdout.String(), fmt.Errorf("script execution failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

// ValidateScript checks if a bash script is valid and executable
func (b *BashExecutor) ValidateScript(scriptName string) error {
	scriptPath := filepath.Join(b.scriptPath, scriptName)

	// Check if file exists
	info, err := os.Stat(scriptPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("script not found: %s", scriptPath)
	}
	if err != nil {
		return fmt.Errorf("failed to stat script: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", scriptPath)
	}

	// Check if it's executable
	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("script is not executable: %s", scriptPath)
	}

	return nil
}

// BackupError represents a backup error with specific phase and exit code
type BackupError struct {
	Phase string         // "collection", "archive", "compression", "verification"
	Err   error          // Underlying error
	Code  types.ExitCode // Specific exit code
}

func (e *BackupError) Error() string {
	return fmt.Sprintf("%s phase failed: %v", e.Phase, e.Err)
}

func (e *BackupError) Unwrap() error {
	return e.Err
}

// BackupStats contains statistics from backup operations
type BackupStats struct {
	Hostname             string
	ProxmoxType          types.ProxmoxType
	Timestamp            string
	Version              string
	StartTime            time.Time
	EndTime              time.Time
	FilesCollected       int
	FilesFailed          int
	DirsCreated          int
	BytesCollected       int64
	ArchiveSize          int64
	Duration             time.Duration
	ArchivePath          string
	RequestedCompression types.CompressionType
	Compression          types.CompressionType
	CompressionLevel     int
	ReportPath           string
	ManifestPath         string
	Checksum             string
}

// Orchestrator coordinates the backup process using both Go and Bash components
type Orchestrator struct {
	bashExecutor *BashExecutor
	checker      *checks.Checker
	logger       *logging.Logger
	cfg          *config.Config
	version      string
	dryRun       bool

	// Backup configuration
	backupPath         string
	logPath            string
	compressionType    types.CompressionType
	compressionLevel   int
	compressionThreads int
	excludePatterns    []string
	optimizationCfg    backup.OptimizationConfig

	storageTargets       []StorageTarget
	notificationChannels []NotificationChannel
}

// New creates a new Orchestrator
func New(logger *logging.Logger, scriptPath string, dryRun bool) *Orchestrator {
	return &Orchestrator{
		bashExecutor:         NewBashExecutor(logger, scriptPath, dryRun),
		logger:               logger,
		dryRun:               dryRun,
		storageTargets:       make([]StorageTarget, 0),
		notificationChannels: make([]NotificationChannel, 0),
	}
}

// RunBackup coordinates the full backup process
// Currently calls the main bash backup script (proxmox-backup.sh)
// This is a hybrid approach during migration
func (o *Orchestrator) RunBackup(ctx context.Context, pType types.ProxmoxType) error {
	o.logger.Info("Starting backup orchestration for %s", pType)

	if o.dryRun {
		o.logger.Info("[DRY RUN] Backup orchestration would call proxmox-backup.sh")
		return nil
	}

	// Phase 2: Call the main bash script
	// This maintains compatibility while we migrate incrementally
	mainScript := "proxmox-backup.sh"

	// Validate script exists and is executable
	if err := o.bashExecutor.ValidateScript(mainScript); err != nil {
		o.logger.Warning("Main backup script not found or not executable: %v", err)
		o.logger.Warning("Skipping backup execution - this is expected in test/dev environments")
		return nil
	}

	o.logger.Info("Executing main backup script: %s", mainScript)

	// Execute with a longer timeout for backup operations
	// Combine parent context with timeout
	execCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()

	output, err := o.bashExecutor.ExecuteScriptWithContext(execCtx, mainScript)
	if err != nil {
		// Check if parent context was canceled (e.g., SIGINT)
		if ctx.Err() == context.Canceled {
			o.logger.Warning("Backup canceled by user")
			return fmt.Errorf("backup canceled: %w", ctx.Err())
		}
		o.logger.Error("Backup script execution failed: %v", err)
		return fmt.Errorf("backup failed: %w", err)
	}

	o.logger.Debug("Backup script output: %s", output)
	o.logger.Info("Backup completed successfully")

	return nil
}

// GetBashExecutor returns the underlying bash executor
func (o *Orchestrator) GetBashExecutor() *BashExecutor {
	return o.bashExecutor
}

// SetConfig attaches the loaded configuration to the orchestrator
func (o *Orchestrator) SetConfig(cfg *config.Config) {
	o.cfg = cfg
}

// SetVersion sets the current tool version (for metadata reporting)
func (o *Orchestrator) SetVersion(version string) {
	o.version = version
}

// SetChecker sets the pre-backup checker
func (o *Orchestrator) SetChecker(checker *checks.Checker) {
	o.checker = checker
}

// RunPreBackupChecks performs all pre-backup validation checks
func (o *Orchestrator) RunPreBackupChecks(ctx context.Context) error {
	if o.checker == nil {
		o.logger.Debug("No checker configured, skipping pre-backup checks")
		return nil
	}

	o.logger.Info("Running pre-backup validation checks")

	results, err := o.checker.RunAllChecks(ctx)
	if err != nil {
		o.logger.Error("Pre-backup checks failed: %v", err)
		return fmt.Errorf("pre-backup checks failed: %w", err)
	}

	// Log all check results
	for _, result := range results {
		if result.Passed {
			o.logger.Info("✓ %s: %s", result.Name, result.Message)
		} else {
			o.logger.Error("✗ %s: %s", result.Name, result.Message)
		}
	}

	o.logger.Info("All pre-backup checks passed")
	return nil
}

// ReleaseBackupLock releases the backup lock file
func (o *Orchestrator) ReleaseBackupLock() error {
	if o.checker == nil {
		return nil
	}
	return o.checker.ReleaseLock()
}

// SetBackupConfig configures paths and compression for Go-based backup
func (o *Orchestrator) SetBackupConfig(backupPath, logPath string, compression types.CompressionType, level int, threads int, excludePatterns []string) {
	o.backupPath = backupPath
	o.logPath = logPath
	o.compressionType = compression
	o.compressionLevel = level
	o.compressionThreads = threads
	o.excludePatterns = append([]string(nil), excludePatterns...)
}

// SetOptimizationConfig configures optional preprocessing (chunking/dedup/prefilter)
func (o *Orchestrator) SetOptimizationConfig(cfg backup.OptimizationConfig) {
	o.optimizationCfg = cfg
}

// RunGoBackup performs the entire backup using Go components (collector + archiver)
func (o *Orchestrator) RunGoBackup(ctx context.Context, pType types.ProxmoxType, hostname string) (*BackupStats, error) {
	o.logger.Info("Starting Go-based backup orchestration for %s", pType)

	startTime := time.Now()
	timestamp := startTime.Format("20060102-150405")
	normalizedLevel := normalizeCompressionLevel(o.compressionType, o.compressionLevel)

	stats := &BackupStats{
		Hostname:             hostname,
		ProxmoxType:          pType,
		Timestamp:            timestamp,
		Version:              o.version,
		StartTime:            startTime,
		RequestedCompression: o.compressionType,
		Compression:          o.compressionType,
		CompressionLevel:     normalizedLevel,
	}

	// Create temporary directory for collection (outside backup path)
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("proxmox-backup-%s-%s-", hostname, timestamp))
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	if o.dryRun {
		o.logger.Info("[DRY RUN] Temporary directory would be: %s", tempDir)
	} else {
		o.logger.Info("Using temporary directory: %s", tempDir)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			o.logger.Warning("Failed to remove temp directory %s: %v", tempDir, err)
		}
	}()

	// Create marker file for parity with Bash cleanup guarantees
	markerPath := filepath.Join(tempDir, ".proxmox-backup-marker")
	markerContent := fmt.Sprintf(
		"Created by PID %d on %s UTC\n",
		os.Getpid(),
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)
	if err := os.WriteFile(markerPath, []byte(markerContent), 0600); err != nil {
		return nil, fmt.Errorf("failed to create temp marker file: %w", err)
	}

	// Step 1: Collect configuration files
	o.logger.Info("Phase 1: Collecting configuration files...")
	collectorConfig := backup.GetDefaultCollectorConfig()
	collectorConfig.ExcludePatterns = append([]string(nil), o.excludePatterns...)
	if o.cfg != nil {
		applyCollectorOverrides(collectorConfig, o.cfg)
		if len(o.cfg.BackupBlacklist) > 0 {
			collectorConfig.ExcludePatterns = append(collectorConfig.ExcludePatterns, o.cfg.BackupBlacklist...)
		}
	}

	if err := collectorConfig.Validate(); err != nil {
		return nil, &BackupError{
			Phase: "config",
			Err:   err,
			Code:  types.ExitConfigError,
		}
	}

	collector := backup.NewCollector(o.logger, collectorConfig, tempDir, pType, o.dryRun)

	if err := collector.CollectAll(ctx); err != nil {
		// Return collection-specific error
		return nil, &BackupError{
			Phase: "collection",
			Err:   err,
			Code:  types.ExitCollectionError,
		}
	}

	// Get collection statistics
	collStats := collector.GetStats()
	stats.FilesCollected = collStats.FilesProcessed
	stats.FilesFailed = collStats.FilesFailed
	stats.DirsCreated = collStats.DirsCreated
	stats.BytesCollected = collStats.BytesCollected

	if err := o.writeBackupMetadata(tempDir, stats); err != nil {
		o.logger.Debug("Failed to write backup metadata: %v", err)
	}

	o.logger.Info("Collection completed: %d files (%s), %d failed, %d dirs created",
		collStats.FilesProcessed,
		backup.FormatBytes(collStats.BytesCollected),
		collStats.FilesFailed,
		collStats.DirsCreated)

	// Additional disk space check using estimated size and safety factor
	if o.checker != nil && stats.BytesCollected > 0 {
		estimatedSizeGB := float64(stats.BytesCollected) / (1024.0 * 1024.0 * 1024.0)
		// Ensure we always reserve at least a small amount
		if estimatedSizeGB < 0.001 {
			estimatedSizeGB = 0.001
		}
		result := o.checker.CheckDiskSpaceForEstimate(estimatedSizeGB)
		if result.Passed {
			o.logger.Info(result.Message)
		} else {
			errMsg := result.Message
			if errMsg == "" && result.Error != nil {
				errMsg = result.Error.Error()
			}
			if errMsg == "" {
				errMsg = "insufficient disk space"
			}
			return nil, &BackupError{
				Phase: "disk",
				Err:   fmt.Errorf(errMsg),
				Code:  types.ExitDiskSpaceError,
			}
		}
	}

	if o.optimizationCfg.Enabled() {
		o.logger.Info("Running backup optimizations on collected data...")
		if err := backup.ApplyOptimizations(ctx, o.logger, tempDir, o.optimizationCfg); err != nil {
			o.logger.Warning("Backup optimizations completed with warnings: %v", err)
		}
	}

	// Step 2: Create archive
	o.logger.Info("Phase 2: Creating compressed archive...")

	// Generate archive filename
	archiveBasename := fmt.Sprintf("%s-backup-%s", hostname, timestamp)

	archiverConfig := &backup.ArchiverConfig{
		Compression:        o.compressionType,
		CompressionLevel:   normalizedLevel,
		CompressionThreads: o.compressionThreads,
		DryRun:             o.dryRun,
	}

	if err := archiverConfig.Validate(); err != nil {
		return nil, &BackupError{
			Phase: "config",
			Err:   err,
			Code:  types.ExitConfigError,
		}
	}

	archiver := backup.NewArchiver(o.logger, archiverConfig)
	effectiveCompression := archiver.ResolveCompression()
	stats.Compression = effectiveCompression
	stats.CompressionLevel = archiver.CompressionLevel()
	archiveExt := archiver.GetArchiveExtension()
	archivePath := filepath.Join(o.backupPath, archiveBasename+archiveExt)
	if stats.RequestedCompression != stats.Compression {
		o.logger.Info("Using %s compression (requested %s)", stats.Compression, stats.RequestedCompression)
	}

	if err := archiver.CreateArchive(ctx, tempDir, archivePath); err != nil {
		phase := "archive"
		code := types.ExitArchiveError
		var compressionErr *backup.CompressionError
		if errors.As(err, &compressionErr) {
			phase = "compression"
			code = types.ExitCompressionError
		}

		return nil, &BackupError{
			Phase: phase,
			Err:   err,
			Code:  code,
		}
	}

	stats.ArchivePath = archivePath

	// Get archive size
	if !o.dryRun {
		if size, err := archiver.GetArchiveSize(archivePath); err == nil {
			stats.ArchiveSize = size
			o.logger.Info("Archive created: %s (%s)", archivePath, backup.FormatBytes(size))
		} else {
			o.logger.Warning("Failed to get archive size: %v", err)
		}

		// Verify archive
		if err := archiver.VerifyArchive(ctx, archivePath); err != nil {
			// Return verification-specific error
			return nil, &BackupError{
				Phase: "verification",
				Err:   err,
				Code:  types.ExitVerificationError,
			}
		}

		// Generate checksum and manifest for the archive
		checksum, err := backup.GenerateChecksum(ctx, o.logger, archivePath)
		if err != nil {
			return nil, &BackupError{
				Phase: "verification",
				Err:   fmt.Errorf("checksum generation failed: %w", err),
				Code:  types.ExitVerificationError,
			}
		}
		stats.Checksum = checksum

		manifestPath := archivePath + ".manifest.json"
		manifestCreatedAt := time.Now().UTC()
		manifest := &backup.Manifest{
			ArchivePath:      archivePath,
			ArchiveSize:      stats.ArchiveSize,
			SHA256:           checksum,
			CreatedAt:        manifestCreatedAt,
			CompressionType:  string(stats.Compression),
			CompressionLevel: stats.CompressionLevel,
			ProxmoxType:      string(stats.ProxmoxType),
			Hostname:         stats.Hostname,
		}

		if err := backup.CreateManifest(ctx, o.logger, manifest, manifestPath); err != nil {
			return nil, &BackupError{
				Phase: "verification",
				Err:   fmt.Errorf("manifest creation failed: %w", err),
				Code:  types.ExitVerificationError,
			}
		}
		stats.ManifestPath = manifestPath
		stats.EndTime = time.Now()
	} else {
		o.logger.Info("[DRY RUN] Would create archive: %s", archivePath)
		stats.EndTime = time.Now()
	}

	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	if !o.dryRun {
		if err := o.dispatchPostBackup(ctx, stats); err != nil {
			return nil, err
		}
	}

	o.logger.Info("Go backup completed in %s", backup.FormatDuration(stats.Duration))

	return stats, nil
}

func normalizeCompressionLevel(comp types.CompressionType, level int) int {
	const defaultLevel = 6

	switch comp {
	case types.CompressionGzip:
		if level < 1 || level > 9 {
			return defaultLevel
		}
	case types.CompressionXZ:
		if level < 0 || level > 9 {
			return defaultLevel
		}
	case types.CompressionZstd:
		if level < 1 || level > 22 {
			return defaultLevel
		}
	case types.CompressionNone:
		return 0
	default:
		return level
	}
	return level
}

// SaveStatsReport writes a JSON report with backup statistics to the log directory.
func (o *Orchestrator) SaveStatsReport(stats *BackupStats) error {
	if stats == nil {
		return fmt.Errorf("stats cannot be nil")
	}

	if o.logPath == "" || stats.Timestamp == "" {
		return nil
	}

	reportPath := filepath.Join(o.logPath, fmt.Sprintf("backup-stats-%s.json", stats.Timestamp))
	stats.ReportPath = reportPath

	if o.dryRun {
		o.logger.Info("[DRY RUN] Would write stats report: %s", reportPath)
		return nil
	}

	if err := os.MkdirAll(o.logPath, 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	file, err := os.OpenFile(reportPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("create stats report: %w", err)
	}
	defer file.Close()

	durationSeconds := stats.Duration.Seconds()
	var compressionRatio float64
	if stats.BytesCollected > 0 {
		compressionRatio = float64(stats.ArchiveSize) / float64(stats.BytesCollected)
	}

	payload := struct {
		Hostname          string                `json:"hostname"`
		ProxmoxType       types.ProxmoxType     `json:"proxmox_type"`
		Timestamp         string                `json:"timestamp"`
		StartTime         time.Time             `json:"start_time"`
		EndTime           time.Time             `json:"end_time"`
		DurationSeconds   float64               `json:"duration_seconds"`
		DurationHuman     string                `json:"duration_human"`
		FilesCollected    int                   `json:"files_collected"`
		FilesFailed       int                   `json:"files_failed"`
		DirsCreated       int                   `json:"directories_created"`
		BytesCollected    int64                 `json:"bytes_collected"`
		BytesCollectedStr string                `json:"bytes_collected_human"`
		ArchivePath       string                `json:"archive_path"`
		ArchiveSize       int64                 `json:"archive_size"`
		ArchiveSizeStr    string                `json:"archive_size_human"`
		RequestedComp     types.CompressionType `json:"requested_compression"`
		Compression       types.CompressionType `json:"compression"`
		CompressionLevel  int                   `json:"compression_level"`
		CompressionRatio  float64               `json:"compression_ratio"`
		CompressionPct    float64               `json:"compression_ratio_percent"`
		Checksum          string                `json:"checksum"`
		ManifestPath      string                `json:"manifest_path"`
	}{
		Hostname:          stats.Hostname,
		ProxmoxType:       stats.ProxmoxType,
		Timestamp:         stats.Timestamp,
		StartTime:         stats.StartTime,
		EndTime:           stats.EndTime,
		DurationSeconds:   durationSeconds,
		DurationHuman:     backup.FormatDuration(stats.Duration),
		FilesCollected:    stats.FilesCollected,
		FilesFailed:       stats.FilesFailed,
		DirsCreated:       stats.DirsCreated,
		BytesCollected:    stats.BytesCollected,
		BytesCollectedStr: backup.FormatBytes(stats.BytesCollected),
		ArchivePath:       stats.ArchivePath,
		ArchiveSize:       stats.ArchiveSize,
		ArchiveSizeStr:    backup.FormatBytes(stats.ArchiveSize),
		RequestedComp:     stats.RequestedCompression,
		Compression:       stats.Compression,
		CompressionLevel:  stats.CompressionLevel,
		CompressionRatio:  compressionRatio,
		CompressionPct:    compressionRatio * 100,
		Checksum:          stats.Checksum,
		ManifestPath:      stats.ManifestPath,
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("write stats report: %w", err)
	}

	o.logger.Info("Backup stats written to %s", reportPath)
	return nil
}

func (o *Orchestrator) writeBackupMetadata(tempDir string, stats *BackupStats) error {
	infoDir := filepath.Join(tempDir, "var/lib/proxmox-backup-info")
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return err
	}

	version := strings.TrimSpace(stats.Version)
	if version == "" {
		version = "0.0.0"
	}

	builder := strings.Builder{}
	builder.WriteString("# Proxmox Backup Metadata\n")
	builder.WriteString("# This file enables selective restore functionality in newer restore scripts\n")
	builder.WriteString(fmt.Sprintf("VERSION=%s\n", version))
	builder.WriteString(fmt.Sprintf("BACKUP_TYPE=%s\n", stats.ProxmoxType.String()))
	builder.WriteString(fmt.Sprintf("TIMESTAMP=%s\n", stats.Timestamp))
	builder.WriteString(fmt.Sprintf("HOSTNAME=%s\n", stats.Hostname))
	builder.WriteString("SUPPORTS_SELECTIVE_RESTORE=true\n")
	builder.WriteString("BACKUP_FEATURES=selective_restore,category_mapping,version_detection,auto_directory_creation\n")

	target := filepath.Join(infoDir, "backup_metadata.txt")
	if err := os.WriteFile(target, []byte(builder.String()), 0640); err != nil {
		return err
	}
	return nil
}

func applyCollectorOverrides(cc *backup.CollectorConfig, cfg *config.Config) {
	cc.BackupVMConfigs = cfg.BackupVMConfigs
	cc.BackupClusterConfig = cfg.BackupClusterConfig
	cc.BackupPVEFirewall = cfg.BackupPVEFirewall
	cc.BackupVZDumpConfig = cfg.BackupVZDumpConfig
	cc.BackupPVEACL = cfg.BackupPVEACL
	cc.BackupPVEJobs = cfg.BackupPVEJobs
	cc.BackupPVESchedules = cfg.BackupPVESchedules
	cc.BackupPVEReplication = cfg.BackupPVEReplication
	cc.BackupPVEBackupFiles = cfg.BackupPVEBackupFiles
	cc.BackupCephConfig = cfg.BackupCephConfig

	cc.BackupDatastoreConfigs = cfg.BackupDatastoreConfigs
	cc.BackupUserConfigs = cfg.BackupUserConfigs
	cc.BackupRemoteConfigs = cfg.BackupRemoteConfigs
	cc.BackupSyncJobs = cfg.BackupSyncJobs
	cc.BackupVerificationJobs = cfg.BackupVerificationJobs
	cc.BackupTapeConfigs = cfg.BackupTapeConfigs
	cc.BackupPruneSchedules = cfg.BackupPruneSchedules
	cc.BackupPxarFiles = cfg.BackupPxarFiles

	cc.BackupNetworkConfigs = cfg.BackupNetworkConfigs
	cc.BackupAptSources = cfg.BackupAptSources
	cc.BackupCronJobs = cfg.BackupCronJobs
	cc.BackupSystemdServices = cfg.BackupSystemdServices
	cc.BackupSSLCerts = cfg.BackupSSLCerts
	cc.BackupSysctlConfig = cfg.BackupSysctlConfig
	cc.BackupKernelModules = cfg.BackupKernelModules
	cc.BackupFirewallRules = cfg.BackupFirewallRules
	cc.BackupInstalledPackages = cfg.BackupInstalledPackages
	cc.BackupScriptDir = cfg.BackupScriptDir
	cc.BackupCriticalFiles = cfg.BackupCriticalFiles
	cc.BackupSSHKeys = cfg.BackupSSHKeys
	cc.BackupZFSConfig = cfg.BackupZFSConfig
	cc.BackupRootHome = cfg.BackupRootHome
	cc.BackupScriptRepository = cfg.BackupScriptRepository
	cc.BackupUserHomes = cfg.BackupUserHomes
	cc.ScriptRepositoryPath = cfg.BaseDir

	cc.CustomBackupPaths = append([]string(nil), cfg.CustomBackupPaths...)
	cc.BackupBlacklist = append([]string(nil), cfg.BackupBlacklist...)
}
