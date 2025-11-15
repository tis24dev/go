# Proxmox Backup - Go Version

> Enterprise-grade backup system for Proxmox VE/PBS, reimplemented in Go.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-In%20Development-yellow.svg)]()

---

## 📖 About

This is the Go reimplementation of the Proxmox Backup system, originally written in Bash (~20K lines). The project aims to provide better performance, maintainability, and type safety while maintaining full compatibility with the existing system.

### Current Status

- ✅ **Bash version**: Production-ready (~20,370 lines)
- 🟨 **Go version**: In development (incremental migration)

See [MIGRATION_PLAN.md](MIGRATION_PLAN.md) for the complete migration roadmap.

---

## ✨ Features

### Current (Bash)
- ✅ Multi-storage support (local, secondary, cloud)
- ✅ Prometheus metrics integration
- ✅ Telegram, Email & Gotify notifications
- ✅ Sophisticated logging with buffering
- ✅ Retention policies management
- ✅ Parallel cloud uploads (rclone)
- ✅ Integrity verification with checksums
- ✅ Security auditing

### Planned (Go)
- 🚀 **Better Performance**: Native binary, faster execution
- 🛡️ **Type Safety**: Catch errors at compile time
- 🧪 **Better Testing**: Comprehensive test suite
- 📦 **Single Binary**: No runtime dependencies
- ⚡ **Concurrency**: Efficient goroutines for parallel operations

### Latest Additions (Phase 4)
- 🎛️ **Advanced Compression Controls**: `COMPRESSION_LEVEL`, `COMPRESSION_MODE` e `COMPRESSION_THREADS` replicano i preset Bash per ogni algoritmo (gzip/pigz/xz/zstd, ecc.), con log e manifest che riportano il valore effettivo usato.
- 📊 **Stats & Manifest Parity**: Il report JSON e i manifest `.manifest.json` includono ora tipo, livello, mode e thread di compressione così da confrontare immediatamente Go vs Bash.
- 📡 **Storage warnings coerenti**: se la copia su Secondary/Cloud fallisce, il riepilogo finale mostra `✗ … operations completed with warnings` (nessuna spunta verde ingannevole) ma il backup locale continua.

---

## 🚀 Quick Start

### Prerequisites

- Go 1.25 or higher
- Proxmox VE or Proxmox Backup Server
- Linux system (tested on Debian/Ubuntu)

### Installation

```bash
# Clone the repository
git clone https://github.com/tis24dev/proxmox-backup.git
cd proxmox-backup

# Initialize Go module (if not already done)
go mod init github.com/tis24dev/proxmox-backup

# Build
make build

# Run
./build/proxmox-backup --help
```

---

## 📁 Project Structure

```
proxmox-backup/
├── cmd/                    # Command-line applications
│   └── proxmox-backup/    # Main application
├── internal/              # Private application code
│   ├── config/           # Configuration management
│   ├── logging/          # Logging system
│   ├── collect/          # Data collection
│   ├── archiver/         # Backup creation & compression
│   ├── storage/          # Storage operations
│   ├── notify/           # Notifications (Telegram, Email)
│   └── metrics/          # Prometheus metrics
├── pkg/                   # Public libraries
│   └── utils/            # Utility functions
├── test/                  # Tests
├── script/                # Original Bash scripts (legacy)
└── MIGRATION_PLAN.md      # Detailed migration plan
```

---

## 🛠️ Development

### Build

```bash
# Development build
make build

# Optimized release build
make build-release

# Run without building
make run
```

### Testing

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint code
make lint

# Format code
make fmt
```

### Dependencies

```bash
# Download dependencies
make deps

