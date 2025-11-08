package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tis24dev/proxmox-backup/internal/types"
	"github.com/tis24dev/proxmox-backup/pkg/utils"
)

// Config contiene tutta la configurazione del sistema di backup
type Config struct {
	// General settings
	BackupEnabled  bool
	DebugLevel     types.LogLevel
	UseColor       bool
	EnableGoBackup bool
	BaseDir        string

	// Compression settings
	CompressionType    types.CompressionType
	CompressionLevel   int
	CompressionThreads int
	CompressionMode    string

	// Safety settings
	MinDiskPrimaryGB   float64
	MinDiskSecondaryGB float64
	MinDiskCloudGB     float64
	SafetyFactor       float64

	// Optimization settings
	EnableSmartChunking    bool
	EnableDeduplication    bool
	EnablePrefilter        bool
	ChunkSizeMB            int
	ChunkThresholdMB       int
	PrefilterMaxFileSizeMB int

	// Paths
	BackupPath    string
	LogPath       string
	LockPath      string
	SecureAccount string

	// Storage settings
	SecondaryEnabled bool
	SecondaryPath    string
	CloudEnabled     bool
	CloudRemote      string

	// Retention settings
	LocalRetentionDays     int
	SecondaryRetentionDays int
	CloudRetentionDays     int

	// Notifications
	TelegramEnabled bool
	TelegramToken   string
	TelegramChatID  string
	EmailEnabled    bool
	EmailRecipients string

	// Metrics
	MetricsEnabled bool
	MetricsPath    string

	// Collector options
	ExcludePatterns []string

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

	CustomBackupPaths []string
	BackupBlacklist   []string

	// raw configuration map
	raw map[string]string
}

