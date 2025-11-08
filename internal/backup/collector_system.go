package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CollectSystemInfo collects common system information (both PVE and PBS)
func (c *Collector) CollectSystemInfo(ctx context.Context) error {
	c.logger.Info("Collecting system information")

	ensureSystemPath()

	// Collect system directories
	if err := c.collectSystemDirectories(ctx); err != nil {
		return fmt.Errorf("failed to collect system directories: %w", err)
	}

	// Collect system commands output
	if err := c.collectSystemCommands(ctx); err != nil {
		return fmt.Errorf("failed to collect system commands: %w", err)
	}

	// Collect kernel information
	if err := c.collectKernelInfo(ctx); err != nil {
		c.logger.Warning("Failed to collect kernel info: %v", err)
		// Non-fatal, continue
	}

	// Collect hardware information
	if err := c.collectHardwareInfo(ctx); err != nil {
		c.logger.Warning("Failed to collect hardware info: %v", err)
		// Non-fatal, continue
	}

	if c.config.BackupCriticalFiles {
		if err := c.collectCriticalFiles(ctx); err != nil {
			c.logger.Warning("Failed to collect critical files: %v", err)
		}
	}

	if len(c.config.CustomBackupPaths) > 0 {
		if err := c.collectCustomPaths(ctx); err != nil {
			c.logger.Warning("Failed to collect custom paths: %v", err)
		}
	}

	if c.config.BackupScriptDir {
		if err := c.collectScriptDirectories(ctx); err != nil {
			c.logger.Warning("Failed to collect script directories: %v", err)
		}
	}

	if c.config.BackupScriptRepository {
		if err := c.collectScriptRepository(ctx); err != nil {
			c.logger.Warning("Failed to collect script repository: %v", err)
		}
	}

	if c.config.BackupSSHKeys {
		if err := c.collectSSHKeys(ctx); err != nil {
			c.logger.Warning("Failed to collect SSH keys: %v", err)
		}
	}

	if c.config.BackupRootHome {
		if err := c.collectRootHome(ctx); err != nil {
			c.logger.Warning("Failed to collect root home files: %v", err)
		}
	}

	if c.config.BackupUserHomes {
		if err := c.collectUserHomes(ctx); err != nil {
			c.logger.Warning("Failed to collect user home directories: %v", err)
		}
	}

	c.logger.Info("System information collection completed")
	return nil
}

