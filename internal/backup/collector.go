package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/logging"
	"github.com/tis24dev/proxmox-backup/internal/types"
)

// CollectionStats tracks statistics during backup collection
type CollectionStats struct {
	FilesProcessed int
	FilesFailed    int
	DirsCreated    int
	BytesCollected int64
}

// FileSummary represents metadata about a sampled file
type FileSummary struct {
	RelativePath string    `json:"relative_path"`
	SizeBytes    int64     `json:"size_bytes"`
	SizeHuman    string    `json:"size_human"`
	ModTime      time.Time `json:"mod_time"`
}

// Collector handles backup data collection
type Collector struct {
	logger   *logging.Logger
	config   *CollectorConfig
	stats    *CollectionStats
	tempDir  string
	proxType types.ProxmoxType
	dryRun   bool
}

// CollectorConfig holds configuration for backup collection
type CollectorConfig struct {
	// PVE-specific collection options
	BackupVMConfigs      bool
	BackupClusterConfig  bool
	BackupPVEFirewall    bool
	BackupVZDumpConfig   bool
	BackupPVEACL         bool
	BackupPVEJobs        bool
	BackupPVESchedules   bool
	BackupPVEReplication bool
	BackupPVEBackupFiles bool
	BackupCephConfig     bool

	// PBS-specific collection options
	BackupDatastoreConfigs bool
	BackupUserConfigs      bool
	BackupRemoteConfigs    bool
	BackupSyncJobs         bool
	BackupVerificationJobs bool
	BackupTapeConfigs      bool
	BackupPruneSchedules   bool
	BackupPxarFiles        bool

	// System collection options
	BackupNetworkConfigs    bool
	BackupAptSources        bool
	BackupCronJobs          bool
	BackupSystemdServices   bool
	BackupSSLCerts          bool
	BackupSysctlConfig      bool
	BackupKernelModules     bool
	BackupFirewallRules     bool
	BackupInstalledPackages bool
	BackupScriptDir         bool
	BackupCriticalFiles     bool
	BackupSSHKeys           bool
	BackupZFSConfig         bool
	BackupRootHome          bool
	BackupScriptRepository  bool
	BackupUserHomes         bool

	// PXAR scanning concurrency
	PxarDatastoreConcurrency int
	PxarIntraConcurrency     int

	// Exclude patterns (glob patterns to skip)
	ExcludePatterns []string

	CustomBackupPaths []string
	BackupBlacklist   []string

	// Script repository base path (usually BASE_DIR)
	ScriptRepositoryPath string

	// PBS Authentication (auto-detected)
	PBSRepository  string
	PBSPassword    string
	PBSFingerprint string
}

var defaultExcludePatterns = []string{
	"**/node_modules/**",
	"**/.vscode/**",
	"**/.cursor*",
	"**/.cursor-server*",
}

// Validate checks if the collector configuration is valid
func (c *CollectorConfig) Validate() error {
	// Validate exclude patterns (basic glob syntax check)
	for i, pattern := range c.ExcludePatterns {
		if pattern == "" {
			return fmt.Errorf("exclude pattern at index %d is empty", i)
		}
		// Test if pattern is valid glob syntax
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("invalid glob pattern at index %d: %s (error: %w)", i, pattern, err)
		}
	}

	// At least one collection option should be enabled
	hasAnyEnabled := c.BackupVMConfigs || c.BackupClusterConfig ||
		c.BackupPVEFirewall || c.BackupVZDumpConfig || c.BackupPVEACL ||
		c.BackupPVEJobs || c.BackupPVESchedules || c.BackupPVEReplication ||
		c.BackupPVEBackupFiles || c.BackupCephConfig ||
		c.BackupDatastoreConfigs || c.BackupUserConfigs || c.BackupRemoteConfigs ||
		c.BackupSyncJobs || c.BackupVerificationJobs || c.BackupTapeConfigs ||
		c.BackupPruneSchedules || c.BackupPxarFiles ||
		c.BackupNetworkConfigs || c.BackupAptSources || c.BackupCronJobs ||
		c.BackupSystemdServices || c.BackupSSLCerts || c.BackupSysctlConfig ||
		c.BackupKernelModules || c.BackupFirewallRules ||
		c.BackupInstalledPackages || c.BackupScriptDir || c.BackupCriticalFiles ||
		c.BackupSSHKeys || c.BackupZFSConfig

	if !hasAnyEnabled {
		return fmt.Errorf("at least one backup option must be enabled")
	}

	if c.PxarDatastoreConcurrency <= 0 {
		c.PxarDatastoreConcurrency = 3
	}
	if c.PxarIntraConcurrency <= 0 {
		c.PxarIntraConcurrency = 4
	}

	return nil
}

