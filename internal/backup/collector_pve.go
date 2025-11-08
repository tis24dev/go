package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type pveStorageEntry struct {
	Name    string
	Path    string
	Type    string
	Content string
}

type pveRuntimeInfo struct {
	Nodes    []string
	Storages []pveStorageEntry
}

// CollectPVEConfigs collects Proxmox VE specific configurations
func (c *Collector) CollectPVEConfigs(ctx context.Context) error {
	c.logger.Info("Collecting PVE configurations")

	// Check if we're actually on PVE
	if _, err := os.Stat("/etc/pve"); os.IsNotExist(err) {
		return fmt.Errorf("not a PVE system: /etc/pve not found")
	}

	clustered := false
	if isClustered, err := c.isClusteredPVE(ctx); err != nil {
		if ctx.Err() != nil {
			return err
		}
		c.logger.Debug("Cluster detection failed, assuming standalone node: %v", err)
	} else {
		clustered = isClustered
	}

	// Collect PVE directories
	if err := c.collectPVEDirectories(ctx, clustered); err != nil {
		return fmt.Errorf("failed to collect PVE directories: %w", err)
	}

	// Collect PVE commands output
	runtimeInfo, err := c.collectPVECommands(ctx, clustered)
	if err != nil {
		return fmt.Errorf("failed to collect PVE commands: %w", err)
	}

	// Collect VM/CT configurations
	if c.config.BackupVMConfigs {
		if err := c.collectVMConfigs(ctx); err != nil {
			c.logger.Warning("Failed to collect VM configs: %v", err)
			// Non-fatal, continue
		}
	}

	if c.config.BackupPVEJobs {
		if err := c.collectPVEJobs(ctx, runtimeInfo.Nodes); err != nil {
			c.logger.Warning("Failed to collect PVE job information: %v", err)
		}
	}

	if c.config.BackupPVESchedules {
		if err := c.collectPVESchedules(ctx); err != nil {
			c.logger.Warning("Failed to collect PVE schedules: %v", err)
		}
	}

	if c.config.BackupPVEReplication {
		if err := c.collectPVEReplication(ctx, runtimeInfo.Nodes); err != nil {
			c.logger.Warning("Failed to collect PVE replication info: %v", err)
		}
	}

	if c.config.BackupPVEBackupFiles {
		if err := c.collectPVEStorageMetadata(ctx, runtimeInfo.Storages); err != nil {
			c.logger.Warning("Failed to collect PVE datastore metadata: %v", err)
		}
	}

	if c.config.BackupCephConfig {
		if err := c.collectPVECephInfo(ctx); err != nil {
			c.logger.Warning("Failed to collect Ceph information: %v", err)
		}
	}

	c.logger.Info("PVE configuration collection completed")
	return nil
}

// collectPVEDirectories collects PVE-specific directories
func (c *Collector) collectPVEDirectories(ctx context.Context, clustered bool) error {
	// PVE main configuration directory
	if err := c.safeCopyDir(ctx,
		"/etc/pve",
		filepath.Join(c.tempDir, "etc/pve"),
		"PVE configuration"); err != nil {
		return err
	}

	// Cluster configuration (if clustered)
	if c.config.BackupClusterConfig && clustered {
		if err := c.safeCopyFile(ctx,
			"/etc/pve/corosync.conf",
			filepath.Join(c.tempDir, "etc/pve/corosync.conf"),
			"Corosync configuration"); err != nil {
			c.logger.Warning("Failed to copy corosync.conf: %v", err)
		}

		// Cluster directory
		if err := c.safeCopyDir(ctx,
			"/var/lib/pve-cluster",
			filepath.Join(c.tempDir, "var/lib/pve-cluster"),
			"PVE cluster data"); err != nil {
			c.logger.Warning("Failed to copy cluster data: %v", err)
		}
	}

	// Firewall configuration
	if c.config.BackupPVEFirewall {
		if err := c.safeCopyDir(ctx,
			"/etc/pve/firewall",
			filepath.Join(c.tempDir, "etc/pve/firewall"),
			"PVE firewall"); err != nil {
			c.logger.Debug("No firewall configuration found")
		}
	}

	// VZDump configuration
	if c.config.BackupVZDumpConfig {
		if err := c.safeCopyFile(ctx,
			"/etc/vzdump.conf",
			filepath.Join(c.tempDir, "etc/vzdump.conf"),
			"VZDump configuration"); err != nil {
			c.logger.Debug("No vzdump.conf found")
		}
	}

	return nil
}

