package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/backup"
	"github.com/tis24dev/proxmox-backup/internal/checks"
	"github.com/tis24dev/proxmox-backup/internal/cli"
	"github.com/tis24dev/proxmox-backup/internal/config"
	"github.com/tis24dev/proxmox-backup/internal/environment"
	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/orchestrator"
	"github.com/tis24dev/proxmox-backup/internal/types"
	"github.com/tis24dev/proxmox-backup/pkg/utils"
)

const (
	version = "0.2.0-dev"
)

func main() {
	bootstrap := logging.NewBootstrapLogger()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT (Ctrl+C) and SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		bootstrap.Warning("\nReceived signal %v, initiating graceful shutdown...", sig)
		cancel() // Cancel context to stop all operations
	}()

	// Parse command-line arguments
	args := cli.Parse()

	// Handle version flag
	if args.ShowVersion {
		cli.ShowVersion()
		return
	}

	// Handle help flag
	if args.ShowHelp {
		cli.ShowHelp()
		return
	}

	// Print header
	bootstrap.Println("===========================================")
	bootstrap.Println("  Proxmox Backup - Go Version")
	bootstrap.Printf("  Version: %s", version)
	bootstrap.Println("  Phase: 4.1 - Advanced Collection")
	bootstrap.Println("===========================================")
	bootstrap.Println("")

	// Detect Proxmox environment
	bootstrap.Println("Detecting Proxmox environment...")
	envInfo, err := environment.Detect()
	if err != nil {
		bootstrap.Warning("WARNING: %v", err)
		bootstrap.Println("Continuing with limited functionality...")
	}
	bootstrap.Printf("✓ Proxmox Type: %s", envInfo.Type)
	bootstrap.Printf("  Version: %s", envInfo.Version)
	bootstrap.Println("")

	// Load configuration
	autoBaseDir, autoFound := detectBaseDir()
	if autoBaseDir == "" {
		autoBaseDir = "/opt/proxmox-backup"
	}
	initialEnvBaseDir := os.Getenv("BASE_DIR")
	if initialEnvBaseDir == "" {
		_ = os.Setenv("BASE_DIR", autoBaseDir)
	}
	bootstrap.Printf("Loading configuration from: %s", args.ConfigPath)
	cfg, err := config.LoadConfig(args.ConfigPath)
	if err != nil {
		bootstrap.Error("ERROR: Failed to load configuration: %v", err)
		os.Exit(types.ExitConfigError.Int())
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = autoBaseDir
	}
	_ = os.Setenv("BASE_DIR", cfg.BaseDir)
	bootstrap.Println("✓ Configuration loaded successfully")
	bootstrap.Println("")

	if err := validateFutureFeatures(cfg); err != nil {
		bootstrap.Error("ERROR: Invalid configuration: %v", err)
		os.Exit(types.ExitConfigError.Int())
	}

	// Determine log level (CLI overrides config)
	logLevel := cfg.DebugLevel
	if args.LogLevel != types.LogLevelNone {
		logLevel = args.LogLevel
	}

	// Initialize logger with configuration
	logger := logging.New(logLevel, cfg.UseColor)
	logging.SetDefaultLogger(logger)
	bootstrap.SetLevel(logLevel)
	bootstrap.Flush(logger)
	defer cleanupAfterRun(logger)

	if args.DryRun {
		logging.Info("DRY RUN MODE: No actual changes will be made")
	}

	// Determine base directory source for logging
	baseDirSource := "default fallback"
	if rawBaseDir, ok := cfg.Get("BASE_DIR"); ok && strings.TrimSpace(rawBaseDir) != "" {
		baseDirSource = "configured in backup.env"
	} else if initialEnvBaseDir != "" {
		baseDirSource = "from environment (BASE_DIR)"
	} else if autoFound {
		baseDirSource = "auto-detected from executable path"
	}

	// Log environment info
	logging.Info("Environment: %s %s", envInfo.Type, envInfo.Version)
	logging.Info("Backup enabled: %v", cfg.BackupEnabled)
	logging.Info("Debug level: %s", logLevel.String())
	logging.Info("Compression: %s (level %d)", cfg.CompressionType, cfg.CompressionLevel)
	logging.Info("Base directory: %s (%s)", cfg.BaseDir, baseDirSource)
	logging.Info("Backup path: %s", cfg.BackupPath)
	logging.Info("Log path: %s", cfg.LogPath)
	fmt.Println()

	// Verify directories
	logging.Info("Verifying directory structure...")
	checkDir := func(name, path string) {
		if utils.DirExists(path) {
			logging.Info("✓ %s exists: %s", name, path)
		} else {
			logging.Warning("✗ %s not found: %s", name, path)
		}
	}

	checkDir("Backup directory", cfg.BackupPath)
	checkDir("Log directory", cfg.LogPath)
	checkDir("Lock directory", cfg.LockPath)
	fmt.Println()

	// Initialize orchestrator
	logging.Info("Initializing backup orchestrator...")
	bashScriptPath := "/opt/proxmox-backup/script"
	orch := orchestrator.New(logger, bashScriptPath, args.DryRun)
	orch.SetVersion(version)
	orch.SetConfig(cfg)

	// Configure backup paths and compression
	excludePatterns := append([]string(nil), cfg.ExcludePatterns...)
	excludePatterns = addPathExclusion(excludePatterns, cfg.BackupPath)
	if cfg.SecondaryEnabled {
		excludePatterns = addPathExclusion(excludePatterns, cfg.SecondaryPath)
	}
	if cfg.CloudEnabled && isLocalPath(cfg.CloudRemote) {
		excludePatterns = addPathExclusion(excludePatterns, cfg.CloudRemote)
	}

	orch.SetBackupConfig(
		cfg.BackupPath,
		cfg.LogPath,
		cfg.CompressionType,
		cfg.CompressionLevel,
		cfg.CompressionThreads,
		excludePatterns,
	)

	orch.SetOptimizationConfig(backup.OptimizationConfig{
		EnableChunking:            cfg.EnableSmartChunking,
		EnableDeduplication:       cfg.EnableDeduplication,
		EnablePrefilter:           cfg.EnablePrefilter,
		ChunkSizeBytes:            int64(cfg.ChunkSizeMB) * 1024 * 1024,
		ChunkThresholdBytes:       int64(cfg.ChunkThresholdMB) * 1024 * 1024,
		PrefilterMaxFileSizeBytes: int64(cfg.PrefilterMaxFileSizeMB) * 1024 * 1024,
	})

	logging.Info("✓ Orchestrator initialized")
	fmt.Println()

	// Initialize pre-backup checker
	logging.Info("Configuring pre-backup validation checks...")
	checkerConfig := checks.GetDefaultCheckerConfig(cfg.BackupPath, cfg.LogPath, cfg.LockPath)
	checkerConfig.SecondaryEnabled = cfg.SecondaryEnabled
	if cfg.SecondaryEnabled && strings.TrimSpace(cfg.SecondaryPath) != "" {
		checkerConfig.SecondaryPath = cfg.SecondaryPath
	} else {
		checkerConfig.SecondaryPath = ""
	}
	checkerConfig.CloudEnabled = cfg.CloudEnabled
	if cfg.CloudEnabled && strings.TrimSpace(cfg.CloudRemote) != "" {
		checkerConfig.CloudPath = cfg.CloudRemote
	} else {
		checkerConfig.CloudPath = ""
	}
	checkerConfig.MinDiskPrimaryGB = cfg.MinDiskPrimaryGB
	checkerConfig.MinDiskSecondaryGB = cfg.MinDiskSecondaryGB
	checkerConfig.MinDiskCloudGB = cfg.MinDiskCloudGB
	checkerConfig.DryRun = args.DryRun
	if err := checkerConfig.Validate(); err != nil {
		logging.Error("Invalid checker configuration: %v", err)
		os.Exit(types.ExitConfigError.Int())
	}
	checker := checks.NewChecker(logger, checkerConfig)
	orch.SetChecker(checker)

	// Ensure lock is released on exit
	defer func() {
		if err := orch.ReleaseBackupLock(); err != nil {
			logging.Warning("Failed to release backup lock: %v", err)
		}
	}()

	logging.Info("✓ Pre-backup checks configured")
	fmt.Println()

	// Validate bash scripts exist
	logging.Info("Validating bash script environment...")
	if utils.DirExists(bashScriptPath) {
		logging.Info("✓ Bash scripts directory exists: %s", bashScriptPath)
	} else {
		logging.Warning("✗ Bash scripts directory not found: %s", bashScriptPath)
		logging.Warning("  Hybrid mode may not work correctly")
	}
	fmt.Println()

	// Storage info
	logging.Info("Storage configuration:")
	logging.Info("  Primary: %s", cfg.BackupPath)
	logging.Info("  Secondary storage: %v", cfg.SecondaryEnabled)
	if cfg.SecondaryEnabled {
		logging.Info("    Path: %s", cfg.SecondaryPath)
		logging.Info("    Retention: %d days", cfg.SecondaryRetentionDays)
	}
	logging.Info("  Cloud storage: %v", cfg.CloudEnabled)
	if cfg.CloudEnabled {
		logging.Info("    Remote: %s", cfg.CloudRemote)
		logging.Info("    Retention: %d days", cfg.CloudRetentionDays)
	}
	fmt.Println()

	// Notification info
	logging.Info("Notification configuration:")
	logging.Info("  Telegram: %v", cfg.TelegramEnabled)
	logging.Info("  Email: %v", cfg.EmailEnabled)
	logging.Info("  Metrics: %v", cfg.MetricsEnabled)
	fmt.Println()

	useGoPipeline := cfg.EnableGoBackup
	if !useGoPipeline {
		logging.Info("Go backup pipeline disabled (ENABLE_GO_BACKUP=false). Using legacy bash workflow.")
	}

	// Run backup orchestration
	if cfg.BackupEnabled {
		if useGoPipeline {
			// Run pre-backup validation checks
			logging.Info("Running pre-backup validation checks...")
			if err := orch.RunPreBackupChecks(ctx); err != nil {
				logging.Error("Pre-backup validation failed: %v", err)
				os.Exit(types.ExitBackupError.Int())
			}
			logging.Info("✓ All pre-backup checks passed")
			fmt.Println()

			logging.Info("Starting Go backup orchestration...")

			// Get hostname for backup naming
			hostname := resolveHostname()

			// Run Go-based backup (collection + archive)
			stats, err := orch.RunGoBackup(ctx, envInfo.Type, hostname)
			if err != nil {
				// Check if error is due to cancellation
				if ctx.Err() == context.Canceled {
					logging.Warning("Backup was canceled")
					os.Exit(128 + int(syscall.SIGINT)) // Standard Unix exit code for SIGINT
				}

				// Check if it's a BackupError with specific exit code
				var backupErr *orchestrator.BackupError
				if errors.As(err, &backupErr) {
					logging.Error("Backup %s failed: %v", backupErr.Phase, backupErr.Err)
					os.Exit(backupErr.Code.Int())
				}

				// Generic backup error
				logging.Error("Backup orchestration failed: %v", err)
				os.Exit(types.ExitBackupError.Int())
			}

			if err := orch.SaveStatsReport(stats); err != nil {
				logging.Warning("Failed to persist backup statistics: %v", err)
			} else if stats.ReportPath != "" {
				logging.Info("Statistics report saved to %s", stats.ReportPath)
			}

			// Display backup statistics
			fmt.Println()
			logging.Info("=== Backup Statistics ===")
			logging.Info("Files collected: %d", stats.FilesCollected)
			if stats.FilesFailed > 0 {
				logging.Warning("Files failed: %d", stats.FilesFailed)
			}
			logging.Info("Directories created: %d", stats.DirsCreated)
			logging.Info("Data collected: %s", formatBytes(stats.BytesCollected))
			logging.Info("Archive size: %s", formatBytes(stats.ArchiveSize))
			if stats.BytesCollected > 0 {
				ratio := float64(stats.ArchiveSize) / float64(stats.BytesCollected) * 100
				logging.Info("Compression ratio: %.1f%%", ratio)
			}
			logging.Info("Compression used: %s (level %d)", stats.Compression, stats.CompressionLevel)
			if stats.RequestedCompression != stats.Compression {
				logging.Info("Requested compression: %s", stats.RequestedCompression)
			}
			logging.Info("Duration: %s", formatDuration(stats.Duration))
			logging.Info("Archive path: %s", stats.ArchivePath)
			if stats.ManifestPath != "" {
				logging.Info("Manifest path: %s", stats.ManifestPath)
			}
			if stats.Checksum != "" {
				logging.Info("Archive checksum (SHA256): %s", stats.Checksum)
			}
			fmt.Println()

			logging.Info("✓ Go backup orchestration completed")
		} else {
			logging.Info("Starting legacy bash backup orchestration...")
			if err := orch.RunBackup(ctx, envInfo.Type); err != nil {
				if ctx.Err() == context.Canceled {
					logging.Warning("Backup was canceled")
					os.Exit(128 + int(syscall.SIGINT))
				}
				logging.Error("Bash backup orchestration failed: %v", err)
				os.Exit(types.ExitBackupError.Int())
			}
			logging.Info("✓ Bash backup orchestration completed")
		}
	} else {
		logging.Warning("Backup is disabled in configuration")
	}
	fmt.Println()

	// Summary
	fmt.Println("===========================================")
	fmt.Println("Status: Phase 4.1 Collection")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("Phase 2 (Complete):")
	fmt.Println("  ✓ Environment detection")
	fmt.Println("  ✓ CLI argument parsing")
	fmt.Println("  ✓ Hybrid orchestrator")
	fmt.Println("  ✓ Configuration parser")
	fmt.Println("  ✓ Signal handling")
	fmt.Println()
	fmt.Println("Phase 3 (Core):")
	fmt.Println("  ✓ Pre-backup validation checks")
	fmt.Println("  ✓ Disk space verification")
	fmt.Println("  ✓ Lock file management")
	fmt.Println("  ✓ Permission checks")
	if useGoPipeline {
		fmt.Println("  ✓ Go backup pipeline (collect → archive → verify)")
		fmt.Println("  ✓ Statistics & JSON report")
	} else {
		fmt.Println("  → Go backup pipeline disabilitato (ENABLE_GO_BACKUP=false)")
		fmt.Println("  ✓ Legacy bash orchestrator attivo")
	}
	fmt.Println()
	fmt.Println("Phase 4 (Collection & Storage):")
	fmt.Println("  ✓ 4.1 - Collector Go allineati al Bash")
	fmt.Println("  → Storage operations (4.2)")
	fmt.Println()
	fmt.Println("Fasi successive:")
	fmt.Println("  → Notifications (Telegram/Email)")
	fmt.Println("  → Metrics Prometheus")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  make test          - Run all tests")
	fmt.Println("  make build         - Build binary")
	fmt.Println("  --help             - Show all options")
	fmt.Println("  --dry-run          - Test without changes")
	fmt.Println()
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