// NewCollector creates a new backup collector
func NewCollector(logger *logging.Logger, config *CollectorConfig, tempDir string, proxType types.ProxmoxType, dryRun bool) *Collector {
	return &Collector{
		logger:   logger,
		config:   config,
		stats:    &CollectionStats{},
		tempDir:  tempDir,
		proxType: proxType,
		dryRun:   dryRun,
	}
}

// GetDefaultCollectorConfig returns default collection configuration
func GetDefaultCollectorConfig() *CollectorConfig {
	return &CollectorConfig{
		// PVE-specific (all enabled by default)
		BackupVMConfigs:      true,
		BackupClusterConfig:  true,
		BackupPVEFirewall:    true,
		BackupVZDumpConfig:   true,
		BackupPVEACL:         true,
		BackupPVEJobs:        true,
		BackupPVESchedules:   true,
		BackupPVEReplication: true,
		BackupPVEBackupFiles: true,
		BackupCephConfig:     true,

		// PBS-specific (all enabled by default)
		BackupDatastoreConfigs: true,
		BackupUserConfigs:      true,
		BackupRemoteConfigs:    true,
		BackupSyncJobs:         true,
		BackupVerificationJobs: true,
		BackupTapeConfigs:      true,
		BackupPruneSchedules:   true,
		BackupPxarFiles:        true,

		// System collection (all enabled by default)
		BackupNetworkConfigs:    true,
		BackupAptSources:        true,
		BackupCronJobs:          true,
		BackupSystemdServices:   true,
		BackupSSLCerts:          true,
		BackupSysctlConfig:      true,
		BackupKernelModules:     true,
		BackupFirewallRules:     true,
		BackupInstalledPackages: true,
		BackupScriptDir:         true,
		BackupCriticalFiles:     true,
		BackupSSHKeys:           true,
		BackupZFSConfig:         true,
		BackupRootHome:          true,
		BackupScriptRepository:  true,
		BackupUserHomes:         true,

		PxarDatastoreConcurrency: 3,
		PxarIntraConcurrency:     4,

		ExcludePatterns:   append([]string(nil), defaultExcludePatterns...),
		CustomBackupPaths: []string{},
		BackupBlacklist:   []string{},
	}
}

// CollectAll performs full backup collection based on Proxmox type
func (c *Collector) CollectAll(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Info("Starting backup collection for %s", c.proxType)
	c.logger.Debug("Collector dry-run=%v tempDir=%s", c.dryRun, c.tempDir)

	switch c.proxType {
	case types.ProxmoxVE:
		c.logger.Debug("Invoking PVE-specific collectors (configs, jobs, schedules, storage metadata)")
		if err := c.CollectPVEConfigs(ctx); err != nil {
			return fmt.Errorf("PVE collection failed: %w", err)
		}
		c.logger.Debug("PVE-specific collection completed")
	case types.ProxmoxBS:
		c.logger.Debug("Invoking PBS-specific collectors (datastores, users, namespaces, pxar metadata)")
		if err := c.CollectPBSConfigs(ctx); err != nil {
			return fmt.Errorf("PBS collection failed: %w", err)
		}
		c.logger.Debug("PBS-specific collection completed")
	case types.ProxmoxUnknown:
		c.logger.Warning("Unknown Proxmox type, collecting generic system info only")
		c.logger.Debug("Skipping hypervisor-specific collection because type is unknown")
	}

	// Collect common system information (always collect)
	if err := ctx.Err(); err != nil {
		return err
	}
	c.logger.Debug("Collecting baseline system information (network/system files, commands, hardware data)")
	if err := c.CollectSystemInfo(ctx); err != nil {
		c.logger.Warning("System info collection had warnings: %v", err)
	}
	c.logger.Debug("Baseline system information collected successfully")

	c.logger.Debug("Collection completed: %d files, %d failed, %d dirs created",
		c.stats.FilesProcessed, c.stats.FilesFailed, c.stats.DirsCreated)

	return nil
}