// collectSystemDirectories collects system configuration directories
func (c *Collector) collectSystemDirectories(ctx context.Context) error {
	// Network configuration
	if c.config.BackupNetworkConfigs {
		if err := c.safeCopyFile(ctx,
			"/etc/network/interfaces",
			filepath.Join(c.tempDir, "etc/network/interfaces"),
			"Network interfaces"); err != nil {
			c.logger.Debug("No /etc/network/interfaces found")
		}

		// Additional network configs
		if err := c.safeCopyDir(ctx,
			"/etc/network/interfaces.d",
			filepath.Join(c.tempDir, "etc/network/interfaces.d"),
			"Network interfaces.d"); err != nil {
			c.logger.Debug("No /etc/network/interfaces.d found")
		}
	}

	// Hostname and hosts
	if err := c.safeCopyFile(ctx,
		"/etc/hostname",
		filepath.Join(c.tempDir, "etc/hostname"),
		"Hostname"); err != nil {
		c.logger.Debug("No /etc/hostname found")
	}

	if err := c.safeCopyFile(ctx,
		"/etc/hosts",
		filepath.Join(c.tempDir, "etc/hosts"),
		"Hosts file"); err != nil {
		c.logger.Debug("No /etc/hosts found")
	}

	// DNS configuration
	if err := c.safeCopyFile(ctx,
		"/etc/resolv.conf",
		filepath.Join(c.tempDir, "etc/resolv.conf"),
		"DNS resolver"); err != nil {
		c.logger.Debug("No /etc/resolv.conf found")
	}

	// Timezone configuration
	if err := c.safeCopyFile(ctx,
		"/etc/timezone",
		filepath.Join(c.tempDir, "etc/timezone"),
		"Timezone configuration"); err != nil {
		c.logger.Debug("No /etc/timezone found")
	}

	// Apt sources
	if c.config.BackupAptSources {
		if err := c.safeCopyFile(ctx,
			"/etc/apt/sources.list",
			filepath.Join(c.tempDir, "etc/apt/sources.list"),
			"APT sources"); err != nil {
			c.logger.Debug("No /etc/apt/sources.list found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/apt/sources.list.d",
			filepath.Join(c.tempDir, "etc/apt/sources.list.d"),
			"APT sources.list.d"); err != nil {
			c.logger.Debug("No /etc/apt/sources.list.d found")
		}

		// APT preferences
		if err := c.safeCopyFile(ctx,
			"/etc/apt/preferences",
			filepath.Join(c.tempDir, "etc/apt/preferences"),
			"APT preferences"); err != nil {
			c.logger.Debug("No /etc/apt/preferences found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/apt/preferences.d",
			filepath.Join(c.tempDir, "etc/apt/preferences.d"),
			"APT preferences.d"); err != nil {
			c.logger.Debug("No /etc/apt/preferences.d found")
		}

		// APT authentication keys
		if err := c.safeCopyDir(ctx,
			"/etc/apt/trusted.gpg.d",
			filepath.Join(c.tempDir, "etc/apt/trusted.gpg.d"),
			"APT GPG keys"); err != nil {
			c.logger.Debug("No /etc/apt/trusted.gpg.d found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/apt/apt.conf.d",
			filepath.Join(c.tempDir, "etc/apt/apt.conf.d"),
			"APT apt.conf.d"); err != nil {
			c.logger.Debug("No /etc/apt/apt.conf.d found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/apt/auth.conf.d",
			filepath.Join(c.tempDir, "etc/apt/auth.conf.d"),
			"APT auth.conf.d"); err != nil {
			c.logger.Debug("No /etc/apt/auth.conf.d found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/apt/keyrings",
			filepath.Join(c.tempDir, "etc/apt/keyrings"),
			"APT keyrings"); err != nil {
			c.logger.Debug("No /etc/apt/keyrings found")
		}

		if err := c.safeCopyFile(ctx,
			"/etc/apt/listchanges.conf",
			filepath.Join(c.tempDir, "etc/apt/listchanges.conf"),
			"APT listchanges.conf"); err != nil {
			c.logger.Debug("No /etc/apt/listchanges.conf found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/apt/listchanges.conf.d",
			filepath.Join(c.tempDir, "etc/apt/listchanges.conf.d"),
			"APT listchanges.conf.d"); err != nil {
			c.logger.Debug("No /etc/apt/listchanges.conf.d found")
		}
	}

	// Cron jobs
	if c.config.BackupCronJobs {
		if err := c.safeCopyFile(ctx,
			"/etc/crontab",
			filepath.Join(c.tempDir, "etc/crontab"),
			"System crontab"); err != nil {
			c.logger.Debug("No /etc/crontab found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/cron.d",
			filepath.Join(c.tempDir, "etc/cron.d"),
			"Cron.d directory"); err != nil {
			c.logger.Debug("No /etc/cron.d found")
		}

		// Cron scripts directories
		cronDirs := []string{
			"/etc/cron.daily",
			"/etc/cron.hourly",
			"/etc/cron.monthly",
			"/etc/cron.weekly",
		}
		for _, dir := range cronDirs {
			if err := c.safeCopyDir(ctx, dir,
				filepath.Join(c.tempDir, dir[1:]), // Remove leading /
				filepath.Base(dir)); err != nil {
				c.logger.Debug("No %s found", dir)
			}
		}

		// Per-user crontabs
		if err := c.safeCopyDir(ctx,
			"/var/spool/cron/crontabs",
			filepath.Join(c.tempDir, "var/spool/cron/crontabs"),
			"User crontabs"); err != nil {
			c.logger.Debug("No user crontabs found")
		}
	}

	// Systemd services
	if c.config.BackupSystemdServices {
		if err := c.safeCopyDir(ctx,
			"/etc/systemd/system",
			filepath.Join(c.tempDir, "etc/systemd/system"),
			"Systemd services"); err != nil {
			c.logger.Debug("No /etc/systemd/system found")
		}
	}

	// SSL certificates
	if c.config.BackupSSLCerts {
		if err := c.safeCopyDir(ctx,
			"/etc/ssl/certs",
			filepath.Join(c.tempDir, "etc/ssl/certs"),
			"SSL certificates"); err != nil {
			c.logger.Debug("No /etc/ssl/certs found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/ssl/private",
			filepath.Join(c.tempDir, "etc/ssl/private"),
			"SSL private keys"); err != nil {
			c.logger.Debug("No /etc/ssl/private found")
		}

		if err := c.safeCopyFile(ctx,
			"/etc/ssl/openssl.cnf",
			filepath.Join(c.tempDir, "etc/ssl/openssl.cnf"),
			"OpenSSL configuration"); err != nil {
			c.logger.Debug("No /etc/ssl/openssl.cnf found")
		}
	}

	// Sysctl configuration
	if c.config.BackupSysctlConfig {
		if err := c.safeCopyFile(ctx,
			"/etc/sysctl.conf",
			filepath.Join(c.tempDir, "etc/sysctl.conf"),
			"Sysctl configuration"); err != nil {
			c.logger.Debug("No /etc/sysctl.conf found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/sysctl.d",
			filepath.Join(c.tempDir, "etc/sysctl.d"),
			"Sysctl.d directory"); err != nil {
			c.logger.Debug("No /etc/sysctl.d found")
		}
	}

	// Kernel modules
	if c.config.BackupKernelModules {
		if err := c.safeCopyFile(ctx,
			"/etc/modules",
			filepath.Join(c.tempDir, "etc/modules"),
			"Kernel modules"); err != nil {
			c.logger.Debug("No /etc/modules found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/modprobe.d",
			filepath.Join(c.tempDir, "etc/modprobe.d"),
			"Modprobe.d directory"); err != nil {
			c.logger.Debug("No /etc/modprobe.d found")
		}
	}

	// Firewall rules (iptables/nftables)
	if c.config.BackupFirewallRules {
		if err := c.safeCopyDir(ctx,
			"/etc/iptables",
			filepath.Join(c.tempDir, "etc/iptables"),
			"iptables rules"); err != nil {
			c.logger.Debug("No /etc/iptables found")
		}

		if err := c.safeCopyDir(ctx,
			"/etc/nftables.d",
			filepath.Join(c.tempDir, "etc/nftables.d"),
			"nftables rules"); err != nil {
			c.logger.Debug("No /etc/nftables.d found")
		}

		if err := c.safeCopyFile(ctx,
			"/etc/nftables.conf",
			filepath.Join(c.tempDir, "etc/nftables.conf"),
			"nftables configuration"); err != nil {
			c.logger.Debug("No /etc/nftables.conf found")
		}
	}

	// Logrotate configuration
	if err := c.safeCopyDir(ctx,
		"/etc/logrotate.d",
		filepath.Join(c.tempDir, "etc/logrotate.d"),
		"logrotate configuration"); err != nil {
		c.logger.Debug("No /etc/logrotate.d found")
	}

	return nil
}

// collectSystemCommands collects output from system commands
func (c *Collector) collectSystemCommands(ctx context.Context) error {
	commandsDir := filepath.Join(c.tempDir, "commands")
	if err := c.ensureDir(commandsDir); err != nil {
		return fmt.Errorf("failed to create commands directory: %w", err)
	}

	infoDir := filepath.Join(c.tempDir, "var/lib/proxmox-backup-info")
	if err := c.ensureDir(infoDir); err != nil {
		return fmt.Errorf("failed to create system info directory: %w", err)
	}

	// OS release information (CRITICAL)
	if err := c.collectCommandMulti(ctx,
		"cat /etc/os-release",
		filepath.Join(commandsDir, "os_release.txt"),
		"OS release",
		true,
		filepath.Join(infoDir, "os-release.txt")); err != nil {
		return fmt.Errorf("failed to get OS release (critical): %w", err)
	}

	// Kernel version (CRITICAL)
	if err := c.collectCommandMulti(ctx,
		"uname -a",
		filepath.Join(commandsDir, "uname.txt"),
		"Kernel version",
		true,
		filepath.Join(infoDir, "uname.txt")); err != nil {
		return fmt.Errorf("failed to get kernel version (critical): %w", err)
	}

	// Hostname
	c.safeCmdOutput(ctx,
		"hostname -f",
		filepath.Join(commandsDir, "hostname.txt"),
		"Hostname",
		false)

	// IP addresses
	if err := c.collectCommandMulti(ctx,
		"ip addr show",
		filepath.Join(commandsDir, "ip_addr.txt"),
		"IP addresses",
		false,
		filepath.Join(infoDir, "ip_addr.txt")); err != nil {
		return err
	}

	// IP routes
	if err := c.collectCommandMulti(ctx,
		"ip route show",
		filepath.Join(commandsDir, "ip_route.txt"),
		"IP routes",
		false,
		filepath.Join(infoDir, "ip_route.txt")); err != nil {
		return err
	}

	// IP link statistics
	c.collectCommandOptional(ctx,
		"ip -s link",
		filepath.Join(commandsDir, "ip_link.txt"),
		"IP link statistics",
		filepath.Join(infoDir, "ip_link.txt"))

	// DNS resolver
	c.safeCmdOutput(ctx,
		"cat /etc/resolv.conf",
		filepath.Join(commandsDir, "resolv_conf.txt"),
		"DNS configuration",
		false)

	// Disk usage
	if err := c.collectCommandMulti(ctx,
		"df -h",
		filepath.Join(commandsDir, "df.txt"),
		"Disk usage",
		false,
		filepath.Join(infoDir, "disk_space.txt")); err != nil {
		return err
	}

	// Mounted filesystems
	c.safeCmdOutput(ctx,
		"mount",
		filepath.Join(commandsDir, "mount.txt"),
		"Mounted filesystems",
		false)

	// Block devices
	if err := c.collectCommandMulti(ctx,
		"lsblk -f",
		filepath.Join(commandsDir, "lsblk.txt"),
		"Block devices",
		false,
		filepath.Join(infoDir, "lsblk.txt")); err != nil {
		return err
	}

	// Memory information
	if err := c.collectCommandMulti(ctx,
		"free -h",
		filepath.Join(commandsDir, "free.txt"),
		"Memory usage",
		false,
		filepath.Join(infoDir, "memory.txt")); err != nil {
		return err
	}

	// CPU information
	if err := c.collectCommandMulti(ctx,
		"lscpu",
		filepath.Join(commandsDir, "lscpu.txt"),
		"CPU information",
		false,
		filepath.Join(infoDir, "lscpu.txt")); err != nil {
		return err
	}

	// PCI devices
	if err := c.collectCommandMulti(ctx,
		"lspci -v",
		filepath.Join(commandsDir, "lspci.txt"),
		"PCI devices",
		false,
		filepath.Join(infoDir, "lspci.txt")); err != nil {
		return err
	}

	// USB devices
	c.safeCmdOutput(ctx,
		"lsusb",
		filepath.Join(commandsDir, "lsusb.txt"),
		"USB devices",
		false)

	// Systemd services status
	if c.config.BackupSystemdServices {
		if err := c.collectCommandMulti(ctx,
			"systemctl list-units --type=service --all",
			filepath.Join(commandsDir, "systemctl_services.txt"),
			"Systemd services",
			false,
			filepath.Join(infoDir, "services.txt")); err != nil {
			return err
		}

		c.safeCmdOutput(ctx, "systemctl list-unit-files --type=service",
			filepath.Join(commandsDir, "systemctl_service_files.txt"),
			"Systemd service files", false)
	}

	// Installed packages
	if c.config.BackupInstalledPackages {
		packagesDir := filepath.Join(infoDir, "packages")
		if err := c.ensureDir(packagesDir); err != nil {
			return fmt.Errorf("failed to create packages directory: %w", err)
		}

		if err := c.collectCommandMulti(ctx,
			"dpkg -l",
			filepath.Join(commandsDir, "dpkg_list.txt"),
			"Installed packages",
			false,
			filepath.Join(packagesDir, "dpkg_list.txt")); err != nil {
			return err
		}
	}

	// APT policy
	if c.config.BackupAptSources {
		c.safeCmdOutput(ctx,
			"apt-cache policy",
			filepath.Join(commandsDir, "apt_policy.txt"),
			"APT policy",
			false)
	}

	// Firewall status
	if c.config.BackupFirewallRules {
		if err := c.collectCommandMulti(ctx,
			"iptables-save",
			filepath.Join(commandsDir, "iptables.txt"),
			"iptables rules",
			false,
			filepath.Join(infoDir, "iptables.txt")); err != nil {
			return err
		}

		// ip6tables
		if err := c.collectCommandMulti(ctx,
			"ip6tables-save",
			filepath.Join(commandsDir, "ip6tables.txt"),
			"ip6tables rules",
			false,
			filepath.Join(infoDir, "ip6tables.txt")); err != nil {
			return err
		}

		// nftables
		c.safeCmdOutput(ctx,
			"nft list ruleset",
			filepath.Join(commandsDir, "nftables.txt"),
			"nftables rules",
			false)
	}

	// Loaded kernel modules
	if c.config.BackupKernelModules {
		c.safeCmdOutput(ctx,
			"lsmod",
			filepath.Join(commandsDir, "lsmod.txt"),
			"Loaded kernel modules",
			false)
	}

	// Sysctl values
	if c.config.BackupSysctlConfig {
		c.safeCmdOutput(ctx,
			"sysctl -a",
			filepath.Join(commandsDir, "sysctl.txt"),
			"Sysctl values",
			false)
	}

	// ZFS pools (if ZFS is present)
	if c.config.BackupZFSConfig {
		zfsDir := filepath.Join(infoDir, "zfs")
		if err := c.ensureDir(zfsDir); err != nil {
			return fmt.Errorf("failed to create zfs info directory: %w", err)
		}

		if _, err := exec.LookPath("zpool"); err == nil {
			c.collectCommandOptional(ctx,
				"zpool status",
				filepath.Join(commandsDir, "zpool_status.txt"),
				"ZFS pool status",
				filepath.Join(zfsDir, "zpool_status.txt"))

			c.collectCommandOptional(ctx,
				"zpool list",
				filepath.Join(commandsDir, "zpool_list.txt"),
				"ZFS pool list",
				filepath.Join(zfsDir, "zpool_list.txt"))
		}

		if _, err := exec.LookPath("zfs"); err == nil {
			c.collectCommandOptional(ctx,
				"zfs list",
				filepath.Join(commandsDir, "zfs_list.txt"),
				"ZFS filesystem list",
				filepath.Join(zfsDir, "zfs_list.txt"))

			c.collectCommandOptional(ctx,
				"zfs get all",
				filepath.Join(commandsDir, "zfs_get_all.txt"),
				"ZFS properties",
				filepath.Join(zfsDir, "zfs_get_all.txt"))
		}
	}

	// LVM information
	if _, err := os.Stat("/sbin/pvs"); err == nil {
		c.safeCmdOutput(ctx,
			"pvs",
			filepath.Join(commandsDir, "lvm_pvs.txt"),
			"LVM physical volumes",
			false)

		c.safeCmdOutput(ctx,
			"vgs",
			filepath.Join(commandsDir, "lvm_vgs.txt"),
			"LVM volume groups",
			false)

		c.safeCmdOutput(ctx,
			"lvs",
			filepath.Join(commandsDir, "lvm_lvs.txt"),
			"LVM logical volumes",
			false)
	}

	return nil
}

func ensureSystemPath() {
	current := os.Getenv("PATH")
	if current == "" {
		current = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}

	segments := strings.Split(current, string(os.PathListSeparator))
	seen := make(map[string]struct{}, len(segments))
	filtered := make([]string, 0, len(segments))

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if _, ok := seen[seg]; ok {
			continue
		}
		seen[seg] = struct{}{}
		filtered = append(filtered, seg)
	}

	extras := []string{"/usr/local/sbin", "/usr/sbin", "/sbin"}
	for _, extra := range extras {
		if _, ok := seen[extra]; !ok {
			filtered = append(filtered, extra)
			seen[extra] = struct{}{}
		}
	}

	_ = os.Setenv("PATH", strings.Join(filtered, string(os.PathListSeparator)))
}

// collectKernelInfo collects kernel-specific information
func (c *Collector) collectKernelInfo(ctx context.Context) error {
	commandsDir := filepath.Join(c.tempDir, "commands")

	// Kernel command line
	c.safeCmdOutput(ctx,
		"cat /proc/cmdline",
		filepath.Join(commandsDir, "kernel_cmdline.txt"),
		"Kernel command line",
		false)

	// Kernel version details
	c.safeCmdOutput(ctx,
		"cat /proc/version",
		filepath.Join(commandsDir, "kernel_version.txt"),
		"Kernel version details",
		false)

	return nil
}

// collectHardwareInfo collects hardware information
func (c *Collector) collectHardwareInfo(ctx context.Context) error {
	commandsDir := filepath.Join(c.tempDir, "commands")

	// DMI decode (requires root)
	c.safeCmdOutput(ctx,
		"dmidecode",
		filepath.Join(commandsDir, "dmidecode.txt"),
		"Hardware DMI information",
		false)

	// Hardware sensors (if available)
	if _, err := os.Stat("/usr/bin/sensors"); err == nil {
		c.safeCmdOutput(ctx,
			"sensors",
			filepath.Join(commandsDir, "sensors.txt"),
			"Hardware sensors",
			false)
	}

	// SMART status for disks (if available)
	if _, err := os.Stat("/usr/sbin/smartctl"); err == nil {
		// Get list of disks
		c.safeCmdOutput(ctx,
			"smartctl --scan",
			filepath.Join(commandsDir, "smartctl_scan.txt"),
			"SMART scan",
			false)
	}

	return nil
}

func (c *Collector) collectCriticalFiles(ctx context.Context) error {
	criticalFiles := []string{
		"/etc/fstab",
		"/etc/passwd",
		"/etc/group",
		"/etc/shadow",
		"/etc/gshadow",
		"/etc/sudoers",
	}

	for _, file := range criticalFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		dest := filepath.Join(c.tempDir, strings.TrimPrefix(file, "/"))
		if err := c.safeCopyFile(ctx, file, dest, fmt.Sprintf("critical file %s", filepath.Base(file))); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Failed to copy critical file %s: %v", file, err)
		}
	}

	return nil
}

func (c *Collector) collectCustomPaths(ctx context.Context) error {
	seen := make(map[string]struct{})

	for _, rawPath := range c.config.CustomBackupPaths {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}

		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join("/", path)
		}
		absPath = filepath.Clean(absPath)

		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}

		info, err := os.Lstat(absPath)
		if err != nil {
			if !os.IsNotExist(err) {
				c.logger.Debug("Custom path %s not accessible: %v", absPath, err)
			}
			continue
		}

		dest := filepath.Join(c.tempDir, strings.TrimPrefix(absPath, "/"))
		if info.IsDir() {
			if err := c.safeCopyDir(ctx, absPath, dest, fmt.Sprintf("custom directory %s", absPath)); err != nil {
				c.logger.Debug("Failed to copy custom directory %s: %v", absPath, err)
			}
		} else {
			if err := c.safeCopyFile(ctx, absPath, dest, fmt.Sprintf("custom file %s", filepath.Base(absPath))); err != nil {
				c.logger.Debug("Failed to copy custom file %s: %v", absPath, err)
			}
		}
	}

	return nil
}

func (c *Collector) collectScriptDirectories(ctx context.Context) error {
	scriptDirs := []string{
		"/usr/local/bin",
		"/usr/local/sbin",
	}

	for _, dir := range scriptDirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		dest := filepath.Join(c.tempDir, strings.TrimPrefix(dir, "/"))
		if err := c.safeCopyDir(ctx, dir, dest, fmt.Sprintf("scripts in %s", dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Failed to copy script directory %s: %v", dir, err)
		}
	}

	return nil
}

func (c *Collector) collectSSHKeys(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Host keys (public)
	if matches, err := filepath.Glob("/etc/ssh/ssh_host_*"); err == nil {
		for _, file := range matches {
			if !strings.HasSuffix(file, ".pub") {
				continue
			}
			dest := filepath.Join(c.tempDir, strings.TrimPrefix(file, "/"))
			if err := c.safeCopyFile(ctx, file, dest, "SSH host key"); err != nil && !errors.Is(err, os.ErrNotExist) {
				c.logger.Debug("Failed to copy SSH host key %s: %v", file, err)
			}
		}
	}

	// Root SSH keys
	if err := c.safeCopyDir(ctx, "/root/.ssh", filepath.Join(c.tempDir, "root/.ssh"), "root SSH keys"); err != nil && !errors.Is(err, os.ErrNotExist) {
		c.logger.Debug("Failed to copy root SSH keys: %v", err)
	}

	// User SSH keys
	homeEntries, err := os.ReadDir("/home")
	if err == nil {
		for _, entry := range homeEntries {
			if !entry.IsDir() {
				continue
			}
			userSSH := filepath.Join("/home", entry.Name(), ".ssh")
			if _, err := os.Stat(userSSH); err == nil {
				dest := filepath.Join(c.tempDir, "home", entry.Name(), ".ssh")
				if err := c.safeCopyDir(ctx, userSSH, dest, fmt.Sprintf("%s SSH keys", entry.Name())); err != nil {
					c.logger.Debug("Failed to copy SSH keys for user %s: %v", entry.Name(), err)
				}
			}
		}
	}

	return nil
}

func (c *Collector) collectScriptRepository(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	base := strings.TrimSpace(c.config.ScriptRepositoryPath)
	if base == "" {
		return nil
	}

	info, err := os.Stat(base)
	if err != nil || !info.IsDir() {
		return nil
	}

	target := filepath.Join(c.tempDir, "script-repository", filepath.Base(base))
	c.logger.Debug("Collecting script repository from %s", base)

	return filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == base {
			return nil
		}

		rel, err := filepath.Rel(base, path)
		if err != nil || rel == "." {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 0 {
			if parts[0] == "backup" || parts[0] == "log" {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		dest := filepath.Join(target, rel)
		if d.IsDir() {
			return c.ensureDir(dest)
		}
		return c.safeCopyFile(ctx, path, dest, fmt.Sprintf("script repository file %s", rel))
	})
}

func (c *Collector) collectRootHome(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := os.Stat("/root"); err != nil {
		return nil
	}

	target := filepath.Join(c.tempDir, "root")
	if err := c.ensureDir(target); err != nil {
		return err
	}

	files := []string{
		".bashrc",
		".profile",
		".bash_logout",
		".lesshst",
		".selected_editor",
		".forward",
		".wget-hsts",
		"pkg-list.txt",
		"test-cron.log",
	}
	for _, name := range files {
		src := filepath.Join("/root", name)
		dest := filepath.Join(target, name)
		if err := c.safeCopyFile(ctx, src, dest, fmt.Sprintf("root file %s", name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Failed to copy root file %s: %v", name, err)
		}
	}

	historyPatterns := []string{".bash_history", ".bash_history-*"}
	for _, pattern := range historyPatterns {
		matches, err := filepath.Glob(filepath.Join("/root", pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			name := filepath.Base(match)
			if err := c.safeCopyFile(ctx, match, filepath.Join(target, name), fmt.Sprintf("root history %s", name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				c.logger.Debug("Failed to copy root history %s: %v", match, err)
			}
		}
	}

	dirs := []string{
		".config",
		".ssh",
		"go",
		"my-worker",
	}
	for _, dir := range dirs {
		src := filepath.Join("/root", dir)
		dest := filepath.Join(target, dir)
		if err := c.safeCopyDir(ctx, src, dest, fmt.Sprintf("root directory %s", dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Failed to copy root directory %s: %v", dir, err)
		}
	}

	return nil
}

func (c *Collector) collectUserHomes(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entries, err := os.ReadDir("/home")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if name == "" {
			continue
		}
		src := filepath.Join("/home", name)
		dest := filepath.Join(c.tempDir, "users", name)

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.IsDir() {
			if err := c.safeCopyDir(ctx, src, dest, fmt.Sprintf("home directory for %s", name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				c.logger.Debug("Failed to copy home for %s: %v", name, err)
			}
			continue
		}

		if err := c.safeCopyFile(ctx, src, dest, fmt.Sprintf("home entry %s", name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Failed to copy home entry %s: %v", name, err)
		}
	}

	return nil
}