// formatDuration formats a duration in human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func resolveHostname() string {
	if path, err := exec.LookPath("hostname"); err == nil {
		if out, err := exec.Command(path, "-f").Output(); err == nil {
			if fqdn := strings.TrimSpace(string(out)); fqdn != "" {
				return fqdn
			}
		}
	}

	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return "unknown"
	}
	return host
}

func validateFutureFeatures(cfg *config.Config) error {
	if cfg.SecondaryEnabled && cfg.SecondaryPath == "" {
		return fmt.Errorf("secondary backup enabled but SECONDARY_PATH is empty")
	}
	if cfg.CloudEnabled && cfg.CloudRemote == "" {
		return fmt.Errorf("cloud backup enabled but CLOUD_REMOTE is empty")
	}
	if cfg.TelegramEnabled {
		if cfg.TelegramToken == "" || cfg.TelegramChatID == "" {
			return fmt.Errorf("telegram notifications enabled but TELEGRAM_TOKEN or TELEGRAM_CHAT_ID missing")
		}
	}
	if cfg.EmailEnabled && cfg.EmailRecipients == "" {
		return fmt.Errorf("email notifications enabled but EMAIL_RECIPIENTS is empty")
	}
	if cfg.MetricsEnabled && cfg.MetricsPath == "" {
		return fmt.Errorf("metrics enabled but METRICS_PATH is empty")
	}
	return nil
}