# Update go.mod and go.sum
go mod tidy
```

### Debugging

- **Temporary workspaces** – Every run keeps its `/tmp/proxmox-backup-*` directory so you can inspect the collected files and command outputs. The next run automatically deletes the directories recorded in the registry (`/var/run/proxmox-backup/temp-dirs.json` or the temp fallback), so you never have to clean them manually.
- **Actionable CLI warnings** – When a PBS/PVE command fails (e.g. `proxmox-backup-manager datastore status`), the warning now includes the exact command and its stdout/stderr snippet. You no longer see generic “exit status 255” messages without context.

---

## 📚 Documentation

- **[MIGRATION_PLAN.md](MIGRATION_PLAN.md)** - Comprehensive migration guide (Italian)
- **Code Documentation** - Run `godoc -http=:6060` and visit http://localhost:6060

### Compression Settings (Go pipeline)

| Algoritmo (`COMPRESSION_TYPE`) | Livelli supportati (`COMPRESSION_LEVEL`) | Note sul mode (`COMPRESSION_MODE`) |
|--------------------------------|------------------------------------------|------------------------------------|
| `none`                         | 0                                        | Nessuna compressione |
| `gzip`, `pigz`, `bzip2`        | 1‑9                                      | `maximum/ultra` forzano il livello 9; `pigz` usa `--best` |
| `xz`, `lzma`                   | 0‑9                                      | `maximum/ultra` aggiungono `--extreme` / suffisso `e` |
| `zstd`                         | 1‑22                                     | Livelli >19 attivano `--ultra`; `maximum/ultra` mappano a 19/22 |

`COMPRESSION_THREADS` (0=auto) controlla pigz/xz/zstd multi-thread. Il manifest JSON e lo stats report Go riportano sempre type/level/mode effettivi per garantire parità con lo script Bash.

### Key Documents

| Document | Description |
|----------|-------------|
| [MIGRATION_PLAN.md](MIGRATION_PLAN.md) | Complete migration strategy (6 phases) |
| [README-GO.md](README-GO.md) | This file - Go version overview |
| [env/backup.env](env/backup.env) | Configuration file (90+ options) |

---

## 🔄 Migration Strategy

The migration from Bash to Go follows a **6-phase incremental approach**:

### Phase 0: Initial Setup (1 week)
- Initialize Go project structure
- Setup build system (Makefile)
- Git workflow setup

### Phase 1: Core Infrastructure (2-3 weeks)
- Configuration system
- Logging framework
- Error handling
- Basic utilities

### Phase 2: Hybrid Orchestrator (2-3 weeks)
- Go main entry point
- Calls Bash modules (temporary)
- CLI argument parsing

### Phase 3: Environment & Core (2 weeks)
- Proxmox type detection (PVE vs PBS)
- Environment validation
- Directory setup

### Phase 4: Collection & Storage (4-6 weeks)
#### 4.1 Collection (Completed)
- Data collection (PVE, PBS, System)
- Archive creation & compression
- Integrity verification

#### 4.2 Storage Operations (Completed)
- Multi-storage support (local, secondary, cloud)
- Cloud integration (rclone)
- Retention policies
- Filesystem detection
- Bundle creation

### Phase 5: Notifications & Metrics (2-3 weeks)
#### 5.1 Notifications (Completed)
- Telegram notifications (personal/centralized modes)
- Email notifications (relay/sendmail with fallback)
- Gotify push notifications (self-hosted)
- HTML email templates
- Cloud relay parity: worker receives only structured data, HTML/text is kept for the local sendmail fallback
- Quota-aware retry: 429/quota responses skip remaining attempts and pass directly to sendmail fallback
- Security preflight: executable/config permissions, sensitive-directory checks and optional network/process auditing (parity with legacy `security-check.sh`)
- Auto-detection of recipients
- Non-blocking error handling

#### 5.2 Webhook Notifications (Completed – phase added on top of the original plan)
- Multi-endpoint webhook notifier (Discord, Slack, Teams, generic JSON)
- Fine-grained env config: `WEBHOOK_ENABLED`, `WEBHOOK_ENDPOINTS`, per-endpoint URL/method/auth headers
- Structured payload builder reuses the same `NotificationData` used by Telegram/Email to ensure parity
- Cloud relay worker receives data-only payloads (subject, stats, issues); HTML/text templates exist solely for local sendmail fallback
- Extensive debug logging with sensitive values masked by default

#### 5.3 Metrics (Planned)
- Prometheus metrics

**Total Timeline**: 19-26 weeks (4-6 months)

See [MIGRATION_PLAN.md](MIGRATION_PLAN.md) for detailed implementation guide.

---

## ⚙️ Configuration

### Configuration File

Configuration is loaded from `env/backup.env` (backward compatible with Bash version):

```bash
# Proxmox Backup Configuration
BACKUP_ENABLED=true
COMPRESSION_TYPE=xz
COMPRESSION_LEVEL=6
# ... 90+ options available
```

> ℹ️ Se `BASE_DIR` non è definito nel file o nell'ambiente, il binario ne deduce automaticamente il valore dal percorso dell'eseguibile (stesso comportamento dello script Bash).
>
> ➕ Novità Go pipeline:
> - I messaggi generati prima dell’inizializzazione del logger vengono catturati e riversati nel log finale, così non si perdono informazioni early-stage.
> - Se la detection PVE/PBS fallisce viene generato automaticamente un file di debug in `/tmp/proxmox_detection_debug_*.log` con tutti i dettagli utili alla diagnosi.
> - `BACKUP_PATH`, `LOG_PATH` e la directory per il lock file vengono create automaticamente (in dry-run viene solo loggata l’azione), mantenendo il comportamento dello script Bash.
> - 🔐 Il blocco *Security* del `backup.env` consente di abilitare il preflight (`SECURITY_CHECK_ENABLED`, `AUTO_UPDATE_HASHES`, `AUTO_FIX_PERMISSIONS`, `CHECK_NETWORK_SECURITY`, ecc.): la Go pipeline verifica i permessi di binario/config, crea le directory mancanti e, se richiesto, controlla firewall, porte sospette e processi anomali prima di eseguire il backup.
>   - Le liste `SUSPICIOUS_PORTS`, `PORT_WHITELIST`, `SUSPICIOUS_PROCESSES` e `SAFE_BRACKET_PROCESSES` permettono di personalizzare il controllo rete/processi per ridurre i falsi positivi; `AUTO_FIX_PERMISSIONS` corregge automaticamente permessi/owner errati e `CONTINUE_ON_SECURITY_ISSUES` (default `false`) stabilisce se fermare il backup o proseguire nonostante i problemi segnalati.
> - 🎨 Vuoi evidenziare a colpo d'occhio il progresso? `COLORIZE_STEP_LOGS=true` (nuovo flag accanto a `USE_COLOR`) colora di blu tutte le righe "Step N/8: …" quando l'output supporta i colori.
- 🔑 Archive encryption: `ENCRYPT_ARCHIVE=true` usa `filippo.io/age` in streaming per cifrare subito il tar/tar.xz (`*.age`) tramite uno o più recipient AGE (`AGE_RECIPIENT` / `AGE_RECIPIENT_FILE`). I meta restano in chiaro per i pre‑check e il wizard interattivo crea il file dei recipient se mancano.

#### Cifratura archivio (streaming + AGE keypair)

- Pipeline: `tar` → (compressione opzionale) → `age` → file cifrato (`*.tar[.gz|.xz|.zst].age`). Nessun archivio in chiaro viene scritto su disco.
- Preferenza per le chiavi pubbliche AGE: `AGE_RECIPIENT` (inline) e/o `AGE_RECIPIENT_FILE` (uno per riga). Al primo avvio, se `ENCRYPT_ARCHIVE=true` e mancano recipient, il wizard interattivo chiede di:
  - Incollare un recipient pubblico (age1…)
  - Generare una chiave pubblica deterministica partendo da una passphrase personale: il server salva **solo** la chiave pubblica derivata, così i job successivi non richiedono input; la passphrase resta l’unico modo per ricostruire la chiave privata e decifrare i backup
  - Derivare un recipient da una chiave privata AGE (non salvata)
  - Uscire dal setup
- I meta (manifest `.manifest.json`, checksum `.sha256`, alias `.metadata`) restano in chiaro per consentire i pre‑check di ripristino. La checksum viene calcolata direttamente sull’artefatto cifrato.
- Verifica archivio: con `ENCRYPT_ARCHIVE=true` i test approfonditi (tar list/test compressore) vengono omessi per evitare il plaintext; restano i controlli di esistenza/size.
- Sicurezza: la directory `identity/age/` viene creata con permessi `700`/`600`, il preflight blocca l’esecuzione se rileva chiavi private sul server e il log ricorda di custodire offline l’identità AGE necessaria per il restore. Usa `./build/proxmox-backup --newkey` per forzare la rigenerazione interattiva dei recipient (solo shell interattiva; l’esecuzione headless fallisce con un errore esplicativo finché non completi il setup).
- Manifest metadata: each bundle now exposes `proxmox_targets` (full list of collected targets), `proxmox_version` (PVE/PBS version detected on that run), `script_version` (binary version that produced the package) and `encryption_mode` (`none` or `age`). These fields power the decrypt workflow.

#### Decrypt workflow (`--decrypt`)

- Launch `./build/proxmox-backup --decrypt`: the tool loads `backup.env`, re-runs the security checks and builds a menu from the configured paths (`BACKUP_PATH`, `SECONDARY_PATH`, `CLOUD_REMOTE` when it points to a local/mounted path).
- Every bundle (either `.bundle.tar` or the raw trio) is inspected via its manifest and displayed with targets (`ProxmoxTargets`), script version, timestamp and `EncryptionMode`.
- After selecting the backup and the destination directory (default `./restore`), the wizard prompts for the decryption material: either an AGE private key (`AGE-SECRET-KEY-…`) or the deterministic passphrase used during encryption. Typing `0` aborts gracefully.
- Decryption runs in streaming; once the archive is plaintext the tool builds a **new** `<name>.decrypted.bundle.tar` containing **exactly three files**: the tar/tar.xz, its `.sha256`, and the manifest copy (`.metadata`). Temporary files are removed right after the bundle is created.
- No optional branches: every run produces that three-file bundle. Wrong keys simply loop back to the prompt; aborting the workflow is treated as a clean exit.

#### Restore workflow (`--restore`)

- Reuses the entire decrypt UX (path selection, manifest inspection, key prompt) but stages the plaintext archive inside a secure temporary directory instead of asking for a destination bundle.
- Restore destination is always `/` (system root) so every file returns to its original absolute path. Root privileges and an explicit confirmation (`RESTORE`) are required.
- Extracts the tar/tar.xz/tar.zst via `tar` with the appropriate compression flags (`-xzpf`, `-xJpf`, `--use-compress-program=zstd`, etc.) so no intermediate plaintext copies remain on disk.
- Immediately deletes the staged plaintext bundle at the end of the workflow (or when aborted), keeping decrypted data off disk by default.

#### Percorsi personali e blacklist

- La pipeline Go non copia più l’intera home di `root`: vengono salvati solo i file critici (dotfile principali, `.ssh`, `pkg-list.txt`, log di wrangler). Qualsiasi directory personalizzata (es. `/root/my-worker`) va aggiunta esplicitamente.
- Riutilizzi gli stessi blocchi multilinea dello script Bash, uno path per riga:

  ```bash
  CUSTOM_BACKUP_PATHS="
  /root/.config/rclone/rclone.conf
  /srv/custom-config.yaml
  "

  BACKUP_BLACKLIST="
  /root/.cache
  ${BASE_DIR}/log/
  "
  ```

- Il parser concatena automaticamente le righe e supporta variabili come `${BASE_DIR}`, così puoi descrivere chiaramente cosa includere/escludere senza trascinarti cartelle inutili.

#### Storage Configuration (Phase 4.2)

The Go implementation supports three-tier storage with automatic filesystem detection and retention policies:

**Storage Backends:**

```bash
# Primary (Local) Storage - Always enabled
# Stored in BACKUP_PATH, retention controlled by MAX_LOCAL_BACKUPS
MAX_LOCAL_BACKUPS=7

