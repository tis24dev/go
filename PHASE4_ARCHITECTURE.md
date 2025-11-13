# Phase 4 - Backup Operations Architecture

## Overview

Phase 4 implementa le operazioni core di backup:
1. **Collection**: Raccolta file di configurazione PVE/PBS e system info
2. **Archive**: Creazione archivi compressi (GIÀ IMPLEMENTATO con miglioramenti)
3. **Verification**: Verifica integrità archivi (GIÀ IMPLEMENTATO)
4. **Storage**: Upload locale/secondary/cloud (NUOVO)

---

## 1. Architettura Moduli

```
internal/backup/
├── collector.go          ✅ GIÀ PRESENTE (con exclude patterns e validation)
├── archiver.go           ✅ GIÀ PRESENTE (con streaming compression)
├── checksum.go           ✅ GIÀ PRESENTE (SHA256 + manifest)
├── collector_pve.go      🆕 NUOVO - Collezione specifica PVE
├── collector_pbs.go      🆕 NUOVO - Collezione specifica PBS
├── collector_system.go   🆕 NUOVO - System info comune
└── stats.go              🆕 NUOVO - Statistiche backup

internal/storage/
├── storage.go            🆕 NUOVO - Interfaccia storage
├── local.go              🆕 NUOVO - Storage locale
├── secondary.go          🆕 NUOVO - Storage secondario
└── cloud.go              🆕 NUOVO - Storage cloud (rclone)

internal/orchestrator/
├── bash.go               ✅ GIÀ PRESENTE
└── backup.go             🆕 NUOVO - Orchestrazione backup completo
```

---

## 2. Stato Attuale

### 2.1 Già Implementato (Ottimo!)
- ✅ **Archiver** con streaming compression (tar → pipe → xz/zstd)
- ✅ **Advanced compression controls** (COMPRESSION_LEVEL/MODE/THREADS con range specifici, logging e manifest aggiornati)
- ✅ **CompressionError** per errori specifici
- ✅ **ResolveCompression()** con fallback intelligente
- ✅ **Permissions/Owner/Timestamp** preservation
- ✅ **Symlink** preservation
- ✅ **Comprehensive verification** (compression + tar test)
- ✅ **Checksum & Manifest** generation
- ✅ **Configuration validation** framework
- ✅ **Exit codes granulari**
- ✅ **Exclude patterns**
- ✅ **Safety factor** per disk space
- ✅ **LockDirPath** separato
- ✅ **Directory auto-creation**

### 2.2 Da Implementare
- 🆕 **Collector PVE-specific** (file/comandi PVE)
- 🆕 **Collector PBS-specific** (file/comandi PBS)
- 🆕 **Collector System** (info sistema comune)
- 🆕 **Storage operations** (local/secondary/cloud)
- 🆕 **Backup orchestration** completa
- 🆕 **Statistics tracking** dettagliato

### 2.3 Approfondimento: controlli di compressione
- `configs/backup.env` documenta i range ammessi per ogni algoritmo (gzip/pigz/bzip2 1‑9, xz/lzma 0‑9, zstd 1‑22) e spiega come i mode `fast/standard/maximum/ultra` mappano sui preset legacy.
- `internal/backup/archiver.go` traduce i mode in flag reali (`--extreme`, `--best`, `--ultra`, suffisso `e`) mantenendo lo streaming tar→cmd e loggando tipo/livello/mode/thread utilizzati.
- `internal/orchestrator/bash.go` propaga i nuovi campi verso `BackupStats`, report JSON e manifest, così l’utente può verificare dal CLI quale preset è stato realmente usato.
- `internal/backup/checksum.go` aggiunge `compression_mode` al manifest `.manifest.json`, garantendo compatibilità con gli strumenti di audit Bash.

---

## 3. Collector Architecture

### 3.1 Collector Base (collector.go) - GIÀ PRESENTE

```go
type Collector struct {
    logger   *logging.Logger
    config   *CollectorConfig
    stats    *CollectionStats
    tempDir  string
    proxType types.ProxmoxType
    dryRun   bool
}

type CollectorConfig struct {
    // PVE specific
    BackupClusterConfig  bool
    BackupCorosyncConfig bool
    BackupPVEFirewall    bool
    BackupVMConfigs      bool
    BackupVZDumpConfig   bool

    // PBS specific
    BackupDatastoreConfig bool
    BackupUserConfig      bool
    BackupACLConfig       bool
    BackupSyncJobs        bool
    BackupVerifyJobs      bool

    // Common
    BackupNetworkConfig bool
    BackupSystemInfo    bool

    // Exclude patterns
    ExcludePatterns []string
}
```

