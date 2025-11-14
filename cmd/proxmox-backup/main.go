package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/backup"
	"github.com/tis24dev/proxmox-backup/internal/checks"
	"github.com/tis24dev/proxmox-backup/internal/cli"
	"github.com/tis24dev/proxmox-backup/internal/config"
	"github.com/tis24dev/proxmox-backup/internal/environment"
	"github.com/tis24dev/proxmox-backup/internal/identity"
	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/notify"
	"github.com/tis24dev/proxmox-backup/internal/orchestrator"
	"github.com/tis24dev/proxmox-backup/internal/security"
	"github.com/tis24dev/proxmox-backup/internal/storage"
	"github.com/tis24dev/proxmox-backup/internal/types"
	"github.com/tis24dev/proxmox-backup/pkg/utils"
)

const (
	version = "0.2.0" // Semantic version format required by cloud relay worker
)

func main() {
	os.Exit(run())
}

var closeStdinOnce sync.Once

func run() int {
	bootstrap := logging.NewBootstrapLogger()

	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			bootstrap.Error("PANIC: %v", r)
			fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", r, stack)
			os.Exit(types.ExitPanicError.Int())
		}
	}()

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
		closeStdinOnce.Do(func() {
			if file := os.Stdin; file != nil {
				_ = file.Close()
			}
		})
	}()

	// Parse command-line arguments
	args := cli.Parse()

	// Handle version flag
	if args.ShowVersion {
		cli.ShowVersion()
		return types.ExitSuccess.Int()
	}

	// Handle help flag
	if args.ShowHelp {
		cli.ShowHelp()
		return types.ExitSuccess.Int()
	}

	// Handle install wizard (runs before normal execution)
	if args.Install {
		if err := runInstall(ctx, args.ConfigPath, bootstrap); err != nil {
			bootstrap.Error("ERROR: %v", err)
			return types.ExitConfigError.Int()
		}
		return types.ExitSuccess.Int()
	}

	// Pre-flight: enforce Go runtime version
	if err := checkGoRuntimeVersion("1.25.4"); err != nil {
		bootstrap.Error("ERROR: %v", err)
		return types.ExitEnvironmentError.Int()
	}

	// Print header
	bootstrap.Println("===========================================")
	bootstrap.Println("  Proxmox Backup - Go Version")
	bootstrap.Printf("  Version: %s", version)
	if sig := buildSignature(); sig != "" {
		bootstrap.Printf("  Build Signature: %s", sig)
	}
	bootstrap.Println("  Phase: 5.1 - Notifications")
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
		return types.ExitConfigError.Int()
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = autoBaseDir
	}
	_ = os.Setenv("BASE_DIR", cfg.BaseDir)
	bootstrap.Println("✓ Configuration loaded successfully")

	// Show dry-run status early in bootstrap phase
	dryRun := args.DryRun || cfg.DryRun
	if dryRun {
		if args.DryRun {
			bootstrap.Println("⚠ DRY RUN MODE (enabled via --dry-run flag)")
		} else {
			bootstrap.Println("⚠ DRY RUN MODE (enabled via DRY_RUN config)")
		}
	}
	bootstrap.Println("")

	if err := validateFutureFeatures(cfg); err != nil {
		bootstrap.Error("ERROR: Invalid configuration: %v", err)
		return types.ExitConfigError.Int()
	}

	// Pre-flight: if features require network, verify basic connectivity
	if needs, reasons := featuresNeedNetwork(cfg); needs {
		if cfg.DisableNetworkPreflight {
			logging.Warning("WARNING: Network preflight disabled via DISABLE_NETWORK_PREFLIGHT; features: %s", strings.Join(reasons, ", "))
		} else {
			if err := checkInternetConnectivity(2 * time.Second); err != nil {
				bootstrap.Error("ERROR: Network connectivity required for: %s. %v", strings.Join(reasons, ", "), err)
				return types.ExitNetworkError.Int()
			}
		}
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

	// Open log file for real-time writing (will be closed after notifications)
	hostname := resolveHostname()
	startTime := time.Now()
	timestampStr := startTime.Format("20060102-150405")
	logFileName := fmt.Sprintf("backup-%s-%s.log", hostname, timestampStr)
	logFilePath := filepath.Join(cfg.LogPath, logFileName)

	// Ensure log directory exists
	if err := os.MkdirAll(cfg.LogPath, 0755); err != nil {
		logging.Warning("Failed to create log directory %s: %v", cfg.LogPath, err)
	} else {
		if err := logger.OpenLogFile(logFilePath); err != nil {
			logging.Warning("Failed to open log file %s: %v", logFilePath, err)
		} else {
			logging.Info("Log file opened: %s", logFilePath)
			// Store log path in environment for backup stats
			_ = os.Setenv("LOG_FILE", logFilePath)
		}
	}

	defer cleanupAfterRun(logger)

	// Log dry-run status in main logger (already shown in bootstrap)
	if dryRun {
		if args.DryRun {
			logging.Info("DRY RUN MODE: No actual changes will be made (enabled via --dry-run flag)")
		} else {
			logging.Info("DRY RUN MODE: No actual changes will be made (enabled via DRY_RUN config)")
		}
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
	logging.Info("Compression: %s (level %d, mode %s)", cfg.CompressionType, cfg.CompressionLevel, cfg.CompressionMode)
	logging.Info("Base directory: %s (%s)", cfg.BaseDir, baseDirSource)
	configSource := args.ConfigPathSource
	if configSource == "" {
		configSource = "configured path"
	}
	logging.Info("Configuration file: %s (%s)", args.ConfigPath, configSource)

	var identityInfo *identity.Info
	serverIDValue := strings.TrimSpace(cfg.ServerID)
	serverMACValue := ""
	telegramServerStatus := "Telegram disabled"
	if info, err := identity.Detect(cfg.BaseDir, logger); err != nil {
		logging.Warning("WARNING: Failed to load server identity: %v", err)
		identityInfo = info
	} else {
		identityInfo = info
	}

	if identityInfo != nil {
		if identityInfo.ServerID != "" {
			serverIDValue = identityInfo.ServerID
		}
		if identityInfo.PrimaryMAC != "" {
			serverMACValue = identityInfo.PrimaryMAC
		}
	}

	if serverIDValue != "" && cfg.ServerID == "" {
		cfg.ServerID = serverIDValue
	}

	logServerIdentityValues(serverIDValue, serverMACValue)
	logTelegramInfo := true
	if cfg.TelegramEnabled {
		if strings.EqualFold(cfg.TelegramBotType, "centralized") {
			logging.Debug("Contacting remote Telegram server...")
			status := notify.CheckTelegramRegistration(ctx, cfg.TelegramServerAPIHost, serverIDValue, logger)
			if status.Error != nil {
				logging.Warning("Telegram: %s", status.Message)
				logTelegramInfo = false
			} else {
				logging.Debug("Remote server contacted: Bot token / chat ID verified (handshake)")
			}
			telegramServerStatus = status.Message
		} else {
			telegramServerStatus = "Personal mode - no remote contact"
		}
	}
	if logTelegramInfo {
		logging.Info("Server Telegram: %s", telegramServerStatus)
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = ""
	}
	if _, secErr := security.Run(ctx, logger, cfg, args.ConfigPath, execPath, envInfo); secErr != nil {
		logging.Error("Security checks failed: %v", secErr)
		return types.ExitSecurityError.Int()
	}
	fmt.Println()

	if args.Restore {
		logging.Info("Restore mode enabled - starting interactive workflow...")
		if err := orchestrator.RunRestoreWorkflow(ctx, cfg, logger, version); err != nil {
			if errors.Is(err, orchestrator.ErrRestoreAborted) || errors.Is(err, orchestrator.ErrDecryptAborted) {
				logging.Info("Restore workflow aborted by user")
				return types.ExitSuccess.Int()
			}
			logging.Error("Restore workflow failed: %v", err)
			return types.ExitGenericError.Int()
		}
		logging.Info("Restore workflow completed successfully")
		return types.ExitSuccess.Int()
	}

	if args.Decrypt {
		logging.Info("Decrypt mode enabled - starting interactive workflow...")
		if err := orchestrator.RunDecryptWorkflow(ctx, cfg, logger, version); err != nil {
			if errors.Is(err, orchestrator.ErrDecryptAborted) {
				logging.Info("Decrypt workflow aborted by user")
				return types.ExitSuccess.Int()
			}
			logging.Error("Decrypt workflow failed: %v", err)
			return types.ExitGenericError.Int()
		}
		logging.Info("Decrypt workflow completed successfully")
		return types.ExitSuccess.Int()
	}

	// Initialize orchestrator
	logging.Info("Initializing backup orchestrator...")
	bashScriptPath := "/opt/proxmox-backup/script"
	orch := orchestrator.New(logger, bashScriptPath, dryRun)
	orch.SetForceNewAgeRecipient(args.ForceNewKey)
	orch.SetVersion(version)
	orch.SetConfig(cfg)
	orch.SetIdentity(serverIDValue, serverMACValue)
	orch.SetProxmoxVersion(envInfo.Version)
	orch.SetStartTime(startTime)

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
		cfg.CompressionMode,
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

	if err := orch.EnsureAgeRecipientsReady(ctx); err != nil {
		if errors.Is(err, orchestrator.ErrAgeRecipientSetupAborted) {
			logging.Warning("Encryption setup aborted by user. Exiting...")
			return types.ExitGenericError.Int()
		}
		logging.Error("ERROR: %v", err)
		return types.ExitConfigError.Int()
	}

	logging.Info("✓ Orchestrator initialized")
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
	if cfg.SecondaryEnabled {
		secondaryLogPath := strings.TrimSpace(cfg.SecondaryLogPath)
		if secondaryLogPath != "" {
			checkDir("Secondary log directory", secondaryLogPath)
		} else {
			logging.Warning("✗ Secondary log directory not configured (secondary storage enabled)")
		}
	}
	if cfg.CloudEnabled {
		cloudLogPath := strings.TrimSpace(cfg.CloudLogPath)
		if cloudLogPath == "" {
			logging.Warning("✗ Cloud log directory not configured (cloud storage enabled)")
		} else if isLocalPath(cloudLogPath) {
			checkDir("Cloud log directory", cloudLogPath)
		} else {
			logging.Info("Skipping local validation for cloud log directory (remote path): %s", cloudLogPath)
		}
	}
	checkDir("Lock directory", cfg.LockPath)

	// Initialize pre-backup checker
	logging.Debug("Configuring pre-backup validation checks...")
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
	checkerConfig.DryRun = dryRun
	if err := checkerConfig.Validate(); err != nil {
		logging.Error("Invalid checker configuration: %v", err)
		return types.ExitConfigError.Int()
	}
	checker := checks.NewChecker(logger, checkerConfig)
	orch.SetChecker(checker)

	// Ensure lock is released on exit
	defer func() {
		if err := orch.ReleaseBackupLock(); err != nil {
			logging.Warning("Failed to release backup lock: %v", err)
		}
	}()

	logging.Debug("✓ Pre-backup checks configured")
	fmt.Println()

	// Initialize storage backends
	logging.Info("Initializing storage backends...")

	// Primary (local) storage - always enabled
	localBackend, err := storage.NewLocalStorage(cfg, logger)
	if err != nil {
		logging.Error("Failed to initialize local storage: %v", err)
		return types.ExitConfigError.Int()
	}
	localFS, err := detectFilesystemInfo(ctx, localBackend, cfg.BackupPath, logger)
	if err != nil {
		logging.Error("Failed to prepare primary storage: %v", err)
		return types.ExitConfigError.Int()
	}
	logging.Info("Path Primary: %s", formatDetailedFilesystemLabel(cfg.BackupPath, localFS))

	localStats := fetchStorageStats(ctx, localBackend, logger, "Local storage")

	localAdapter := orchestrator.NewStorageAdapter(localBackend, logger, cfg.MaxLocalBackups)
	localAdapter.SetFilesystemInfo(localFS)
	localAdapter.SetInitialStats(localStats)
	orch.RegisterStorageTarget(localAdapter)
	logging.Info("%s", formatStorageInitSummary("Local storage", cfg.MaxLocalBackups, localStats))

	// Secondary storage - optional
	var secondaryFS *storage.FilesystemInfo
	if cfg.SecondaryEnabled {
		secondaryBackend, err := storage.NewSecondaryStorage(cfg, logger)
		if err != nil {
			logging.Warning("Failed to initialize secondary storage: %v", err)
			logging.Info("Path Secondary: %s", formatDetailedFilesystemLabel(cfg.SecondaryPath, nil))
		} else {
			secondaryFS, _ = detectFilesystemInfo(ctx, secondaryBackend, cfg.SecondaryPath, logger)
			logging.Info("Path Secondary: %s", formatDetailedFilesystemLabel(cfg.SecondaryPath, secondaryFS))
			secondaryStats := fetchStorageStats(ctx, secondaryBackend, logger, "Secondary storage")
			secondaryAdapter := orchestrator.NewStorageAdapter(secondaryBackend, logger, cfg.MaxSecondaryBackups)
			secondaryAdapter.SetFilesystemInfo(secondaryFS)
			secondaryAdapter.SetInitialStats(secondaryStats)
			orch.RegisterStorageTarget(secondaryAdapter)
			logging.Info("%s", formatStorageInitSummary("Secondary storage", cfg.MaxSecondaryBackups, secondaryStats))
		}
	} else {
		logging.Info("Path Secondary: disabled")
	}

	// Cloud storage - optional
	var cloudFS *storage.FilesystemInfo
	if cfg.CloudEnabled {
		cloudBackend, err := storage.NewCloudStorage(cfg, logger)
		if err != nil {
			logging.Warning("Failed to initialize cloud storage: %v", err)
			logging.Info("Path Cloud: %s", formatDetailedFilesystemLabel(cfg.CloudRemote, nil))
		} else {
			cloudFS, _ = detectFilesystemInfo(ctx, cloudBackend, cfg.CloudRemote, logger)
			logging.Info("Path Cloud: %s", formatDetailedFilesystemLabel(cfg.CloudRemote, cloudFS))
			cloudStats := fetchStorageStats(ctx, cloudBackend, logger, "Cloud storage")
			cloudAdapter := orchestrator.NewStorageAdapter(cloudBackend, logger, cfg.MaxCloudBackups)
			cloudAdapter.SetFilesystemInfo(cloudFS)
			cloudAdapter.SetInitialStats(cloudStats)
			orch.RegisterStorageTarget(cloudAdapter)
			logging.Info("%s", formatStorageInitSummary("Cloud storage", cfg.MaxCloudBackups, cloudStats))
		}
	} else {
		logging.Info("Path Cloud: disabled")
	}

	// Initialize notification channels
	logging.Info("Initializing notification channels...")

	// Telegram notifications
	if cfg.TelegramEnabled {
		telegramConfig := notify.TelegramConfig{
			Enabled:       true,
			Mode:          notify.TelegramMode(cfg.TelegramBotType),
			BotToken:      cfg.TelegramBotToken,
			ChatID:        cfg.TelegramChatID,
			ServerAPIHost: cfg.TelegramServerAPIHost,
			ServerID:      cfg.ServerID,
		}
		telegramNotifier, err := notify.NewTelegramNotifier(telegramConfig, logger)
		if err != nil {
			logging.Warning("Failed to initialize Telegram notifier: %v", err)
		} else {
			telegramAdapter := orchestrator.NewNotificationAdapter(telegramNotifier, logger)
			orch.RegisterNotificationChannel(telegramAdapter)
			logging.Info("✓ Telegram notifications initialized (mode: %s)", cfg.TelegramBotType)
		}
	} else {
		logging.Info("Telegram notifications: disabled")
	}

	// Email notifications
	if cfg.EmailEnabled {
		emailConfig := notify.EmailConfig{
			Enabled:          true,
			DeliveryMethod:   notify.EmailDeliveryMethod(cfg.EmailDeliveryMethod),
			FallbackSendmail: cfg.EmailFallbackSendmail,
			Recipient:        cfg.EmailRecipient,
			From:             cfg.EmailFrom,
			CloudRelayConfig: notify.CloudRelayConfig{
				WorkerURL:   cfg.CloudflareWorkerURL,
				WorkerToken: cfg.CloudflareWorkerToken,
				HMACSecret:  cfg.CloudflareHMACSecret,
				Timeout:     cfg.WorkerTimeout,
				MaxRetries:  cfg.WorkerMaxRetries,
				RetryDelay:  cfg.WorkerRetryDelay,
			},
		}
		emailNotifier, err := notify.NewEmailNotifier(emailConfig, envInfo.Type, logger)
		if err != nil {
			logging.Warning("Failed to initialize Email notifier: %v", err)
		} else {
			emailAdapter := orchestrator.NewNotificationAdapter(emailNotifier, logger)
			orch.RegisterNotificationChannel(emailAdapter)
			logging.Info("✓ Email notifications initialized (method: %s)", cfg.EmailDeliveryMethod)
		}
	} else {
		logging.Info("Email notifications: disabled")
	}

	// Webhook Notifications
	if cfg.WebhookEnabled {
		logging.Debug("Initializing webhook notifier...")
		webhookConfig := cfg.BuildWebhookConfig()
		logging.Debug("Webhook config built: %d endpoints configured", len(webhookConfig.Endpoints))

		webhookNotifier, err := notify.NewWebhookNotifier(webhookConfig, logger)
		if err != nil {
			logging.Warning("Failed to initialize Webhook notifier: %v", err)
		} else {
			logging.Debug("Creating webhook notification adapter...")
			webhookAdapter := orchestrator.NewNotificationAdapter(webhookNotifier, logger)

			logging.Debug("Registering webhook notification channel with orchestrator...")
			orch.RegisterNotificationChannel(webhookAdapter)
			logging.Info("✓ Webhook notifications initialized (%d endpoint(s))", len(webhookConfig.Endpoints))
		}
	} else {
		logging.Info("Webhook notifications: disabled")
	}

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
	logging.Info("  Primary: %s", formatStorageLabel(cfg.BackupPath, localFS))
	if cfg.SecondaryEnabled {
		logging.Info("  Secondary storage: %s", formatStorageLabel(cfg.SecondaryPath, secondaryFS))
	} else {
		logging.Info("  Secondary storage: disabled")
	}
	if cfg.CloudEnabled {
		logging.Info("  Cloud storage: %s", formatStorageLabel(cfg.CloudRemote, cloudFS))
	} else {
		logging.Info("  Cloud storage: disabled")
	}
	fmt.Println()

	// Log configuration info
	logging.Info("Log configuration:")
	logging.Info("  Primary: %s", cfg.LogPath)
	if cfg.SecondaryEnabled {
		if strings.TrimSpace(cfg.SecondaryLogPath) != "" {
			logging.Info("  Secondary: %s", cfg.SecondaryLogPath)
		} else {
			logging.Info("  Secondary: disabled (log path not configured)")
		}
	} else {
		logging.Info("  Secondary: disabled")
	}
	if cfg.CloudEnabled {
		if strings.TrimSpace(cfg.CloudLogPath) != "" {
			logging.Info("  Cloud: %s", cfg.CloudLogPath)
		} else {
			logging.Info("  Cloud: disabled (log path not configured)")
		}
	} else {
		logging.Info("  Cloud: disabled")
	}
	fmt.Println()

	// Notification info
	logging.Info("Notification configuration:")
	logging.Info("  Telegram: %v", cfg.TelegramEnabled)
	logging.Info("  Email: %v", cfg.EmailEnabled)
	logging.Info("  Metrics: %v", cfg.MetricsEnabled)
	fmt.Println()

	useGoPipeline := cfg.EnableGoBackup
	if useGoPipeline {
		logging.Debug("Go backup pipeline enabled")
	} else {
		logging.Info("Go backup pipeline disabled (ENABLE_GO_BACKUP=false). Using legacy bash workflow.")
	}

	// Run backup orchestration
	if cfg.BackupEnabled {
		if useGoPipeline {
			if err := orch.RunPreBackupChecks(ctx); err != nil {
				logging.Error("Pre-backup validation failed: %v", err)
				return types.ExitBackupError.Int()
			}
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
					return 128 + int(syscall.SIGINT) // Standard Unix exit code for SIGINT
				}

				// Check if it's a BackupError with specific exit code
				var backupErr *orchestrator.BackupError
				if errors.As(err, &backupErr) {
					logging.Error("Backup %s failed: %v", backupErr.Phase, backupErr.Err)
					return backupErr.Code.Int()
				}

				// Generic backup error
				logging.Error("Backup orchestration failed: %v", err)
				return types.ExitBackupError.Int()
			}

			if err := orch.SaveStatsReport(stats); err != nil {
				logging.Warning("Failed to persist backup statistics: %v", err)
			} else if stats.ReportPath != "" {
				logging.Info("✓ Statistics report saved to %s", stats.ReportPath)
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
			switch {
			case stats.CompressionSavingsPercent > 0:
				logging.Info("Compression ratio: %.1f%%", stats.CompressionSavingsPercent)
			case stats.CompressionRatioPercent > 0:
				logging.Info("Compression ratio: %.1f%%", stats.CompressionRatioPercent)
			case stats.BytesCollected > 0:
				ratio := float64(stats.ArchiveSize) / float64(stats.BytesCollected) * 100
				logging.Info("Compression ratio: %.1f%%", ratio)
			default:
				logging.Info("Compression ratio: N/A")
			}
			logging.Info("Compression used: %s (level %d, mode %s)", stats.Compression, stats.CompressionLevel, stats.CompressionMode)
			if stats.RequestedCompression != stats.Compression {
				logging.Info("Requested compression: %s", stats.RequestedCompression)
			}
			logging.Info("Duration: %s", formatDuration(stats.Duration))
			if stats.BundleCreated {
				logging.Info("Bundle path: %s", stats.ArchivePath)
				logging.Info("Bundle contents: archive + checksum + metadata")
			} else {
				logging.Info("Archive path: %s", stats.ArchivePath)
				if stats.ManifestPath != "" {
					logging.Info("Manifest path: %s", stats.ManifestPath)
				}
				if stats.Checksum != "" {
					logging.Info("Archive checksum (SHA256): %s", stats.Checksum)
				}
			}
			fmt.Println()

			logging.Info("✓ Go backup orchestration completed")
			logServerIdentityValues(serverIDValue, serverMACValue)
		} else {
			logging.Info("Starting legacy bash backup orchestration...")
			if err := orch.RunBackup(ctx, envInfo.Type); err != nil {
				if ctx.Err() == context.Canceled {
					logging.Warning("Backup was canceled")
					return 128 + int(syscall.SIGINT)
				}
				logging.Error("Bash backup orchestration failed: %v", err)
				return types.ExitBackupError.Int()
			}
			logging.Info("✓ Bash backup orchestration completed")
			logServerIdentityValues(serverIDValue, serverMACValue)
		}
	} else {
		logging.Warning("Backup is disabled in configuration")
	}
	fmt.Println()

	// Summary
	fmt.Println("===========================================")
	fmt.Println("Status: Phase 5.1 Notifications")
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
	fmt.Println("  ✓ 4.1 - Collection (PVE/PBS/System)")
	fmt.Println("  ✓ 4.2 - Storage (Local/Secondary/Cloud)")
	fmt.Println()
	fmt.Println("Phase 5 (Notifications & Metrics):")
	fmt.Println("  ✓ 5.1 - Notifications (Telegram/Email)")
	fmt.Println("  → 5.2 - Metrics (Prometheus)")
	fmt.Println()
	fmt.Println("Fasi successive:")
	fmt.Println("  → Metrics Prometheus")
	fmt.Println("  → Performance benchmarks")
	fmt.Println("  → Complete test coverage")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  make test          - Run all tests")
	fmt.Println("  make build         - Build binary")
	fmt.Println("  --help             - Show all options")
	fmt.Println("  --dry-run          - Test without changes")
	fmt.Println()

	return types.ExitSuccess.Int()
}