# Secondary Storage - Optional filesystem-based backup (NFS/CIFS/local mount)
SECONDARY_ENABLED=false
SECONDARY_PATH=/mnt/secondary-backup
MAX_SECONDARY_BACKUPS=14

# Cloud Storage - Optional rclone-based remote backup
CLOUD_ENABLED=false
CLOUD_REMOTE=rclone-remote:pbs-backups
MAX_CLOUD_BACKUPS=30
```

**Retention Policies:**

The Go implementation supports two intelligent retention strategies:

**1. Simple Retention (Count-based) - Default**
```bash
# Keep N most recent backups per destination
MAX_LOCAL_BACKUPS=15
MAX_SECONDARY_BACKUPS=15
MAX_CLOUD_BACKUPS=15
```

**2. GFS Retention (Grandfather-Father-Son) - Time-distributed**
```bash
# Automatically enabled when RETENTION_* variables are set
# Distributes backups across time periods for better historical coverage

RETENTION_DAILY=7        # Keep last 7 days of backups
RETENTION_WEEKLY=4       # Keep 4 weekly backups (1 per ISO week)
RETENTION_MONTHLY=12     # Keep 12 monthly backups (1 per month)
RETENTION_YEARLY=3       # Keep 3 yearly backups (1 per year)
```

**GFS Features:**
- ✅ **Automatic classification**: Backups are categorized as daily/weekly/monthly/yearly
- ✅ **ISO week numbering**: Weekly backups use standard ISO 8601 week numbers
- ✅ **Intelligent deletion**: Preserves time-distributed backups automatically
- ✅ **Per-destination control**: Different policies for local, secondary, and cloud
- ✅ **Predictive logging**: Shows what will be deleted before deletion

**Example GFS output:**
```
Local storage initialized (present 25 backups)
  Policy: GFS (daily=7, weekly=4, monthly=12, yearly=3)
  Total: 25/-
  Daily: 7/7
  Weekly: 4/4
  Monthly: 12/12
  Yearly: 2/3