// LoadConfig legge il file di configurazione backup.env
func LoadConfig(configPath string) (*Config, error) {
	if !utils.FileExists(configPath) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	cfg := &Config{
		raw: make(map[string]string),
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip comments and empty lines
		if utils.IsComment(line) {
			continue
		}

		// Parse key=value
		key, value, ok := utils.SplitKeyValue(line)
		if !ok {
			// Invalid line, skip or log warning
			continue
		}

		cfg.raw[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse configuration
	if err := cfg.parse(); err != nil {
		return nil, fmt.Errorf("error parsing configuration: %w", err)
	}

	return cfg, nil
}

// parse interpreta i valori raw della configurazione
// Supporta sia il formato legacy che quello nuovo del backup.env
func (c *Config) parse() error {
	// General settings
	c.BackupEnabled = c.getBool("BACKUP_ENABLED", true)

	// DEBUG_LEVEL: supporta sia numerico che string ("standard", "advanced", "extreme")
	c.DebugLevel = c.getLogLevel("DEBUG_LEVEL", types.LogLevelInfo)

	// USE_COLOR vs DISABLE_COLORS (invertito)
	if disableColors, ok := c.raw["DISABLE_COLORS"]; ok {
		c.UseColor = !utils.ParseBool(disableColors)
	} else {
		c.UseColor = c.getBool("USE_COLOR", true)
	}

	// Compression
	c.CompressionType = c.getCompressionType("COMPRESSION_TYPE", types.CompressionXZ)
	c.CompressionLevel = c.getInt("COMPRESSION_LEVEL", 6)
	c.CompressionThreads = c.getInt("COMPRESSION_THREADS", 0) // 0 = auto
	c.CompressionMode = strings.ToLower(c.getString("COMPRESSION_MODE", "standard"))
	if c.CompressionMode == "" {
		c.CompressionMode = "standard"
	}
	c.CompressionLevel = adjustLevelForMode(c.CompressionType, c.CompressionMode, c.CompressionLevel)

	// Optimizations
	c.EnableSmartChunking = c.getBool("ENABLE_SMART_CHUNKING", false)
	c.EnableDeduplication = c.getBool("ENABLE_DEDUPLICATION", false)
	c.EnablePrefilter = c.getBool("ENABLE_PREFILTER", false)
	c.ChunkSizeMB = c.getInt("CHUNK_SIZE_MB", 10)
	if c.ChunkSizeMB <= 0 {
		c.ChunkSizeMB = 10
	}
	c.ChunkThresholdMB = c.getInt("CHUNK_THRESHOLD_MB", 50)
	if c.ChunkThresholdMB <= 0 {
		c.ChunkThresholdMB = 50
	}
	c.PrefilterMaxFileSizeMB = c.getInt("PREFILTER_MAX_FILE_SIZE_MB", 8)
	if c.PrefilterMaxFileSizeMB <= 0 {
		c.PrefilterMaxFileSizeMB = 8
	}

	c.MinDiskPrimaryGB = sanitizeMinDisk(c.getFloat("MIN_DISK_SPACE_PRIMARY_GB", 10.0))
	c.MinDiskSecondaryGB = sanitizeMinDisk(c.getFloat("MIN_DISK_SPACE_SECONDARY_GB", c.MinDiskPrimaryGB))
	c.MinDiskCloudGB = sanitizeMinDisk(c.getFloat("MIN_DISK_SPACE_CLOUD_GB", c.MinDiskPrimaryGB))

	// Feature flags
	c.EnableGoBackup = c.getBoolWithFallback([]string{"ENABLE_GO_BACKUP", "ENABLE_GO_PIPELINE"}, true)

	// Base directory (compatibile con lo script Bash: se non specificato, usa env o default)
	envBaseDir := os.Getenv("BASE_DIR")
	c.BaseDir = c.getString("BASE_DIR", envBaseDir)
	if c.BaseDir == "" {
		c.BaseDir = "/opt/proxmox-backup"
	}
	_ = os.Setenv("BASE_DIR", c.BaseDir)

	// Paths: supporta LOCAL_BACKUP_PATH o BACKUP_PATH
	c.BackupPath = c.getStringWithFallback([]string{"LOCAL_BACKUP_PATH", "BACKUP_PATH"}, filepath.Join(c.BaseDir, "backup"))
	c.LogPath = c.getStringWithFallback([]string{"LOCAL_LOG_PATH", "LOG_PATH"}, filepath.Join(c.BaseDir, "log"))
	c.LockPath = c.getString("LOCK_PATH", filepath.Join(c.BaseDir, "lock"))
	c.SecureAccount = c.getString("SECURE_ACCOUNT", filepath.Join(c.BaseDir, "secure_account"))

	// Storage: supporta ENABLE_SECONDARY_BACKUP o SECONDARY_ENABLED
	c.SecondaryEnabled = c.getBoolWithFallback([]string{"ENABLE_SECONDARY_BACKUP", "SECONDARY_ENABLED"}, false)
	c.SecondaryPath = c.getStringWithFallback([]string{"SECONDARY_BACKUP_PATH", "SECONDARY_PATH"}, "")

	c.CloudEnabled = c.getBoolWithFallback([]string{"ENABLE_CLOUD_BACKUP", "CLOUD_ENABLED"}, false)
	c.CloudRemote = c.getStringWithFallback([]string{"RCLONE_REMOTE", "CLOUD_REMOTE"}, "")

	// Retention: supporta MAX_LOCAL_BACKUPS o LOCAL_RETENTION_DAYS
	c.LocalRetentionDays = c.getIntWithFallback([]string{"MAX_LOCAL_BACKUPS", "LOCAL_RETENTION_DAYS"}, 7)
	c.SecondaryRetentionDays = c.getIntWithFallback([]string{"MAX_SECONDARY_BACKUPS", "SECONDARY_RETENTION_DAYS"}, 14)
	c.CloudRetentionDays = c.getIntWithFallback([]string{"MAX_CLOUD_BACKUPS", "CLOUD_RETENTION_DAYS"}, 30)

	c.SafetyFactor = 1.5

	// Notifications
	c.TelegramEnabled = c.getBool("TELEGRAM_ENABLED", false)
	c.TelegramToken = c.getString("TELEGRAM_TOKEN", "")
	c.TelegramChatID = c.getString("TELEGRAM_CHAT_ID", "")
	c.EmailEnabled = c.getBool("EMAIL_ENABLED", false)
	c.EmailRecipients = c.getString("EMAIL_RECIPIENTS", "")

	// Metrics: supporta PROMETHEUS_ENABLED o METRICS_ENABLED
	c.MetricsEnabled = c.getBoolWithFallback([]string{"PROMETHEUS_ENABLED", "METRICS_ENABLED"}, false)
	c.MetricsPath = c.getString("METRICS_PATH", filepath.Join(c.BaseDir, "metrics"))

	if patterns := c.getStringSlice("BACKUP_EXCLUDE_PATTERNS", nil); patterns != nil {
		c.ExcludePatterns = patterns
	} else {
		c.ExcludePatterns = []string{}
	}

	// PVE-specific collection options
	c.BackupVMConfigs = c.getBool("BACKUP_VM_CONFIGS", true)
	c.BackupClusterConfig = c.getBool("BACKUP_CLUSTER_CONFIG", true)
	c.BackupPVEFirewall = c.getBool("BACKUP_PVE_FIREWALL", true)
	c.BackupVZDumpConfig = c.getBool("BACKUP_VZDUMP_CONFIG", true)
	c.BackupPVEACL = c.getBool("BACKUP_PVE_ACL", true)
	c.BackupPVEJobs = c.getBool("BACKUP_PVE_JOBS", true)
	c.BackupPVESchedules = c.getBool("BACKUP_PVE_SCHEDULES", true)
	c.BackupPVEReplication = c.getBool("BACKUP_PVE_REPLICATION", true)
	c.BackupPVEBackupFiles = c.getBool("BACKUP_PVE_BACKUP_FILES", true)
	c.BackupCephConfig = c.getBool("BACKUP_CEPH_CONFIG", true)

	// PBS-specific collection options
	c.BackupDatastoreConfigs = c.getBool("BACKUP_DATASTORE_CONFIGS", true)
	c.BackupUserConfigs = c.getBool("BACKUP_USER_CONFIGS", true)
	c.BackupRemoteConfigs = c.getBool("BACKUP_REMOTE_CONFIGS", true)
	c.BackupSyncJobs = c.getBool("BACKUP_SYNC_JOBS", true)
	c.BackupVerificationJobs = c.getBool("BACKUP_VERIFICATION_JOBS", true)
	c.BackupTapeConfigs = c.getBool("BACKUP_TAPE_CONFIGS", true)
	c.BackupPruneSchedules = c.getBool("BACKUP_PRUNE_SCHEDULES", true)
	c.BackupPxarFiles = c.getBool("BACKUP_PXAR_FILES", true)

	// System collection options
	c.BackupNetworkConfigs = c.getBool("BACKUP_NETWORK_CONFIGS", true)
	c.BackupAptSources = c.getBool("BACKUP_APT_SOURCES", true)
	c.BackupCronJobs = c.getBool("BACKUP_CRON_JOBS", true)
	c.BackupSystemdServices = c.getBool("BACKUP_SYSTEMD_SERVICES", true)
	c.BackupSSLCerts = c.getBool("BACKUP_SSL_CERTS", true)
	c.BackupSysctlConfig = c.getBool("BACKUP_SYSCTL_CONFIG", true)
	c.BackupKernelModules = c.getBool("BACKUP_KERNEL_MODULES", true)
	c.BackupFirewallRules = c.getBool("BACKUP_FIREWALL_RULES", true)
	c.BackupInstalledPackages = c.getBool("BACKUP_INSTALLED_PACKAGES", true)
	c.BackupScriptDir = c.getBool("BACKUP_SCRIPT_DIR", true)
	c.BackupCriticalFiles = c.getBool("BACKUP_CRITICAL_FILES", true)
	c.BackupSSHKeys = c.getBool("BACKUP_SSH_KEYS", true)
	c.BackupZFSConfig = c.getBool("BACKUP_ZFS_CONFIG", true)
	c.BackupRootHome = c.getBool("BACKUP_ROOT_HOME", true)
	c.BackupScriptRepository = c.getBool("BACKUP_SCRIPT_REPOSITORY", true)
	c.BackupUserHomes = c.getBool("BACKUP_USER_HOMES", true)

	c.CustomBackupPaths = normalizeList(c.getStringSlice("CUSTOM_BACKUP_PATHS", nil))
	c.BackupBlacklist = normalizeList(c.getStringSlice("BACKUP_BLACKLIST", nil))

	return nil
}

// Helper methods per ottenere valori tipizzati

func (c *Config) getString(key, defaultValue string) string {
	if val, ok := c.raw[key]; ok {
		return expandEnvVars(val)
	}
	return defaultValue
}

func (c *Config) getBool(key string, defaultValue bool) bool {
	if val, ok := c.raw[key]; ok {
		return utils.ParseBool(val)
	}
	return defaultValue
}

func (c *Config) getInt(key string, defaultValue int) int {
	if val, ok := c.raw[key]; ok {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func (c *Config) getLogLevel(key string, defaultValue types.LogLevel) types.LogLevel {
	if val, ok := c.raw[key]; ok {
		// Try numeric first
		if intVal, err := strconv.Atoi(val); err == nil {
			return types.LogLevel(intVal)
		}
		// Try string values: "standard", "advanced", "extreme"
		switch val {
		case "standard":
			return types.LogLevelInfo
		case "advanced":
			return types.LogLevelDebug
		case "extreme":
			return types.LogLevelDebug
		}
	}
	return defaultValue
}

func (c *Config) getCompressionType(key string, defaultValue types.CompressionType) types.CompressionType {
	if val, ok := c.raw[key]; ok {
		return types.CompressionType(val)
	}
	return defaultValue
}

func (c *Config) getStringSlice(key string, defaultValue []string) []string {
	val, ok := c.raw[key]
	if !ok {
		return defaultValue
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return []string{}
	}

	parts := strings.FieldsFunc(val, func(r rune) bool {
		switch r {
		case ',', ';', ':', '|', '\n':
			return true
		default:
			return false
		}
	})

	var result []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return []string{}
	}
	return result
}

// Helper methods with fallback support (try multiple keys)

// expandEnvVars expands environment variables and special variables like ${BASE_DIR}
func expandEnvVars(s string) string {
	// Expand ${VAR} and $VAR style variables
	result := os.Expand(s, func(key string) string {
		// Special handling for BASE_DIR
		if key == "BASE_DIR" {
			// Check if BASE_DIR is set in environment, otherwise use default
			if val := os.Getenv("BASE_DIR"); val != "" {
				return val
			}
			return "/opt/proxmox-backup"
		}
		return os.Getenv(key)
	})
	return result
}

func (c *Config) getStringWithFallback(keys []string, defaultValue string) string {
	for _, key := range keys {
		if val, ok := c.raw[key]; ok && val != "" {
			return expandEnvVars(val)
		}
	}
	return defaultValue
}

func (c *Config) getBoolWithFallback(keys []string, defaultValue bool) bool {
	for _, key := range keys {
		if val, ok := c.raw[key]; ok {
			return utils.ParseBool(val)
		}
	}
	return defaultValue
}

func (c *Config) getIntWithFallback(keys []string, defaultValue int) int {
	for _, key := range keys {
		if val, ok := c.raw[key]; ok {
			if intVal, err := strconv.Atoi(val); err == nil {
				return intVal
			}
		}
	}
	return defaultValue
}

func (c *Config) getFloat(key string, defaultValue float64) float64 {
	if val, ok := c.raw[key]; ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func adjustLevelForMode(comp types.CompressionType, mode string, current int) int {
	switch mode {
	case "fast":
		return 1
	case "maximum":
		switch comp {
		case types.CompressionZstd:
			return 19
		case types.CompressionXZ, types.CompressionGzip, types.CompressionPigz,
			types.CompressionBzip2, types.CompressionLZMA:
			return 9
		default:
			return current
		}
	case "ultra":
		switch comp {
		case types.CompressionZstd:
			return 22
		case types.CompressionXZ, types.CompressionGzip, types.CompressionPigz,
			types.CompressionBzip2, types.CompressionLZMA:
			return 9
		default:
			return current
		}
	default:
		return current
	}
}

func sanitizeMinDisk(value float64) float64 {
	if value <= 0 {
		return 10.0
	}
	return value
}

// Get restituisce un valore raw dalla configurazione
func (c *Config) Get(key string) (string, bool) {
	val, ok := c.raw[key]
	return val, ok
}

// Set imposta un valore nella configurazione
func (c *Config) Set(key, value string) {
	c.raw[key] = value
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	clean := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return []string{}
	}
	return clean
}

// expandEnvVars expands environment variables and special variables like ${BASE_DIR}
