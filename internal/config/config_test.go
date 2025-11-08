package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tis24dev/proxmox-backup/internal/types"
)

func setBaseDirEnv(t *testing.T, value string) func() {
	t.Helper()

	prev := os.Getenv("BASE_DIR")
	if value == "" {
		_ = os.Unsetenv("BASE_DIR")
	} else {
		if err := os.Setenv("BASE_DIR", value); err != nil {
			t.Fatalf("failed to set BASE_DIR: %v", err)
		}
	}

	return func() {
		if prev == "" {
			_ = os.Unsetenv("BASE_DIR")
		} else {
			_ = os.Setenv("BASE_DIR", prev)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.env")

	content := `# Test configuration
BACKUP_ENABLED=true
DEBUG_LEVEL=5
USE_COLOR=true
COMPRESSION_TYPE=xz
COMPRESSION_LEVEL=9
BACKUP_PATH=/test/backup
LOG_PATH=/test/log
LOCK_PATH=/test/lock
SECONDARY_ENABLED=true
SECONDARY_PATH=/test/secondary
LOCAL_RETENTION_DAYS=7
TELEGRAM_ENABLED=false
METRICS_ENABLED=true
BACKUP_PVE_JOBS=false
BACKUP_PXAR_FILES=false
CUSTOM_BACKUP_PATHS=/etc/custom,/var/data
BACKUP_BLACKLIST=/var/data/tmp
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cleanup := setBaseDirEnv(t, "/env/base/dir")
	defer cleanup()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Test parsed values
	if !cfg.BackupEnabled {
		t.Error("Expected BackupEnabled to be true")
	}

	if cfg.DebugLevel != types.LogLevelDebug {
		t.Errorf("DebugLevel = %v; want %v", cfg.DebugLevel, types.LogLevelDebug)
	}

	if !cfg.UseColor {
		t.Error("Expected UseColor to be true")
	}

	if cfg.CompressionType != types.CompressionXZ {
		t.Errorf("CompressionType = %v; want %v", cfg.CompressionType, types.CompressionXZ)
	}

	if cfg.CompressionLevel != 9 {
		t.Errorf("CompressionLevel = %d; want 9", cfg.CompressionLevel)
	}

	if cfg.BackupPath != "/test/backup" {
		t.Errorf("BackupPath = %q; want %q", cfg.BackupPath, "/test/backup")
	}

	if !cfg.SecondaryEnabled {
		t.Error("Expected SecondaryEnabled to be true")
	}

	if cfg.SecondaryPath != "/test/secondary" {
		t.Errorf("SecondaryPath = %q; want %q", cfg.SecondaryPath, "/test/secondary")
	}

	if cfg.LocalRetentionDays != 7 {
		t.Errorf("LocalRetentionDays = %d; want 7", cfg.LocalRetentionDays)
	}

	if cfg.TelegramEnabled {
		t.Error("Expected TelegramEnabled to be false")
	}

	if !cfg.MetricsEnabled {
		t.Error("Expected MetricsEnabled to be true")
	}

	if cfg.BaseDir != "/env/base/dir" {
		t.Errorf("BaseDir = %q; want %q", cfg.BaseDir, "/env/base/dir")
	}

	if cfg.BackupPVEJobs {
		t.Error("Expected BackupPVEJobs to be false")
	}

	if cfg.BackupPxarFiles {
		t.Error("Expected BackupPxarFiles to be false")
	}

	if len(cfg.CustomBackupPaths) != 2 || cfg.CustomBackupPaths[0] != "/etc/custom" || cfg.CustomBackupPaths[1] != "/var/data" {
		t.Errorf("CustomBackupPaths = %#v; want [/etc/custom /var/data]", cfg.CustomBackupPaths)
	}

	if len(cfg.BackupBlacklist) != 1 || cfg.BackupBlacklist[0] != "/var/data/tmp" {
		t.Errorf("BackupBlacklist = %#v; want [/var/data/tmp]", cfg.BackupBlacklist)
	}
}

func TestLoadConfigNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.env")
	if err == nil {
		t.Error("Expected error for nonexistent config file")
	}
}

func TestLoadConfigWithQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_quotes.env")

	content := `BACKUP_PATH="/path/with spaces/backup"
CLOUD_REMOTE='my-remote'
LOG_PATH=/path/without/quotes
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cleanup := setBaseDirEnv(t, "/quotes/base")
	defer cleanup()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.BackupPath != "/path/with spaces/backup" {
		t.Errorf("BackupPath = %q; want %q", cfg.BackupPath, "/path/with spaces/backup")
	}

	if cfg.CloudRemote != "my-remote" {
		t.Errorf("CloudRemote = %q; want %q", cfg.CloudRemote, "my-remote")
	}

	if cfg.LogPath != "/path/without/quotes" {
		t.Errorf("LogPath = %q; want %q", cfg.LogPath, "/path/without/quotes")
	}

	if cfg.BaseDir != "/quotes/base" {
		t.Errorf("BaseDir = %q; want %q", cfg.BaseDir, "/quotes/base")
	}
}

func TestLoadConfigWithComments(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_comments.env")

	content := `# This is a comment
BACKUP_ENABLED=true
# Another comment
  # Comment with spaces
COMPRESSION_TYPE=xz

# Empty line above
DEBUG_LEVEL=4
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cleanup := setBaseDirEnv(t, "/comments/base")
	defer cleanup()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if !cfg.BackupEnabled {
		t.Error("Expected BackupEnabled to be true")
	}

	if cfg.CompressionType != types.CompressionXZ {
		t.Errorf("CompressionType = %v; want %v", cfg.CompressionType, types.CompressionXZ)
	}

	if cfg.DebugLevel != types.LogLevelInfo {
		t.Errorf("DebugLevel = %v; want %v", cfg.DebugLevel, types.LogLevelInfo)
	}
}

func TestConfigGetSet(t *testing.T) {
	cfg := &Config{
		raw: make(map[string]string),
	}

	// Test Set
	cfg.Set("TEST_KEY", "test_value")

	// Test Get
	value, ok := cfg.Get("TEST_KEY")
	if !ok {
		t.Error("Expected key TEST_KEY to exist")
	}
	if value != "test_value" {
		t.Errorf("Get(TEST_KEY) = %q; want %q", value, "test_value")
	}

	// Test Get non-existent key
	_, ok = cfg.Get("NON_EXISTENT")
	if ok {
		t.Error("Expected NON_EXISTENT key to not exist")
	}
}

func TestConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "empty.env")

	// Create empty config file
	if err := os.WriteFile(configPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cleanup := setBaseDirEnv(t, "/defaults/base")
	defer cleanup()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Test default values
	if !cfg.BackupEnabled {
		t.Error("Expected default BackupEnabled to be true")
	}

	if cfg.DebugLevel != types.LogLevelInfo {
		t.Errorf("Default DebugLevel = %v; want %v", cfg.DebugLevel, types.LogLevelInfo)
	}

	if cfg.CompressionType != types.CompressionXZ {
		t.Errorf("Default CompressionType = %v; want %v", cfg.CompressionType, types.CompressionXZ)
	}

	if cfg.CompressionLevel != 6 {
		t.Errorf("Default CompressionLevel = %d; want 6", cfg.CompressionLevel)
	}

	if cfg.LocalRetentionDays != 7 {
		t.Errorf("Default LocalRetentionDays = %d; want 7", cfg.LocalRetentionDays)
	}

	if !cfg.EnableGoBackup {
		t.Error("Expected default EnableGoBackup to be true")
	}

	if cfg.BaseDir != "/defaults/base" {
		t.Errorf("Default BaseDir = %q; want %q", cfg.BaseDir, "/defaults/base")
	}
}

func TestEnableGoBackupFlag(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "go_pipeline.env")

	content := `ENABLE_GO_BACKUP=false
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cleanup := setBaseDirEnv(t, "/flag/base")
	defer cleanup()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.EnableGoBackup {
		t.Error("Expected EnableGoBackup to be false when explicitly disabled")
	}
}

func TestLoadConfigBaseDirFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "base_dir.env")

	content := `BASE_DIR=/custom/base
BACKUP_PATH=${BASE_DIR}/backup-data
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cleanup := setBaseDirEnv(t, "")
	defer cleanup()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.BaseDir != "/custom/base" {
		t.Errorf("BaseDir = %q; want %q", cfg.BaseDir, "/custom/base")
	}

	if cfg.BackupPath != "/custom/base/backup-data" {
		t.Errorf("BackupPath = %q; want %q", cfg.BackupPath, "/custom/base/backup-data")
	}
}