// NOTE: CollectPVEConfigs, CollectPBSConfigs, and CollectSystemInfo are now in separate files:
// - collector_pve.go
// - collector_pbs.go
// - collector_system.go

// Helper functions

func (c *Collector) shouldExclude(path string) bool {
	if len(c.config.ExcludePatterns) == 0 {
		return false
	}

	candidates := uniqueCandidates(path, c.tempDir)

	for _, pattern := range c.config.ExcludePatterns {
		for _, candidate := range candidates {
			if matchesGlob(pattern, candidate) {
				c.logger.Debug("Excluding %s (matches pattern %s)", path, pattern)
				return true
			}
		}
	}
	return false
}

func uniqueCandidates(path, tempDir string) []string {
	base := filepath.Base(path)
	candidates := []string{path}
	if base != "" && base != "." && base != string(filepath.Separator) {
		candidates = append(candidates, base)
	}

	if rel, err := filepath.Rel("/", path); err == nil {
		if rel != "." && rel != "" {
			candidates = append(candidates, rel)
		}
	}

	if tempDir != "" {
		if relTemp, err := filepath.Rel(tempDir, path); err == nil {
			if relTemp != "." && relTemp != "" && relTemp != ".." {
				candidates = append(candidates, relTemp)
			}
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		if _, ok := seen[cand]; ok {
			continue
		}
		seen[cand] = struct{}{}
		unique = append(unique, cand)
	}
	return unique
}

func matchesGlob(pattern, candidate string) bool {
	normalizedPattern := filepath.ToSlash(pattern)
	normalizedCandidate := filepath.ToSlash(candidate)

	if matched, err := filepath.Match(normalizedPattern, normalizedCandidate); err == nil && matched {
		return true
	}

	if strings.Contains(normalizedPattern, "**") {
		regexPattern := globToRegex(normalizedPattern)
		matched, err := regexp.MatchString(regexPattern, normalizedCandidate)
		if err == nil && matched {
			return true
		}
	}

	return false
}

func globToRegex(pattern string) string {
	var builder strings.Builder
	builder.WriteString("^")

	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				builder.WriteString(".*")
				i++
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString("[^/]")
		case '[':
			builder.WriteByte('[')
			j := i + 1
			if j < len(runes) && (runes[j] == '!' || runes[j] == '^') {
				builder.WriteByte('^')
				j++
			}
			for ; j < len(runes) && runes[j] != ']'; j++ {
				switch runes[j] {
				case '\\':
					builder.WriteString("\\\\")
				default:
					builder.WriteRune(runes[j])
				}
			}
			if j >= len(runes) {
				builder.WriteString("\\[")
			} else {
				builder.WriteByte(']')
				i = j
			}
		case '\\':
			builder.WriteString("\\\\")
		default:
			builder.WriteString(regexp.QuoteMeta(string(runes[i])))
		}
	}

	builder.WriteString("$")
	return builder.String()
}

func (c *Collector) ensureDir(path string) error {
	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would create directory: %s", path)
		return nil
	}

	created := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		created = true
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	if created {
		c.stats.DirsCreated++
	}
	return nil
}