### 3.2 Collector PVE (collector_pve.go) - NUOVO

```go
// Collect PVE-specific configurations
func (c *Collector) CollectPVEConfigs(ctx context.Context) error {
    // Directories
    - /etc/pve/
    - /etc/network/
    - /etc/vzdump.conf
    - /etc/pve/corosync.conf (if clustered)
    - /etc/pve/firewall/
    - /var/lib/pve-cluster/
    - /etc/pve/qemu-server/*.conf (VMs)
    - /etc/pve/lxc/*.conf (Containers)

    // Commands
    - pveversion -v
    - pvecm status (if clustered)
    - pvecm nodes (if clustered)
    - pvenode config
    - pvesh get /version
    - pvesh get /cluster/ha/status
    - pvesh get /nodes/{node}/storage
    - pvesh get /nodes/{node}/disks/list
}
```

### 3.3 Collector PBS (collector_pbs.go) - NUOVO

```go
// Collect PBS-specific configurations
func (c *Collector) CollectPBSConfigs(ctx context.Context) error {
    // Directories
    - /etc/proxmox-backup/
    - /etc/network/

    // Commands
    - proxmox-backup-manager version
    - proxmox-backup-manager datastore list
    - proxmox-backup-manager network list
    - proxmox-backup-manager acl list
    - proxmox-backup-manager user list
    - proxmox-backup-manager sync-job list
    - proxmox-backup-manager verify-job list
    - proxmox-backup-manager prune-job list
    - proxmox-backup-manager traffic-control list

    // Files
    - /etc/proxmox-backup/datastore.cfg
    - /etc/proxmox-backup/user.cfg
    - /etc/proxmox-backup/acl.cfg
    - /etc/proxmox-backup/sync.cfg
    - /etc/proxmox-backup/verification.cfg
    - /etc/proxmox-backup/tape.cfg (if exists)
    - /etc/proxmox-backup/node.cfg
}
```

### 3.4 Collector System (collector_system.go) - NUOVO

```go
// Collect common system information
func (c *Collector) CollectSystemInfo(ctx context.Context) error {
    // Commands
    - hostname -f
    - uname -a
    - cat /etc/os-release
    - ip addr show
    - ip route show
    - df -h
    - mount
    - dpkg -l (installed packages)
    - systemctl list-units --type=service --state=running
    - pvesm status (PVE only)
    - zpool status (if ZFS)
    - zfs list (if ZFS)

    // Files
    - /etc/hosts
    - /etc/hostname
    - /etc/resolv.conf
    - /etc/timezone
    - /etc/network/interfaces
    - /etc/crontab (if BACKUP_CRON_JOBS=true)
    - /etc/cron.d/*
    - /var/spool/cron/crontabs/*
    - /etc/apt/sources.list (if BACKUP_APT_SOURCES=true)
    - /etc/apt/sources.list.d/*
}
```

---

## 4. Storage Architecture

### 4.1 Storage Interface

```go
type Storage interface {
    // Upload uploads a file to storage
    Upload(ctx context.Context, localPath, remotePath string) error

    // List lists files in storage
    List(ctx context.Context, remotePath string) ([]StorageItem, error)

    // Delete deletes a file from storage
    Delete(ctx context.Context, remotePath string) error

    // Size returns available space
    Size(ctx context.Context) (*StorageInfo, error)

    // Type returns storage type
    Type() types.StorageType
}

type StorageItem struct {
    Path     string
    Size     int64
    ModTime  time.Time
    IsDir    bool
}

type StorageInfo struct {
    Total     int64
    Used      int64
    Available int64
}
```

### 4.2 Local Storage

```go
type LocalStorage struct {
    logger  *logging.Logger
    basePath string
}

func (s *LocalStorage) Upload(ctx context.Context, localPath, remotePath string) error {
    // Simple file copy to local backup directory
    // Already exists in current implementation
}
```

### 4.3 Secondary Storage

```go
type SecondaryStorage struct {
    logger  *logging.Logger
    basePath string
    method  string // "copy" (rsync deprecated; replaced by native Go atomic copy)
}

func (s *SecondaryStorage) Upload(ctx context.Context, localPath, remotePath string) error {
    // Rsync to secondary location (NFS mount, etc.)
}
```

### 4.4 Cloud Storage

```go
type CloudStorage struct {
    logger   *logging.Logger
    rclone   *RcloneClient
    remote   string
    basePath string
}

func (s *CloudStorage) Upload(ctx context.Context, localPath, remotePath string) error {
    // Upload via rclone
    // Support multiple cloud providers (B2, S3, etc.)
}
```