GFS classification → daily: 7/7, weekly: 4/4, monthly: 12/12, yearly: 2/3, to_delete: 0
```

Secondary storage copies are executed directly in Go (atomic copy + fsync + rename), so no rsync dependency or extra tuning flags are required. Cancellation/timeout is inherited from the main context, and detailed progress is logged at DEBUG level.

**Rclone Settings (for cloud storage):**

```bash
# Connection timeout: quick check if remote is accessible
RCLONE_TIMEOUT_CONNECTION=30

# Operation timeout: complete upload/download operations
RCLONE_TIMEOUT_OPERATION=300

RCLONE_BANDWIDTH_LIMIT=     # e.g., "10M" for 10 MB/s, empty = unlimited
RCLONE_TRANSFERS=4          # parallel transfers
RCLONE_RETRIES=3            # retry attempts on failure
RCLONE_VERIFY_METHOD=primary # primary | alternative
```

**Batch Deletion (cloud storage - avoid API rate limits):**

```bash
CLOUD_BATCH_SIZE=20   # files per batch
CLOUD_BATCH_PAUSE=1   # seconds between batches
```

**Bundle Associated Files:**

```bash
# Create bundle.tar with compression=0 containing backup + checksum + metadata
BUNDLE_ASSOCIATED_FILES=true
```

Quando il bundling è attivo la pipeline crea immediatamente `*.bundle.tar` e
rimuove i file originali (`.tar.xz`, `.sha256`, `.metadata`, `.manifest.json`);
gli storage secondario/cloud ricevono quindi lo stesso bundle già pronto.

**Error Handling:**
- **Primary (local) storage**: Errors are CRITICAL and abort the backup
- **Secondary storage**: Errors are NON-CRITICAL, log warnings and continue
- **Cloud storage**: Errors are NON-CRITICAL, log warnings and continue
- Filesystem detection automatically excludes incompatible filesystems (CIFS/SMB for secondary)
- During startup each storage path is probed and the detected filesystem is
  displayed next to the path (e.g. `/backup-test [ext4]`). If the filesystem
  does not support ownership (FAT32/NTFS, alcuni network FS) the tool logs a
  warning and automatically skips chown/chmod while proceeding with the backup.
- Ogni storage logga il numero reale di backup **subito dopo** la copia/upload
  e nuovamente dopo la retention (il log `DEBUG … current backups detected …`
  usa un `List()` reale), così controlli subito cosa è stato visto.

### Future: YAML Configuration

The Go version will support YAML configuration (while maintaining .env compatibility):

```yaml
general:
  debug_level: 0
  use_color: true