func (c *Collector) safeCopyFile(ctx context.Context, src, dest, description string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Debug("Collecting %s: %s -> %s", description, src, dest)

	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			c.logger.Debug("%s not found: %s (skipping)", description, src)
			return nil
		}
		c.stats.FilesFailed++
		return fmt.Errorf("failed to stat %s: %w", src, err)
	}

	// Check if this file should be excluded
	if c.shouldExclude(src) {
		return nil
	}

	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would copy file: %s -> %s", src, dest)
		c.stats.FilesProcessed++
		return nil
	}

	// Handle symbolic links by recreating the link
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			c.stats.FilesFailed++
			return fmt.Errorf("failed to read symlink %s: %w", src, err)
		}

		if err := c.ensureDir(filepath.Dir(dest)); err != nil {
			c.stats.FilesFailed++
			return err
		}

		// Remove existing file if present
		if _, err := os.Lstat(dest); err == nil {
			if err := os.Remove(dest); err != nil {
				c.stats.FilesFailed++
				return fmt.Errorf("failed to replace existing file %s: %w", dest, err)
			}
		}

		if err := os.Symlink(target, dest); err != nil {
			c.stats.FilesFailed++
			return fmt.Errorf("failed to create symlink %s -> %s: %w", dest, target, err)
		}

		c.stats.FilesProcessed++
		c.logger.Debug("Successfully copied symlink %s -> %s", dest, target)
		return nil
	}

	if !info.Mode().IsRegular() {
		// Skip non-regular files (devices, sockets, etc.) but count as processed
		c.logger.Debug("Skipping non-regular file: %s", src)
		return nil
	}

	// Ensure destination directory exists
	if err := c.ensureDir(filepath.Dir(dest)); err != nil {
		c.stats.FilesFailed++
		return err
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to open %s: %w", src, err)
	}
	defer srcFile.Close()

	// Create destination file with restrictive permissions
	destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to create %s: %w", dest, err)
	}
	defer destFile.Close()

	// Copy content
	written, err := io.Copy(destFile, srcFile)
	if err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to copy %s: %w", src, err)
	}

	c.stats.FilesProcessed++
	c.stats.BytesCollected += written
	c.logger.Debug("Successfully collected %s: %s", description, src)

	return nil
}

func (c *Collector) safeCopyDir(ctx context.Context, src, dest, description string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Debug("Collecting directory %s: %s -> %s", description, src, dest)

	if c.shouldExclude(src) {
		c.logger.Debug("Skipping directory %s due to exclusion pattern", src)
		return nil
	}

	if _, err := os.Stat(src); os.IsNotExist(err) {
		c.logger.Debug("%s not found: %s (skipping)", description, src)
		return nil
	}

	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would copy directory: %s -> %s", src, dest)
		return nil
	}

	// Ensure destination exists
	if err := c.ensureDir(dest); err != nil {
		return err
	}

	// Walk source directory
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if errCtx := ctx.Err(); errCtx != nil {
			return errCtx
		}

		if err != nil {
			return err
		}

		// Check if this path should be excluded
		if c.shouldExclude(path) {
			// If it's a directory, skip it entirely
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, relPath)

		if info.IsDir() {
			return c.ensureDir(destPath)
		}

		return c.safeCopyFile(ctx, path, destPath, filepath.Base(path))
	})

	if err != nil {
		c.logger.Warning("Failed to copy directory %s: %v", description, err)
		return err
	}

	c.logger.Debug("Successfully collected %s: %s", description, src)
	return nil
}