// checkGoRuntimeVersion ensures the running binary was built with at least the specified Go version (semver: major.minor.patch).
func checkGoRuntimeVersion(min string) error {
	rt := runtime.Version() // e.g., "go1.25.4"
	// Normalize versions to x.y.z
	parse := func(v string) (int, int, int) {
		// Accept forms: go1.25.4, go1.25, 1.25.4, 1.25
		v = strings.TrimPrefix(v, "go")
		parts := strings.Split(v, ".")
		toInt := func(s string) int { n, _ := strconv.Atoi(s); return n }
		major, minor, patch := 0, 0, 0
		if len(parts) > 0 {
			major = toInt(parts[0])
		}
		if len(parts) > 1 {
			minor = toInt(parts[1])
		}
		if len(parts) > 2 {
			patch = toInt(parts[2])
		}
		return major, minor, patch
	}

	rtMaj, rtMin, rtPatch := parse(rt)
	minMaj, minMin, minPatch := parse(min)

	newer := func(aMaj, aMin, aPatch, bMaj, bMin, bPatch int) bool {
		if aMaj != bMaj {
			return aMaj > bMaj
		}
		if aMin != bMin {
			return aMin > bMin
		}
		return aPatch >= bPatch
	}

	if !newer(rtMaj, rtMin, rtPatch, minMaj, minMin, minPatch) {
		return fmt.Errorf("Go runtime version %s is below required %s — rebuild with Go %s or set GOTOOLCHAIN=auto", rt, "go"+min, "go"+min)
	}
	return nil
}