// collectPVECommands collects output from PVE commands and returns runtime info
func (c *Collector) collectPVECommands(ctx context.Context, clustered bool) (*pveRuntimeInfo, error) {
	commandsDir := filepath.Join(c.tempDir, "commands")
	if err := c.ensureDir(commandsDir); err != nil {
		return nil, fmt.Errorf("failed to create commands directory: %w", err)
	}

	// PVE version (CRITICAL)
	if err := c.safeCmdOutput(ctx,
		"pveversion -v",
		filepath.Join(commandsDir, "pveversion.txt"),
		"PVE version",
		true); err != nil {
		return nil, fmt.Errorf("failed to get PVE version (critical): %w", err)
	}

	// Node configuration
	c.safeCmdOutput(ctx,
		"pvenode config",
		filepath.Join(commandsDir, "node_config.txt"),
		"Node configuration",
		false)

	// API version
	c.safeCmdOutput(ctx,
		"pvesh get /version --output-format=json",
		filepath.Join(commandsDir, "api_version.json"),
		"API version",
		false)

	info := &pveRuntimeInfo{
		Nodes:    make([]string, 0),
		Storages: make([]pveStorageEntry, 0),
	}

	// Collect node list (used for subsequent per-node commands)
	if nodeData, err := c.captureCommandOutput(ctx,
		"pvesh get /nodes --output-format=json",
		filepath.Join(commandsDir, "nodes_status.json"),
		"node status",
		false); err != nil {
		return nil, fmt.Errorf("failed to get node status: %w", err)
	} else if len(nodeData) > 0 {
		var nodes []struct {
			Node string `json:"node"`
		}
		if err := json.Unmarshal(nodeData, &nodes); err != nil {
			c.logger.Debug("Failed to parse node status JSON: %v", err)
		} else {
			for _, n := range nodes {
				if trimmed := strings.TrimSpace(n.Node); trimmed != "" {
					info.Nodes = append(info.Nodes, trimmed)
				}
			}
		}
	}

	// Collect ACL information if enabled
	if c.config.BackupPVEACL {
		c.safeCmdOutput(ctx,
			"pveum user list --output-format=json",
			filepath.Join(commandsDir, "pve_users.json"),
			"PVE users",
			false)

		c.safeCmdOutput(ctx,
			"pveum group list --output-format=json",
			filepath.Join(commandsDir, "pve_groups.json"),
			"PVE groups",
			false)

		c.safeCmdOutput(ctx,
			"pveum role list --output-format=json",
			filepath.Join(commandsDir, "pve_roles.json"),
			"PVE roles",
			false)
	}

	// Cluster commands (if clustered)
	if clustered {
		c.safeCmdOutput(ctx,
			"pvecm status",
			filepath.Join(commandsDir, "cluster_status.txt"),
			"Cluster status",
			false)

		c.safeCmdOutput(ctx,
			"pvecm nodes",
			filepath.Join(commandsDir, "cluster_nodes.txt"),
			"Cluster nodes",
			false)

		// HA status
		c.safeCmdOutput(ctx,
			"pvesh get /cluster/ha/status --output-format=json",
			filepath.Join(commandsDir, "ha_status.json"),
			"HA status",
			false)
	}

	// Storage status
	hostname, _ := os.Hostname()
	nodeName := shortHostname(hostname)
	if nodeName == "" {
		nodeName = hostname
	}
	c.safeCmdOutput(ctx,
		fmt.Sprintf("pvesh get /nodes/%s/storage --output-format=json", nodeName),
		filepath.Join(commandsDir, "storage_status.json"),
		"Storage status",
		false)

	// Disk list
	c.safeCmdOutput(ctx,
		fmt.Sprintf("pvesh get /nodes/%s/disks/list --output-format=json", nodeName),
		filepath.Join(commandsDir, "disks_list.json"),
		"Disks list",
		false)

	// Storage manager status
	c.safeCmdOutput(ctx,
		"pvesm status",
		filepath.Join(commandsDir, "pvesm_status.txt"),
		"Storage manager status",
		false)

	if storageData, err := c.captureCommandOutput(ctx,
		"pvesm status --noborder --output-format=json",
		filepath.Join(commandsDir, "pvesm_status.json"),
		"PVE storage status (json)",
		false); err != nil {
		return nil, fmt.Errorf("failed to query storage status: %w", err)
	} else if len(storageData) > 0 {
		var storages []struct {
			Storage string `json:"storage"`
			Name    string `json:"name"`
			Path    string `json:"path"`
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(storageData, &storages); err != nil {
			c.logger.Debug("Failed to parse storage status JSON: %v", err)
		} else {
			seen := make(map[string]struct{})
			for _, s := range storages {
				name := strings.TrimSpace(s.Storage)
				if name == "" {
					name = strings.TrimSpace(s.Name)
				}
				if name == "" {
					continue
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				info.Storages = append(info.Storages, pveStorageEntry{
					Name:    name,
					Path:    strings.TrimSpace(s.Path),
					Type:    strings.TrimSpace(s.Type),
					Content: strings.TrimSpace(s.Content),
				})
			}
			sort.Slice(info.Storages, func(i, j int) bool {
				return info.Storages[i].Name < info.Storages[j].Name
			})
		}
	}

	// Ensure we have at least one node reference
	if len(info.Nodes) == 0 {
		if short := shortHostname(hostname); short != "" {
			info.Nodes = append(info.Nodes, short)
		} else if hostname != "" {
			info.Nodes = append(info.Nodes, hostname)
		} else {
			info.Nodes = append(info.Nodes, "localhost")
		}
	} else {
		sort.Strings(info.Nodes)
	}

	return info, nil
}

// collectVMConfigs collects VM and Container configurations
func (c *Collector) collectVMConfigs(ctx context.Context) error {
	// QEMU VMs
	vmConfigDir := "/etc/pve/qemu-server"
	if _, err := os.Stat(vmConfigDir); err == nil {
		if err := c.safeCopyDir(ctx,
			vmConfigDir,
			filepath.Join(c.tempDir, "etc/pve/qemu-server"),
			"VM configurations"); err != nil {
			return fmt.Errorf("failed to copy VM configs: %w", err)
		}
	}

	// LXC Containers
	lxcConfigDir := "/etc/pve/lxc"
	if _, err := os.Stat(lxcConfigDir); err == nil {
		if err := c.safeCopyDir(ctx,
			lxcConfigDir,
			filepath.Join(c.tempDir, "etc/pve/lxc"),
			"Container configurations"); err != nil {
			return fmt.Errorf("failed to copy container configs: %w", err)
		}
	}

	// Collect VMs/CTs list
	commandsDir := filepath.Join(c.tempDir, "commands")
	hostname, _ := os.Hostname()
	nodeName := shortHostname(hostname)
	if nodeName == "" {
		nodeName = hostname
	}

	// QEMU VMs list
	c.safeCmdOutput(ctx,
		fmt.Sprintf("pvesh get /nodes/%s/qemu --output-format=json", nodeName),
		filepath.Join(commandsDir, "qemu_vms.json"),
		"QEMU VMs list",
		false)

	// LXC Containers list
	c.safeCmdOutput(ctx,
		fmt.Sprintf("pvesh get /nodes/%s/lxc --output-format=json", nodeName),
		filepath.Join(commandsDir, "lxc_containers.json"),
		"LXC containers list",
		false)

	return nil
}

func (c *Collector) collectPVEJobs(ctx context.Context, nodes []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	jobsDir := filepath.Join(c.tempDir, "var/lib/pve-cluster/info/jobs")
	if err := c.ensureDir(jobsDir); err != nil {
		return fmt.Errorf("failed to create jobs directory: %w", err)
	}

	if _, err := c.captureCommandOutput(ctx,
		"pvesh get /cluster/backup --output-format=json",
		filepath.Join(jobsDir, "backup_jobs.json"),
		"backup jobs",
		false); err != nil {
		return err
	}

	seen := make(map[string]struct{})
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		if node == "" {
			continue
		}
		if _, ok := seen[node]; ok {
			continue
		}
		seen[node] = struct{}{}
		outputPath := filepath.Join(jobsDir, fmt.Sprintf("%s_backup_history.json", node))
		c.captureCommandOutput(ctx,
			fmt.Sprintf("pvesh get /nodes/%s/tasks --output-format=json --typefilter=vzdump", node),
			outputPath,
			fmt.Sprintf("%s backup history", node),
			false)
	}

	// Copy vzdump cron schedule if present
	if err := c.safeCopyFile(ctx,
		"/etc/cron.d/vzdump",
		filepath.Join(c.tempDir, "etc/cron.d/vzdump"),
		"VZDump cron schedule"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (c *Collector) collectPVESchedules(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	schedulesDir := filepath.Join(c.tempDir, "var/lib/pve-cluster/info/schedules")
	if err := c.ensureDir(schedulesDir); err != nil {
		return fmt.Errorf("failed to create schedules directory: %w", err)
	}

	c.captureCommandOutput(ctx,
		"crontab -l",
		filepath.Join(schedulesDir, "root_crontab.txt"),
		"root crontab",
		false)

	c.captureCommandOutput(ctx,
		"systemctl list-timers --all --no-pager",
		filepath.Join(schedulesDir, "systemd_timers.txt"),
		"systemd timers",
		false)

	cronDir := "/etc/cron.d"
	if entries, err := os.ReadDir(cronDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			lower := strings.ToLower(name)
			if strings.Contains(lower, "pve") || strings.Contains(lower, "proxmox") || strings.Contains(lower, "vzdump") {
				src := filepath.Join(cronDir, name)
				dest := filepath.Join(c.tempDir, "etc/cron.d", name)
				if err := c.safeCopyFile(ctx, src, dest, fmt.Sprintf("cron job %s", name)); err != nil {
					c.logger.Debug("Failed to copy cron job %s: %v", name, err)
				}
			}
		}
	}

	return nil
}

func (c *Collector) collectPVEReplication(ctx context.Context, nodes []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	repDir := filepath.Join(c.tempDir, "var/lib/pve-cluster/info/replication")
	if err := c.ensureDir(repDir); err != nil {
		return fmt.Errorf("failed to create replication directory: %w", err)
	}

	if _, err := c.captureCommandOutput(ctx,
		"pvesh get /cluster/replication --output-format=json",
		filepath.Join(repDir, "replication_jobs.json"),
		"replication jobs",
		false); err != nil {
		return err
	}

	seen := make(map[string]struct{})
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		if node == "" {
			continue
		}
		if _, ok := seen[node]; ok {
			continue
		}
		seen[node] = struct{}{}
		outputPath := filepath.Join(repDir, fmt.Sprintf("%s_replication_status.json", node))
		c.captureCommandOutput(ctx,
			fmt.Sprintf("pvesh get /nodes/%s/replication --output-format=json", node),
			outputPath,
			fmt.Sprintf("%s replication status", node),
			false)
	}

	return nil
}

func (c *Collector) collectPVEStorageMetadata(ctx context.Context, storages []pveStorageEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(storages) == 0 {
		c.logger.Debug("No PVE storage entries detected, skipping datastore metadata")
		return nil
	}

	baseDir := filepath.Join(c.tempDir, "var/lib/pve-cluster/info/datastores")
	if err := c.ensureDir(baseDir); err != nil {
		return fmt.Errorf("failed to create datastore metadata directory: %w", err)
	}

	var summary strings.Builder
	summary.WriteString("# PVE datastores detected on ")
	summary.WriteString(time.Now().Format(time.RFC3339))
	summary.WriteString("\n# Format: NAME|PATH|TYPE|CONTENT\n\n")

	processed := 0
	for _, storage := range storages {
		if storage.Path == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if stat, err := os.Stat(storage.Path); err != nil || !stat.IsDir() {
			c.logger.Debug("Skipping datastore %s (path not accessible: %s)", storage.Name, storage.Path)
			continue
		}

		processed++
		summary.WriteString(fmt.Sprintf("%s|%s|%s|%s\n",
			storage.Name,
			storage.Path,
			storage.Type,
			storage.Content))

		metaDir := filepath.Join(baseDir, storage.Name)
		if err := c.ensureDir(metaDir); err != nil {
			c.logger.Warning("Failed to create metadata directory for %s: %v", storage.Name, err)
			continue
		}

		meta := struct {
			Name              string        `json:"name"`
			Path              string        `json:"path"`
			Type              string        `json:"type"`
			Content           string        `json:"content,omitempty"`
			ScannedAt         time.Time     `json:"scanned_at"`
			SampleDirectories []string      `json:"sample_directories,omitempty"`
			DiskUsage         string        `json:"disk_usage,omitempty"`
			SampleFiles       []FileSummary `json:"sample_files,omitempty"`
		}{
			Name:      storage.Name,
			Path:      storage.Path,
			Type:      storage.Type,
			Content:   storage.Content,
			ScannedAt: time.Now(),
		}

		if dirs, err := c.sampleDirectories(ctx, storage.Path, 2, 20); err == nil && len(dirs) > 0 {
			meta.SampleDirectories = dirs
		}

		usageData, _ := c.captureCommandOutput(ctx,
			fmt.Sprintf("du -sh %s", storage.Path),
			filepath.Join(metaDir, "disk_usage.txt"),
			fmt.Sprintf("disk usage for %s", storage.Name),
			false)
		if len(usageData) > 0 {
			meta.DiskUsage = strings.TrimSpace(string(usageData))
		}

		patterns := []string{
			"*.vma", "*.vma.gz", "*.vma.lz4", "*.vma.zst",
			"*.tar", "*.tar.gz", "*.tar.lz4", "*.tar.zst",
			"*.log", "*.notes",
		}

		if files, err := c.sampleFiles(ctx, storage.Path, patterns, 3, 100); err == nil && len(files) > 0 {
			meta.SampleFiles = files
		}

		metaBytes, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal metadata for %s: %w", storage.Name, err)
		}

		if err := c.writeReportFile(filepath.Join(metaDir, "metadata.json"), metaBytes); err != nil {
			return err
		}
	}

	if processed > 0 {
		summary.WriteString(fmt.Sprintf("\n# Total datastores processed: %d\n", processed))
		if err := c.writeReportFile(filepath.Join(baseDir, "detected_datastores.txt"), []byte(summary.String())); err != nil {
			return err
		}
	}

	return nil
}