func (c *Collector) safeCmdOutput(ctx context.Context, cmd, output, description string, critical bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.logger.Debug("Collecting %s via command: %s > %s", description, cmd, output)

	cmdParts := strings.Fields(cmd)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty command")
	}

	// Check if command exists
	if _, err := exec.LookPath(cmdParts[0]); err != nil {
		if critical {
			c.stats.FilesFailed++
			return fmt.Errorf("critical command not available: %s", cmdParts[0])
		}
		c.logger.Debug("Command not available: %s (skipping %s)", cmdParts[0], description)
		return nil
	}

	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would execute command: %s > %s", cmd, output)
		return nil
	}

	// Ensure output directory exists
	if err := c.ensureDir(filepath.Dir(output)); err != nil {
		return err
	}

	// Execute command
	execCmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	outFile, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		if outFile != nil {
			_ = outFile.Close()
		}
	}()

	var outputBuf bytes.Buffer
	multiWriter := io.MultiWriter(outFile, &outputBuf)
	execCmd.Stdout = multiWriter
	execCmd.Stderr = multiWriter

	if err := execCmd.Run(); err != nil {
		if closeErr := outFile.Close(); closeErr != nil {
			c.logger.Debug("Failed to close output file for %s: %v", description, closeErr)
		}
		outFile = nil
		if removeErr := os.Remove(output); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			c.logger.Debug("Failed to remove incomplete output %s: %v", output, removeErr)
		}
		if critical {
			c.stats.FilesFailed++
			return fmt.Errorf("critical command failed for %s: %w", description, err)
		}
		cmdString := cmd
		if len(cmdParts) > 0 {
			cmdString = strings.Join(cmdParts, " ")
		}
		c.logger.Warning("Skipping %s: command `%s` failed (%v). Non-critical; backup continues. Ensure the PBS CLI is available and has proper permissions. Output: %s",
			description,
			cmdString,
			err,
			summarizeCommandOutput(&outputBuf),
		)
		return nil // Non-critical failure
	}

	c.stats.FilesProcessed++
	c.logger.Debug("Successfully collected %s via command: %s", description, cmd)

	return nil
}

// safeCmdOutputWithPBSAuth executes a command with PBS authentication environment variables
// This enables automatic authentication for proxmox-backup-client commands
func (c *Collector) safeCmdOutputWithPBSAuth(ctx context.Context, cmd, output, description string, critical bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmdParts := strings.Fields(cmd)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty command")
	}

	// Check if command exists
	if _, err := exec.LookPath(cmdParts[0]); err != nil {
		if critical {
			c.stats.FilesFailed++
			return fmt.Errorf("critical command not available: %s", cmdParts[0])
		}
		c.logger.Debug("Command not available: %s (skipping %s)", cmdParts[0], description)
		return nil
	}

	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would execute command with PBS auth: %s > %s", cmd, output)
		return nil
	}

	// Ensure output directory exists
	if err := c.ensureDir(filepath.Dir(output)); err != nil {
		return err
	}

	// Execute command with PBS authentication environment
	execCmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)

	// Set PBS authentication environment variables (if available)
	execCmd.Env = os.Environ()
	pbsAuthUsed := false
	if c.config.PBSRepository != "" {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("PBS_REPOSITORY=%s", c.config.PBSRepository))
		pbsAuthUsed = true
	}
	if c.config.PBSPassword != "" {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("PBS_PASSWORD=%s", c.config.PBSPassword))
		pbsAuthUsed = true
	}
	if c.config.PBSFingerprint != "" {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("PBS_FINGERPRINT=%s", c.config.PBSFingerprint))
		pbsAuthUsed = true
	}

	if pbsAuthUsed {
		c.logger.Debug("Using PBS authentication for command: %s", cmdParts[0])
	}

	outFile, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		if outFile != nil {
			_ = outFile.Close()
		}
	}()

	var outputBuf bytes.Buffer
	multiWriter := io.MultiWriter(outFile, &outputBuf)
	execCmd.Stdout = multiWriter
	execCmd.Stderr = multiWriter

	if err := execCmd.Run(); err != nil {
		if closeErr := outFile.Close(); closeErr != nil {
			c.logger.Debug("Failed to close output file for %s: %v", description, closeErr)
		}
		outFile = nil
		if removeErr := os.Remove(output); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			c.logger.Debug("Failed to remove incomplete output %s: %v", output, removeErr)
		}
		if critical {
			c.stats.FilesFailed++
			return fmt.Errorf("critical command failed for %s: %w", description, err)
		}
		cmdString := cmd
		if len(cmdParts) > 0 {
			cmdString = strings.Join(cmdParts, " ")
		}
		c.logger.Warning("Skipping %s: command `%s` failed (%v). Non-critical; backup continues. Ensure the PBS CLI is available and has proper permissions. Output: %s",
			description,
			cmdString,
			err,
			summarizeCommandOutput(&outputBuf),
		)
		return nil // Non-critical failure
	}

	c.stats.FilesProcessed++
	c.logger.Debug("Successfully collected %s via PBS-authenticated command: %s", description, cmd)

	return nil
}

