# Proxmox Backup - Go Version

> **Repository:** [github.com/tis24dev/proxmox-backup](https://github.com/tis24dev/proxmox-backup)
>
> **Status:** Phase 0 Completed - Initial Setup
>
> **Version:** 0.1.0-dev

---

## Directory Structure

This is the **Go development directory**, completely separate from the original bash system.

```
/opt/
├── proxmox-backup/          # ← ORIGINAL: Bash production (UNTOUCHED)
│   ├── script/
│   ├── lib/
│   ├── env/
│   └── ...
│
└── proxmox-backup-go/       # ← NEW: Go development (THIS DIRECTORY)
    ├── cmd/
    ├── internal/
    ├── pkg/
    ├── test/
    ├── reference/           # Symlinks to original bash files
    ├── build/
    └── ...
```

---

## Quick Start

```bash
# Enter the Go project directory
cd /opt/proxmox-backup-go

# Build the project
make build

# Run the binary
./build/proxmox-backup

# See all available commands
make help
```

---

## Completed Tasks (Phase 0)

- ✅ Go 1.25+ installed
- ✅ Directory `/opt/proxmox-backup-go/` created
- ✅ Go module initialized: `github.com/tis24dev/proxmox-backup`
- ✅ Directory structure created (cmd/, internal/, pkg/, test/)
- ✅ Makefile created and tested
- ✅ .gitignore created
- ✅ main.go minimal created
- ✅ Symlinks to original bash files created (reference/)
- ✅ Git repository initialized
- ✅ First commit completed
- ✅ Build verified: `make build` works
- ✅ Binary runs successfully

---

## Reference to Original Bash System

The Go project can access the original bash system files through symlinks:

- `reference/env/` → `/opt/proxmox-backup/env/`
- `reference/script/` → `/opt/proxmox-backup/script/`
- `reference/lib/` → `/opt/proxmox-backup/lib/`

This allows:
- Reading the original configuration files
- Referencing the bash scripts for comparison
- Testing side-by-side without duplicating data

---

## Next Steps (Phase 1)

Read the comprehensive migration plan:
- [MIGRATION_PLAN.md](MIGRATION_PLAN.md) - Full 6-phase migration strategy
- [QUICKSTART.md](QUICKSTART.md) - Quick setup guide
- [README-GO.md](README-GO.md) - Go version overview

Start **Phase 1: Core Infrastructure**:
1. Implement `internal/config` package
2. Implement `internal/logging` package
3. Implement `internal/types` package
4. Implement `pkg/utils` package

---

## Compression Settings (Go pipeline)

| `COMPRESSION_TYPE`           | Livelli ammessi (`COMPRESSION_LEVEL`) | Note sul mode (`COMPRESSION_MODE`) |
|-----------------------------|---------------------------------------|------------------------------------|
| `none`                      | 0                                     | Nessuna compressione |
| `gzip`, `pigz`, `bzip2`     | 1‑9                                   | `maximum/ultra` → livello 9 (`pigz` usa `--best`) |
| `xz`, `lzma`                | 0‑9                                   | `maximum/ultra` aggiungono `--extreme` / suffisso `e` |
| `zstd`                      | 1‑22                                  | >19 forza `--ultra`; `maximum/ultra` mappano a 19/22 |

`COMPRESSION_THREADS=0` lascia auto-threading; valori >0 controllano pigz/xz/zstd. Manifest e stats JSON riportano sempre algoritmo, livello, mode e thread effettivi per garantire parità con lo script Bash.

---

## PXAR, Cloud e Override Percorsi (Go)

### PXAR metadata / sampling
- `PXAR_SCAN_ENABLE` abilita o disabilita completamente la raccolta dei metadata PXAR (default: `true`).
- `PXAR_STOP_ON_CAP`, `PXAR_SCAN_MAX_ROOTS` e `PXAR_ENUM_READDIR_WORKERS` limitano la fan-out traversal per evitare di scandire milioni di entry quando bastano 2K campioni.
- `PXAR_ENUM_BUDGET_MS` imposta un budget massimo (in millisecondi) per l'enumerazione: quando scade interrompe i worker e usa i candidati raccolti fin lì.
- `PXAR_FILE_INCLUDE_PATTERN` / `PXAR_FILE_EXCLUDE_PATTERN` permettono di definire i pattern da includere o escludere (lista separata da spazi/virgole). Senza override la Go pipeline usa automaticamente `*.pxar`, `catalog.pxar*`, oltre ai preset per PVE (`*.vma`, `*.tar.*`, `*.log`, ecc.).

### Override percorsi PVE/PBS
- `PVE_CONFIG_PATH`, `PVE_CLUSTER_PATH`, `COROSYNC_CONFIG_PATH`, `VZDUMP_CONFIG_PATH` puntano ai path reali quando non coincidono con `/etc/pve`, `/var/lib/pve-cluster`, ecc. Il collector Go copia e struttura i file partendo sempre da questi override, quindi puoi lavorare anche su mirror montati altrove.
- `PBS_DATASTORE_PATH` accetta più directory separate da virgola/spazio per forzare la scansione di datastore custom (oltre a quelli rilevati da `proxmox-backup-manager`).

### Cloud upload avanzato
- `CLOUD_REMOTE_PATH` aggiunge un prefisso deterministico all'interno del remote rclone (`remote:prefisso/...`), così puoi isolare i backup Go senza duplicare configurazioni.
- `CLOUD_UPLOAD_MODE` (`sequential` | `parallel`) e `CLOUD_PARALLEL_MAX_JOBS` controllano il worker pool usato per bundle e file associati (`.sha256`, `.metadata`, ecc.).
- `CLOUD_PARALLEL_VERIFICATION` abilita la verifica post-upload anche per i file paralleli (il file principale viene sempre verificato).
- `CLOUD_LOG_PATH` è un riferimento rclone completo (`remote:/logs`). A differenza dei backup (che usano `CLOUD_REMOTE` + `CLOUD_REMOTE_PATH`), qui dichiari direttamente remote e cartella finale per i log.
- Le variabili `MAX_*_BACKUPS` governano anche la rotazione dei log: con al massimo un backup al giorno, log e archivi rimangono sempre allineati.

Tutti i flag qui sopra sono già presenti in `configs/backup.env` (Go) e possono convivere con il file `reference/env/backup.env` originale senza modificarlo.

---

## Log File Management

The Go pipeline automatically manages log files with real-time writing and retention policies:

### Log File Lifecycle
1. **Creation**: Log file is opened at backup start with format `backup-<hostname>-<timestamp>.log`
2. **Real-time Writing**: All log messages are written immediately to disk (O_SYNC flag) and to stdout simultaneously
3. **Closure**: Log file is closed **after** notifications are sent (very late in the process)
4. **Distribution**: After closure, log is copied to secondary and cloud storage (same destinations as backup files)
5. **Rotation**: Old logs are automatically removed based on retention policies

### Configuration Variables

```bash
# Log paths
LOG_PATH=${BASE_DIR}/log    # Primary log directory
SECONDARY_LOG_PATH=         # Secondary log directory (optional, leave empty if not used)
CLOUD_LOG_PATH=             # Cloud log path (optional, leave empty if not used)

# Retention policy (applied to BOTH backups and log files)
MAX_LOCAL_BACKUPS=7         # Maximum number of backups/logs to keep locally
MAX_SECONDARY_BACKUPS=14    # Maximum number of backups/logs on secondary storage
MAX_CLOUD_BACKUPS=30        # Maximum number of backups/logs on cloud storage
```

### Log File Format
- **Filename**: `backup-<hostname>-YYYYMMDD-HHMMSS.log`
- **Content**: Plain text without ANSI color codes
- **Encoding**: UTF-8
- **Permissions**: 0600 (owner read/write only)

### Benefits
- **Crash Safety**: Real-time writing ensures logs are preserved even if the backup process crashes
- **Complete Timeline**: Logs capture everything from initialization to notifications
- **Multi-destination**: Same distribution strategy as backup files (primary, secondary, cloud)
- **Automatic Cleanup**: Old logs are rotated automatically per location

---

## Available Make Commands

```bash
make build         # Build the project
make build-release # Build optimized release binary
make test          # Run tests
make test-coverage # Run tests with coverage report
make lint          # Run linters
make fmt           # Format Go code
make clean         # Remove build artifacts
make run           # Run in development mode
make deps          # Download and tidy dependencies
make help          # Show all available commands
```

---

## Safety

The original bash system at `/opt/proxmox-backup/` is **completely untouched**.

- Production bash scripts: `/opt/proxmox-backup/script/proxmox-backup.sh`
- Development Go binary: `/opt/proxmox-backup-go/build/proxmox-backup`

You can run both versions in parallel for testing and comparison.

---

**Last Updated:** 2025-11-05
**Phase:** 0 - Initial Setup ✅ Completed