---

## 5. Backup Orchestration

### 5.1 Backup Orchestrator (orchestrator/backup.go) - NUOVO

```go
type BackupOrchestrator struct {
    logger    *logging.Logger
    config    *BackupConfig
    collector *backup.Collector
    archiver  *backup.Archiver
    storages  []storage.Storage
}

func (o *BackupOrchestrator) RunBackup(ctx context.Context) (*BackupResult, error) {
    // 1. Pre-checks
    if err := o.runPreChecks(ctx); err != nil {
        return nil, err
    }

    // 2. Collection
    stats := &backup.CollectionStats{}
    if err := o.collector.CollectAll(ctx); err != nil {
        return nil, &BackupError{
            Phase: "collection",
            Err:   err,
            Code:  types.ExitCollectionError,
        }
    }

    // 3. Archive creation
    archivePath := o.generateArchivePath()
    if err := o.archiver.CreateArchive(ctx, o.collector.TempDir(), archivePath); err != nil {
        return nil, &BackupError{
            Phase: "archive",
            Err:   err,
            Code:  types.ExitArchiveError,
        }
    }

    // 4. Verification
    if err := o.archiver.VerifyArchive(ctx, archivePath); err != nil {
        return nil, &BackupError{
            Phase: "verification",
            Err:   err,
            Code:  types.ExitVerificationError,
        }
    }

    // 5. Checksum & Manifest
    checksum, err := backup.GenerateChecksum(ctx, o.logger, archivePath)
    if err != nil {
        o.logger.Warning("Failed to generate checksum: %v", err)
    }

    manifest := &backup.Manifest{
        ArchivePath:      archivePath,
        ArchiveSize:      o.getArchiveSize(archivePath),
        SHA256:           checksum,
        CreatedAt:        time.Now(),
        CompressionType:  string(o.archiver.EffectiveCompression()),
        CompressionLevel: o.archiver.CompressionLevel(),
        ProxmoxType:      string(o.config.ProxmoxType),
        Hostname:         o.config.Hostname,
    }

    manifestPath := archivePath + ".manifest.json"
    if err := backup.CreateManifest(ctx, o.logger, manifest, manifestPath); err != nil {
        o.logger.Warning("Failed to create manifest: %v", err)
    }

    // 6. Storage operations
    if err := o.uploadToStorages(ctx, archivePath); err != nil {
        o.logger.Warning("Storage uploads had errors: %v", err)
    }

    // 7. Retention policies
    if err := o.applyRetention(ctx); err != nil {
        o.logger.Warning("Retention policies had errors: %v", err)
    }

    return &BackupResult{
        ArchivePath:   archivePath,
        ManifestPath:  manifestPath,
        Checksum:      checksum,
        Stats:         stats,
        Duration:      time.Since(startTime),
    }, nil
}
```

---

## 6. Statistics Tracking

### 6.1 Collection Stats (stats.go) - NUOVO

```go
type CollectionStats struct {
    // Counters
    FilesProcessed   int64
    FilesFailed      int64
    DirsCreated      int64
    BytesCollected   int64

    // Timing
    CollectionStart  time.Time
    CollectionEnd    time.Time
    ArchiveStart     time.Time
    ArchiveEnd       time.Time
    VerifyStart      time.Time
    VerifyEnd        time.Time

    // Details
    WarningCount     int
    ErrorCount       int
    WarningDetails   []string
    ErrorDetails     []string
}

func (s *CollectionStats) Duration(phase string) time.Duration {
    switch phase {
    case "collection":
        return s.CollectionEnd.Sub(s.CollectionStart)
    case "archive":
        return s.ArchiveEnd.Sub(s.ArchiveStart)
    case "verify":
        return s.VerifyEnd.Sub(s.VerifyStart)
    default:
        return 0
    }
}

func (s *CollectionStats) TotalDuration() time.Duration {
    if s.VerifyEnd.IsZero() {
        return 0
    }
    return s.VerifyEnd.Sub(s.CollectionStart)
}
```

---

## 7. Error Handling Strategy

### 7.1 Error Types

```go
// BackupError - già implementato in orchestrator/bash.go
type BackupError struct {
    Phase string          // "collection", "archive", "verification", "storage"
    Err   error
    Code  types.ExitCode
}

// CompressionError - già implementato in backup/archiver.go
type CompressionError struct {
    Algorithm string
    Err       error
}

// CollectionError - nuovo
type CollectionError struct {
    Path  string
    Op    string  // "copy", "mkdir", "command"
    Err   error
}

// StorageError - nuovo
type StorageError struct {
    Storage string  // "local", "secondary", "cloud"
    Op      string  // "upload", "delete", "list"
    Err     error
}
```