func detectBaseDir() (string, bool) {
	execPath, err := os.Executable()
	if err != nil {
		return "", false
	}

	resolved, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		resolved = execPath
	}

	dir := filepath.Dir(resolved)
	originalDir := dir

	for {
		if dir == "" || dir == "/" || dir == "." {
			break
		}

		if info, err := os.Stat(filepath.Join(dir, "env")); err == nil && info.IsDir() {
			return dir, true
		}
		if info, err := os.Stat(filepath.Join(dir, "script")); err == nil && info.IsDir() {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Heuristic fallback: mimic bash script behaviour (parent of executable directory)
	if parent := filepath.Dir(originalDir); parent != "" && parent != "/" && parent != "." {
		return parent, true
	}

	return originalDir, true
}

func cleanupAfterRun(logger *logging.Logger) {
	patterns := []string{
		"/tmp/backup_status_update_*.lock",
		"/tmp/backup_*_*.lock",
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logger.Debug("Cleanup glob error for %s: %v", pattern, err)
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}
			if info.Size() != 0 {
				continue
			}
			if err := os.Remove(match); err != nil {
				logger.Warning("Failed to remove orphaned lock file %s: %v", match, err)
			} else {
				logger.Debug("Removed orphaned lock file: %s", match)
			}
		}
	}
}

func addPathExclusion(excludes []string, path string) []string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return excludes
	}
	excludes = append(excludes, clean)
	excludes = append(excludes, filepath.ToSlash(filepath.Join(clean, "**")))
	return excludes
}

func isLocalPath(path string) bool {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return false
	}
	if strings.Contains(clean, ":") && !strings.HasPrefix(clean, "/") {
		// Likely an rclone remote (remote:bucket)
		return false
	}
	return filepath.IsAbs(clean)
}