func (c *Collector) collectPVECephInfo(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := exec.LookPath("ceph"); err != nil {
		c.logger.Debug("Ceph CLI not available, skipping Ceph information collection")
		return nil
	}

	cephDir := filepath.Join(c.tempDir, "var/lib/pve-cluster/info/ceph")
	if err := c.ensureDir(cephDir); err != nil {
		return fmt.Errorf("failed to create ceph directory: %w", err)
	}

	if _, err := os.Stat("/etc/ceph"); err == nil {
		if err := c.safeCopyDir(ctx,
			"/etc/ceph",
			filepath.Join(c.tempDir, "etc/ceph"),
			"Ceph configuration"); err != nil {
			c.logger.Debug("Failed to copy Ceph configuration: %v", err)
		}
	}

	commands := []struct {
		cmd  string
		file string
		desc string
	}{
		{"ceph -s", "ceph_status.txt", "Ceph status"},
		{"ceph osd df", "ceph_osd_df.txt", "Ceph OSD DF"},
		{"ceph osd tree", "ceph_osd_tree.txt", "Ceph OSD tree"},
		{"ceph mon stat", "ceph_mon_stat.txt", "Ceph mon stat"},
		{"ceph pg stat", "ceph_pg_stat.txt", "Ceph PG stat"},
		{"ceph health detail", "ceph_health.txt", "Ceph health"},
	}

	for _, command := range commands {
		c.captureCommandOutput(ctx,
			command.cmd,
			filepath.Join(cephDir, command.file),
			command.desc,
			false)
	}

	return nil
}

// isClusteredPVE checks if this is a clustered PVE system
func (c *Collector) isClusteredPVE(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if _, err := exec.LookPath("pvecm"); err != nil {
		return false, nil
	}

	cmd := exec.CommandContext(ctx, "pvecm", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("pvecm status failed: %w", err)
	}

	return strings.Contains(string(output), "Cluster information"), nil
}

func shortHostname(host string) string {
	if idx := strings.Index(host, "."); idx > 0 {
		return host[:idx]
	}
	return host
}