// featuresNeedNetwork returns whether current configuration requires outbound network, and human reasons.
func featuresNeedNetwork(cfg *config.Config) (bool, []string) {
	reasons := []string{}
	// Telegram (any mode uses network)
	if cfg.TelegramEnabled {
		if strings.EqualFold(cfg.TelegramBotType, "centralized") {
			reasons = append(reasons, "Telegram centralized registration")
		} else {
			reasons = append(reasons, "Telegram personal notifications")
		}
	}
	// Email via relay
	if cfg.EmailEnabled && strings.EqualFold(cfg.EmailDeliveryMethod, "relay") {
		reasons = append(reasons, "Email relay delivery")
	}
	// Webhooks
	if cfg.WebhookEnabled {
		reasons = append(reasons, "Webhooks")
	}
	// Cloud uploads via rclone
	if cfg.CloudEnabled {
		reasons = append(reasons, "Cloud storage (rclone)")
	}
	return len(reasons) > 0, reasons
}

// checkInternetConnectivity attempts a couple of quick TCP dials to common endpoints.
// It succeeds if at least one attempt connects.
func checkInternetConnectivity(timeout time.Duration) error {
	type target struct{ network, addr string }
	targets := []target{
		{"tcp", "1.1.1.1:443"},
		{"tcp", "8.8.8.8:53"},
	}
	deadline := time.Now().Add(timeout)
	for _, t := range targets {
		d := net.Dialer{Timeout: time.Until(deadline)}
		if conn, err := d.Dial(t.network, t.addr); err == nil {
			_ = conn.Close()
			return nil
		}
	}
	return fmt.Errorf("no outbound connectivity (checked %d endpoints)", len(targets))
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

func logServerIdentityValues(serverID, mac string) {
	serverID = strings.TrimSpace(serverID)
	mac = strings.TrimSpace(mac)
	if serverID != "" {
		logging.Info("Server ID: %s", serverID)
	}
	if mac != "" {
		logging.Info("Server MAC Address: %s", mac)
	}
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
	// Telegram validation - only for personal mode
	if cfg.TelegramEnabled && cfg.TelegramBotType == "personal" {
		if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
			return fmt.Errorf("telegram personal mode enabled but TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID missing")
		}
	}
	// Email recipient validation - auto-detection is allowed
	// No validation needed here as email notifier will handle it
	if cfg.MetricsEnabled && cfg.MetricsPath == "" {
		return fmt.Errorf("metrics enabled but METRICS_PATH is empty")
	}
	return nil
}