// safeCmdOutputWithPBSAuthForDatastore executes a command with PBS authentication for a specific datastore
// This function appends the datastore name to the PBS_REPOSITORY environment variable
func (c *Collector) safeCmdOutputWithPBSAuthForDatastore(ctx context.Context, cmd, output, description, datastoreName string, critical bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmdParts := strings.Fields(cmd)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty command")
	}

	// Check if command exists
	if _, err := exec.LookPath(cmdParts[0]); err != nil {
		if critical {
			c.stats.FilesFailed++
			return fmt.Errorf("critical command not available: %s", cmdParts[0])
		}
		c.logger.Debug("Command not available: %s (skipping %s)", cmdParts[0], description)
		return nil
	}

	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would execute command with PBS auth for datastore %s: %s > %s", datastoreName, cmd, output)
		return nil
	}

	// Ensure output directory exists
	if err := c.ensureDir(filepath.Dir(output)); err != nil {
		return err
	}

	// Execute command with PBS authentication environment
	execCmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)

	// Set PBS authentication environment variables with datastore
	execCmd.Env = os.Environ()

	// Check if PBS credentials are configured
	if c.config.PBSRepository == "" && c.config.PBSPassword == "" {
		// No PBS credentials configured - skip this command gracefully
		c.logger.Warning("Skipping %s: PBS credentials not configured. Set PBS_REPOSITORY and PBS_PASSWORD in config or environment to collect namespace information.", description)
		return nil
	}

	// Build PBS_REPOSITORY with datastore
	repoWithDatastore := ""
	if c.config.PBSRepository != "" {
		// If repository already has a datastore (contains :), replace it
		// Otherwise append the datastore name
		repoWithDatastore = c.config.PBSRepository
		if strings.Contains(repoWithDatastore, ":") {
			// Replace existing datastore: "user@host:oldds" -> "user@host:newds"
			parts := strings.SplitN(repoWithDatastore, ":", 2)
			repoWithDatastore = fmt.Sprintf("%s:%s", parts[0], datastoreName)
		} else {
			// Append datastore: "user@host" -> "user@host:datastore"
			repoWithDatastore = fmt.Sprintf("%s:%s", repoWithDatastore, datastoreName)
		}
	} else {
		// No repository configured but we have password - use root@pam as default user
		repoWithDatastore = fmt.Sprintf("root@pam@localhost:%s", datastoreName)
		c.logger.Debug("Using default user root@pam for PBS repository")
	}

	execCmd.Env = append(execCmd.Env, fmt.Sprintf("PBS_REPOSITORY=%s", repoWithDatastore))
	c.logger.Debug("Using PBS_REPOSITORY=%s", repoWithDatastore)

	if c.config.PBSPassword != "" {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("PBS_PASSWORD=%s", c.config.PBSPassword))
		c.logger.Debug("Using PBS_PASSWORD (hidden)")
	}
	if c.config.PBSFingerprint != "" {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("PBS_FINGERPRINT=%s", c.config.PBSFingerprint))
		c.logger.Debug("Using PBS_FINGERPRINT=%s", c.config.PBSFingerprint)
	}

	outFile, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		if outFile != nil {
			_ = outFile.Close()
		}
	}()

	var outputBuf bytes.Buffer
	multiWriter := io.MultiWriter(outFile, &outputBuf)
	execCmd.Stdout = multiWriter
	execCmd.Stderr = multiWriter

	if err := execCmd.Run(); err != nil {
		if closeErr := outFile.Close(); closeErr != nil {
			c.logger.Debug("Failed to close output file for %s: %v", description, closeErr)
		}
		outFile = nil
		if removeErr := os.Remove(output); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			c.logger.Debug("Failed to remove incomplete output %s: %v", output, removeErr)
		}
		if critical {
			c.stats.FilesFailed++
			return fmt.Errorf("critical command failed for %s: %w", description, err)
		}
		cmdString := cmd
		if len(cmdParts) > 0 {
			cmdString = strings.Join(cmdParts, " ")
		}
		c.logger.Warning("Skipping %s: command `%s` failed (%v). Non-critical; backup continues. Ensure the PBS CLI is available and has proper permissions. Output: %s",
			description,
			cmdString,
			err,
			summarizeCommandOutput(&outputBuf),
		)
		return nil // Non-critical failure
	}

	c.stats.FilesProcessed++
	c.logger.Debug("Successfully collected %s via PBS-authenticated command for datastore %s: %s", description, datastoreName, cmd)

	return nil
}