compression:
  type: xz
  level: 6
  threads: 0  # auto

storage:
  local_path: /opt/proxmox-backup/backup
  secondary_enabled: true
  cloud_enabled: true
```

---

## 🧪 Testing

### Running the Go Version (Development)

```bash
# Build first
make build

# Run in check mode (safe, no changes)
./build/proxmox-backup --check

# Compare output with Bash version
./script/proxmox-backup.sh --check > /tmp/bash-output.txt
./build/proxmox-backup --check > /tmp/go-output.txt
diff /tmp/bash-output.txt /tmp/go-output.txt
```

### Side-by-Side Testing

During migration, both versions can run simultaneously:

```bash
# Production (Bash)
/opt/proxmox-backup/script/proxmox-backup.sh

# Development (Go)
/opt/proxmox-backup/build/proxmox-backup
```

### Coverage Guard

Use the built-in coverage gate to prevent regressions:

```bash
# Default threshold: 50%
make coverage-check

# Hardened threshold (e.g. CI)
make coverage-check COVERAGE_THRESHOLD=70
```

The target runs `go test` with `-coverpkg=./...` and fails if total coverage
falls below the requested threshold.

---

## 🤝 Contributing

Contributions are welcome! Here's how you can help:

1. **Report bugs**: Open an issue with details and logs
2. **Feature requests**: Suggest improvements
3. **Code contributions**: Submit pull requests
4. **Documentation**: Improve docs or add examples

### Development Workflow

```bash
# Fork and clone
git clone https://github.com/yourusername/proxmox-backup.git
cd proxmox-backup