type configStatusLogger interface {
	Warning(format string, args ...interface{})
	Info(format string, args ...interface{})
}

func ensureConfigExists(path string, logger configStatusLogger) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("configuration path is empty")
	}

	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat configuration file: %w", err)
	}

	logger.Warning("Configuration file not found: %s", path)
	fmt.Print("Generate default configuration from template? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	answer := strings.ToLower(strings.TrimSpace(response))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("configuration file is required to continue")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(config.DefaultEnvTemplate()), 0o600); err != nil {
		return fmt.Errorf("failed to write default configuration: %w", err)
	}

	logger.Info("✓ Default configuration created at %s", path)
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

func detectFilesystemInfo(ctx context.Context, backend storage.Storage, path string, logger *logging.Logger) (*storage.FilesystemInfo, error) {
	if backend == nil || !backend.IsEnabled() {
		return nil, nil
	}

	fsInfo, err := backend.DetectFilesystem(ctx)
	if err != nil {
		if backend.IsCritical() {
			return nil, err
		}
		logger.Warning("WARNING: %s filesystem detection failed: %v", backend.Name(), err)
		return nil, nil
	}

	if !fsInfo.SupportsOwnership {
		logger.Warning("%s [%s] does not support ownership changes; chown/chmod will be skipped", path, fsInfo.Type)
	}

	return fsInfo, nil
}