func summarizeCommandOutput(buf *bytes.Buffer) string {
	return summarizeCommandOutputText(buf.String())
}

func summarizeCommandOutputText(text string) string {
	const maxLen = 2048
	output := strings.TrimSpace(text)
	if output == "" {
		return "(no stdout/stderr)"
	}
	output = strings.ReplaceAll(output, "\n", " | ")
	runes := []rune(output)
	if len(runes) > maxLen {
		runes = append(runes[:maxLen], '…')
	}
	return string(runes)
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "entry"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"@", "_",
		":", "_",
	)
	clean := replacer.Replace(name)
	clean = strings.ReplaceAll(clean, "..", "_")
	if clean == "" {
		clean = "entry"
	}
	return clean
}

// GetStats returns current collection statistics
func (c *Collector) GetStats() *CollectionStats {
	return c.stats
}

func (c *Collector) writeReportFile(path string, data []byte) error {
	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would write report file: %s (%d bytes)", path, len(data))
		return nil
	}

	if err := c.ensureDir(filepath.Dir(path)); err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0640); err != nil {
		c.stats.FilesFailed++
		return fmt.Errorf("failed to write report %s: %w", path, err)
	}

	c.stats.FilesProcessed++
	c.stats.BytesCollected += int64(len(data))
	c.logger.Debug("Successfully wrote report file: %s", path)
	return nil
}

func (c *Collector) captureCommandOutput(ctx context.Context, cmd, output, description string, critical bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	if _, err := exec.LookPath(parts[0]); err != nil {
		if critical {
			c.stats.FilesFailed++
			return nil, fmt.Errorf("critical command not available: %s", parts[0])
		}
		c.logger.Debug("Command not available: %s (skipping %s)", parts[0], description)
		return nil, nil
	}

	if c.dryRun {
		c.logger.Debug("[DRY RUN] Would execute command: %s > %s", cmd, output)
		return nil, nil
	}

	execCmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	out, err := execCmd.CombinedOutput()
	if err != nil {
		cmdString := strings.Join(parts, " ")
		if critical {
			c.stats.FilesFailed++
			return nil, fmt.Errorf("critical command `%s` failed for %s: %w (output: %s)", cmdString, description, err, summarizeCommandOutputText(string(out)))
		}
		c.logger.Warning("Skipping %s: command `%s` failed (%v). Non-critical; backup continues. Output: %s",
			description,
			cmdString,
			err,
			summarizeCommandOutputText(string(out)),
		)
		return nil, nil
	}

	if err := c.writeReportFile(output, out); err != nil {
		return nil, err
	}

	return out, nil
}

func (c *Collector) collectCommandMulti(ctx context.Context, cmd, output, description string, critical bool, mirrors ...string) error {
	if output == "" {
		return fmt.Errorf("primary output path cannot be empty for %s", description)
	}

	data, err := c.captureCommandOutput(ctx, cmd, output, description, critical)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}

	for _, mirror := range mirrors {
		if mirror == "" {
			continue
		}
		if err := c.writeReportFile(mirror, data); err != nil {
			return err
		}
	}

	return nil
}