# Create feature branch
git checkout -b feature/my-feature

# Make changes, test
make test
make lint

# Commit (follow conventional commits)
git commit -m "feat: add new feature"

# Push and create PR
git push origin feature/my-feature
```

### Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Write tests for new code
- Document exported functions

---

## 📊 Performance Comparison

| Operation | Bash | Go | Improvement |
|-----------|------|-----|-------------|
| Startup time | ~500ms | ~50ms | **10x faster** |
| Configuration parsing | ~200ms | ~10ms | **20x faster** |
| Parallel uploads | Sequential | Concurrent | **3-5x faster** |
| Binary size | N/A | ~15MB | Single binary |

*Note: Go benchmarks will be added as modules are migrated.*

---

## 🐛 Troubleshooting

### Common Issues

**Issue: `go: cannot find main module`**
```bash
cd /opt/proxmox-backup  # Make sure you're in project root
ls go.mod               # Should exist
```

**Issue: Build fails**
```bash
go mod tidy             # Update dependencies
make clean              # Clean build artifacts
make build              # Rebuild
```

**Issue: Binary doesn't find config**
```bash
# Use absolute paths in code
cfg, err := config.LoadConfig("/opt/proxmox-backup/env/backup.env")
```

See [MIGRATION_PLAN.md](MIGRATION_PLAN.md) FAQ section for more troubleshooting tips.

---

## 📄 License

[MIT License](LICENSE) - See LICENSE file for details

---

## 🙏 Acknowledgments

- Original Bash implementation: 20,370 lines of production-tested code
- Proxmox community for the excellent platform
- Go community for the amazing ecosystem

---

## 📞 Support

- **GitHub Issues**: [Report bugs](https://github.com/tis24dev/proxmox-backup/issues)
- **GitHub Discussions**: [Ask questions](https://github.com/tis24dev/proxmox-backup/discussions)
- **Email**: [your-email@example.com]

---

## 🗺️ Roadmap

- [x] Create migration plan
- [x] **Phase 0**: Project setup
- [x] **Phase 1**: Core infrastructure
- [x] **Phase 2**: Hybrid orchestrator
- [x] **Phase 3**: Environment detection
- [x] **Phase 4.1**: Collection (PVE/PBS/System)
- [x] **Phase 4.2**: Storage operations (Local/Secondary/Cloud)
- [x] **Phase 5.1**: Notifications (Telegram/Email)
- [ ] **Phase 5.2**: Metrics (Prometheus)
- [ ] Performance benchmarks
- [ ] Complete test coverage (>80%)
- [ ] Documentation (godoc)
- [ ] First stable release (v1.0.0)

---

**Last updated**: 2025-11-10
**Version**: 0.1.0-dev (Phase 5.1 completed)