func formatStorageLabel(path string, info *storage.FilesystemInfo) string {
	fsType := "unknown"
	if info != nil && info.Type != "" {
		fsType = string(info.Type)
	}
	return fmt.Sprintf("%s [%s]", path, fsType)
}

func formatDetailedFilesystemLabel(path string, info *storage.FilesystemInfo) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return "disabled"
	}
	if info == nil {
		return fmt.Sprintf("%s -> Filesystem: unknown (detection unavailable)", cleanPath)
	}

	ownership := "no ownership"
	if info.SupportsOwnership {
		ownership = "supports ownership"
	}

	network := ""
	if info.IsNetworkFS {
		network = " [network]"
	}

	mount := info.MountPoint
	if mount == "" {
		mount = "unknown"
	}

	return fmt.Sprintf("%s -> Filesystem: %s (%s)%s [mount: %s]",
		cleanPath,
		info.Type,
		ownership,
		network,
		mount,
	)
}

func fetchStorageStats(ctx context.Context, backend storage.Storage, logger *logging.Logger, label string) *storage.StorageStats {
	if ctx.Err() != nil || backend == nil || !backend.IsEnabled() {
		return nil
	}
	stats, err := backend.GetStats(ctx)
	if err != nil {
		logger.Debug("%s: unable to gather stats: %v", label, err)
		return nil
	}
	return stats
}