func (c *Collector) collectCommandOptional(ctx context.Context, cmd, output, description string, mirrors ...string) {
	if output == "" {
		c.logger.Debug("Optional command %s skipped: no primary output path", description)
		return
	}

	data, err := c.captureCommandOutput(ctx, cmd, output, description, false)
	if err != nil {
		c.logger.Debug("Optional command %s skipped: %v", description, err)
		return
	}
	if len(data) == 0 {
		return
	}

	for _, mirror := range mirrors {
		if mirror == "" {
			continue
		}
		if err := c.writeReportFile(mirror, data); err != nil {
			c.logger.Debug("Failed to mirror %s to %s: %v", description, mirror, err)
		}
	}
}

func (c *Collector) sampleDirectories(ctx context.Context, root string, maxDepth, limit int) ([]string, error) {
	results := make([]string, 0, limit)
	stopErr := errors.New("directory sample limit reached")
	start := time.Now()
	lastLog := start
	visited := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if path == root {
			return nil
		}

		if d.IsDir() {
			visited++
			if time.Since(lastLog) > 2*time.Second {
				c.logger.Debug("PXAR sampleDirectories: root=%s visited=%d selected=%d", root, visited, len(results))
				lastLog = time.Now()
			}
		}

		if c.shouldExclude(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		depth := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() {
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			if len(results) < limit {
				results = append(results, filepath.ToSlash(rel))
				if len(results) >= limit {
					return stopErr
				}
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, stopErr) && !errors.Is(err, context.Canceled) {
		return results, err
	}

	c.logger.Debug("PXAR sampleDirectories: root=%s completed (selected=%d visited=%d duration=%s)",
		root, len(results), visited, time.Since(start).Truncate(time.Millisecond))
	return results, nil
}

func (c *Collector) sampleFiles(ctx context.Context, root string, patterns []string, maxDepth, limit int) ([]FileSummary, error) {
	results := make([]FileSummary, 0, limit)
	stopErr := errors.New("file sample limit reached")
	start := time.Now()
	lastLog := start
	visited := 0
	matched := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if path == root {
			return nil
		}

		if d.IsDir() {
			if c.shouldExclude(path) {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			depth := strings.Count(rel, string(filepath.Separator))
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			if time.Since(lastLog) > 2*time.Second {
				c.logger.Debug("PXAR sampleFiles: root=%s visited=%d matched=%d selected=%d", root, visited, matched, len(results))
				lastLog = time.Now()
			}
			return nil
		}

		if c.shouldExclude(path) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		visited++

		if len(patterns) > 0 && !matchAnyPattern(patterns, filepath.Base(path), rel) {
			return nil
		}
		matched++

		info, err := d.Info()
		if err != nil {
			return nil
		}

		results = append(results, FileSummary{
			RelativePath: filepath.ToSlash(rel),
			SizeBytes:    info.Size(),
			SizeHuman:    FormatBytes(info.Size()),
			ModTime:      info.ModTime(),
		})

		if len(results) >= limit {
			return stopErr
		}

		return nil
	})

	if err != nil && !errors.Is(err, stopErr) && !errors.Is(err, context.Canceled) {
		return results, err
	}

	c.logger.Debug("PXAR sampleFiles: root=%s completed (selected=%d matched=%d visited=%d duration=%s)",
		root, len(results), matched, visited, time.Since(start).Truncate(time.Millisecond))
	return results, nil
}

func matchAnyPattern(patterns []string, name, relative string) bool {
	if len(patterns) == 0 {
		return true
	}
	normalizedRel := filepath.ToSlash(relative)
	for _, pattern := range patterns {
		p := filepath.ToSlash(pattern)
		if ok, _ := filepath.Match(p, normalizedRel); ok {
			return true
		}
		if ok, _ := filepath.Match(p, filepath.ToSlash(name)); ok {
			return true
		}
	}
	return false
}