### 7.2 Error Context

Ogni errore deve includere:
- **Fase**: Collection, Archive, Verify, Storage
- **Operazione**: Copy, Mkdir, Command, Upload
- **Path/File**: Quale file/directory
- **Exit code specifico**: 9, 10, 11, 8, 12

---

## 8. Context Awareness

### 8.1 Cancellation Handling

```go
// Ogni operazione lunga deve:
1. Accettare context.Context
2. Check periodico di ctx.Done()
3. Cleanup su cancellazione
4. Propagare ctx.Err()

Example:
func (c *Collector) CollectAll(ctx context.Context) error {
    for _, item := range c.items {
        // Check cancellation
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        // Do work
        if err := c.collectItem(ctx, item); err != nil {
            return err
        }
    }
    return nil
}
```

### 8.2 Timeout Management

```go
// Timeout su operazioni esterne
func (c *Collector) runCommand(ctx context.Context, cmd string, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    execCmd := exec.CommandContext(ctx, "bash", "-c", cmd)
    return execCmd.Run()
}
```

---

## 9. Testing Strategy

### 9.1 Unit Tests
- Collector methods (mock filesystem)
- Archiver methods (già testato)
- Storage operations (mock storage)
- Error handling (inject errors)

### 9.2 Integration Tests
- End-to-end backup flow
- PVE collection (mock pve commands)
- PBS collection (mock pbs commands)
- Storage upload (mock rclone)

### 9.3 Test Fixtures
```
test/fixtures/
├── pve/
│   ├── etc/pve/
│   ├── etc/network/
│   └── commands/
│       ├── pveversion.txt
│       └── pvecm-status.txt
├── pbs/
│   ├── etc/proxmox-backup/
│   └── commands/
│       ├── version.txt
│       └── datastore-list.txt
└── system/
    ├── etc/hosts
    ├── etc/hostname
    └── commands/
        ├── hostname.txt
        └── uname.txt
```

---

## 10. Implementation Plan

### Phase 4.1: Collection (Week 1)
- ✅ Day 1-2: Collector base (già fatto, miglioramenti)
- 🆕 Day 3-4: Collector PVE implementation
- 🆕 Day 5: Collector PBS implementation
- 🆕 Day 6: Collector System implementation
- 🆕 Day 7: Unit tests + Integration tests

### Phase 4.2: Storage (Week 2)
- 🆕 Day 1-2: Storage interface + Local storage
- 🆕 Day 3-4: Secondary storage (rsync — deprecated; now native Go atomic copy)
- 🆕 Day 5-6: Cloud storage (rclone)
- 🆕 Day 7: Unit tests + Integration tests

### Phase 4.3: Orchestration (Week 3)
- 🆕 Day 1-2: Backup orchestrator
- 🆕 Day 3-4: Statistics tracking
- 🆕 Day 5: Retention policies
- 🆕 Day 6: End-to-end testing
- 🆕 Day 7: Documentation

---

## 11. Success Criteria

### 11.1 Functional
- ✅ Backup PVE completo (file + comandi)
- ✅ Backup PBS completo (file + comandi)
- ✅ System info collection
- ✅ Archive creation con tutti i compression types
- ✅ Verification multi-livello
- ✅ Upload a local/secondary/cloud storage
- ✅ Retention policies
- ✅ Checksum & Manifest generation

### 11.2 Non-Functional
- ✅ Performance: ≥ script Bash (grazie a streaming + concurrency)
- ✅ Robustezza: Gestione tutti gli edge case
- ✅ Testabilità: >80% code coverage
- ✅ Maintainability: Codice ben strutturato e documentato
- ✅ Compatibility: Stesso comportamento dello script Bash

---

## 12. Conclusione

Phase 4 costruisce su fondamenta già solide (archiver, verification, checksum) e aggiunge:

1. **Collection completa** per PVE/PBS/System
2. **Storage operations** per tutti i backend
3. **Orchestration robusta** con error handling granulare
4. **Statistics dettagliate** per monitoring

L'implementazione sarà:
- **Robusta**: Gestione completa di errori e edge case
- **Performante**: Streaming + concurrency dove possibile
- **Testabile**: Unit + Integration tests completi
- **Compatibile**: Stesso comportamento dello script Bash

**Timeline**: 3 settimane (Collection + Storage + Orchestration)

---

**Data**: 2025-11-06
**Autore**: tis24dev
**Status**: Architecture document per Phase 4 implementation