func formatStorageInitSummary(name string, retention int, stats *storage.StorageStats) string {
	retentionStr := fmt.Sprintf("retention %s", formatBackupNoun(retention))
	if stats == nil {
		return fmt.Sprintf("✓ %s initialized (%s)", name, retentionStr)
	}
	return fmt.Sprintf("✓ %s initialized (present %s, %s)",
		name,
		formatBackupNoun(stats.TotalBackups),
		retentionStr,
	)
}

func formatBackupNoun(n int) string {
	if n == 1 {
		return "1 backup"
	}
	return fmt.Sprintf("%d backups", n)
}

func runInstall(ctx context.Context, configPath string, bootstrap *logging.BootstrapLogger) error {
	if strings.TrimSpace(configPath) == "" {
		configPath = "./configs/backup.env"
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("===========================================")
	fmt.Println(" Proxmox Backup Go - Install Wizard")
	fmt.Println("===========================================")
	fmt.Printf("Configuration file: %s\n\n", configPath)

	if _, err := os.Stat(configPath); err == nil {
		overwrite, err := promptYesNo(reader, fmt.Sprintf("%s already exists. Overwrite? [y/N]: ", configPath), false)
		if err != nil {
			return err
		}
		if !overwrite {
			return fmt.Errorf("installation aborted (existing configuration kept)")
		}
	}

	create, err := promptYesNo(reader, "Generate configuration file from default template? [y/N]: ", false)
	if err != nil {
		return err
	}
	if !create {
		return fmt.Errorf("installation aborted by user")
	}

	template := config.DefaultEnvTemplate()

	fmt.Println("\n--- Secondary storage ---")
	fmt.Println("Configure an additional local path for redundant copies. (You can change it later)")
	enableSecondary, err := promptYesNo(reader, "Enable secondary backup path? [y/N]: ", false)
	if err != nil {
		return err
	}
	if enableSecondary {
		secondaryPath, err := promptNonEmpty(reader, "Secondary backup path (SECONDARY_PATH): ")
		if err != nil {
			return err
		}
		secondaryLog, err := promptNonEmpty(reader, "Secondary log path (SECONDARY_LOG_PATH): ")
		if err != nil {
			return err
		}
		template = setEnvValue(template, "SECONDARY_ENABLED", "true")
		template = setEnvValue(template, "SECONDARY_PATH", secondaryPath)
		template = setEnvValue(template, "SECONDARY_LOG_PATH", secondaryLog)
	} else {
		template = setEnvValue(template, "SECONDARY_ENABLED", "false")
		template = setEnvValue(template, "SECONDARY_PATH", "")
		template = setEnvValue(template, "SECONDARY_LOG_PATH", "")
	}

	fmt.Println("\n--- Cloud storage (rclone) ---")
	fmt.Println("Remember to configure rclone manually before enabling cloud backups.")
	enableCloud, err := promptYesNo(reader, "Enable cloud backups? [y/N]: ", false)
	if err != nil {
		return err
	}
	if enableCloud {
		remote, err := promptNonEmpty(reader, "Rclone remote for backups (e.g. myremote:pbs-backups): ")
		if err != nil {
			return err
		}
		logRemote, err := promptNonEmpty(reader, "Rclone remote for logs (e.g. myremote:/logs): ")
		if err != nil {
			return err
		}
		template = setEnvValue(template, "CLOUD_ENABLED", "true")
		template = setEnvValue(template, "CLOUD_REMOTE", remote)
		template = setEnvValue(template, "CLOUD_LOG_PATH", logRemote)
	} else {
		template = setEnvValue(template, "CLOUD_ENABLED", "false")
		template = setEnvValue(template, "CLOUD_REMOTE", "")
		template = setEnvValue(template, "CLOUD_LOG_PATH", "")
	}

	fmt.Println("\n--- Telegram ---")
	enableTelegram, err := promptYesNo(reader, "Enable Telegram notifications (centralized)? [y/N]: ", false)
	if err != nil {
		return err
	}
	if enableTelegram {
		template = setEnvValue(template, "TELEGRAM_ENABLED", "true")
		template = setEnvValue(template, "BOT_TELEGRAM_TYPE", "centralized")
	} else {
		template = setEnvValue(template, "TELEGRAM_ENABLED", "false")
	}

	fmt.Println("\n--- Email ---")
	enableEmail, err := promptYesNo(reader, "Enable email notifications (central relay)? [y/N]: ", false)
	if err != nil {
		return err
	}
	if enableEmail {
		template = setEnvValue(template, "EMAIL_ENABLED", "true")
		template = setEnvValue(template, "EMAIL_DELIVERY_METHOD", "relay")
		template = setEnvValue(template, "EMAIL_FALLBACK_SENDMAIL", "true")
	} else {
		template = setEnvValue(template, "EMAIL_ENABLED", "false")
	}

	fmt.Println("\n--- Encryption ---")
	enableEncryption, err := promptYesNo(reader, "Enable backup encryption? [y/N]: ", false)
	if err != nil {
		return err
	}
	if enableEncryption {
		template = setEnvValue(template, "ENCRYPT_ARCHIVE", "true")
	} else {
		template = setEnvValue(template, "ENCRYPT_ARCHIVE", "false")
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(template), 0o600); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	bootstrap.Info("✓ Configuration saved at %s", configPath)

	if enableEncryption {
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to reload configuration after install: %w", err)
		}
		logger := logging.New(types.LogLevelError, false)
		logger.SetOutput(io.Discard)
		orch := orchestrator.New(logger, "/opt/proxmox-backup/script", false)
		orch.SetConfig(cfg)
		orch.SetForceNewAgeRecipient(true)
		if err := orch.EnsureAgeRecipientsReady(ctx); err != nil {
			if errors.Is(err, orchestrator.ErrAgeRecipientSetupAborted) {
				return fmt.Errorf("encryption setup aborted by user")
			}
			return fmt.Errorf("encryption setup failed: %w", err)
		}
	}

	fmt.Println("\nInstallation completed.")
	fmt.Println("You can adjust any other advanced option directly in the generated env file.")
	return nil
}

func promptYesNo(reader *bufio.Reader, question string, defaultYes bool) (bool, error) {
	for {
		fmt.Print(question)
		resp, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp == "" {
			return defaultYes, nil
		}
		switch resp {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("Please answer with 'y' or 'n'.")
		}
	}
}

func promptNonEmpty(reader *bufio.Reader, question string) (string, error) {
	for {
		fmt.Print(question)
		resp, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		resp = strings.TrimSpace(resp)
		if resp != "" {
			return resp, nil
		}
		fmt.Println("Value cannot be empty.")
	}
}

func setEnvValue(template, key, value string) string {
	target := key + "="
	lines := strings.Split(template, "\n")
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, target) {
			lines[i] = key + "=" + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, key+"="+value)
	}
	return strings.Join(lines, "\n")
}

func buildSignature() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		var revision, vcsTime string
		modified := ""
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				vcsTime = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					modified = "*"
				}
			}
		}
		if revision != "" || vcsTime != "" {
			shortRev := revision
			if len(shortRev) > 9 {
				shortRev = shortRev[:9]
			}
			if shortRev != "" && vcsTime != "" {
				return fmt.Sprintf("%s%s (%s)", shortRev, modified, vcsTime)
			}
			if shortRev != "" {
				return shortRev + modified
			}
			if vcsTime != "" {
				return vcsTime
			}
		}
	}
	return ""
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
