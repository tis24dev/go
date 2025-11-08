# Piano di Migrazione: Proxmox Backup da Bash a Go

> **Repository:** [github.com/tis24dev/proxmox-backup](https://github.com/tis24dev/proxmox-backup)
> **Versione Bash attuale:** ~20.370 righe di codice in 21 file
> **Obiettivo:** Migrazione graduale e incrementale a Go
> **Timeline stimata:** 32-40 settimane nominali (+20% slack pianificabile)
> **Ultimo aggiornamento:** 2025-11-05

---

## 🎯 Quick Start

**Se vuoi iniziare subito:**

```bash
# 1. Setup iniziale (Fase 0)
cd /opt/proxmox-backup
go mod init github.com/tis24dev/proxmox-backup

# 2. Crea struttura
mkdir -p cmd/proxmox-backup internal/{config,logging,types} pkg/utils build

# 3. Leggi questo documento dall'inizio
# 4. Inizia dalla Fase 1
```

**Filosofia della migrazione:**
- ✅ **Incrementale**: Mai riscrivere tutto in una volta
- ✅ **Coesistenza**: Bash e Go funzionano insieme durante la transizione
- ✅ **Testabile**: Ogni fase è validata prima di procedere
- ✅ **Rollback**: Sempre possibile tornare alla versione Bash

---

## 🚀 TL;DR Aggiornato

- **Incrementalità disciplinata**: orchestratore Go affianca gli script Bash finché ogni dominio non raggiunge la parità funzionale.
- **Parità verificata**: regression harness automatico (golden results) più canary rollout su nodi selezionati.
- **Timeline realistica**: 32-40 settimane nominali con slack del 20% (~9 mesi) e exit criteria misurabili per ogni fase.
- **Cut-over governato**: feature flag per ogni blocco (CLI, orchestrator, backup, storage, notify, metrics) e rollback one-click.
- **Investimento iniziale**: tooling, documentation e observability potenziati prima di affrontare le fasi a maggior rischio (backup/storage).

---

## 📋 Indice

0. [TL;DR Aggiornato](#-tldr-aggiornato)
1. [Panoramica del Progetto](#panoramica-del-progetto)
2. [Obiettivi della Migrazione](#obiettivi-della-migrazione)
3. [Strategia di Migrazione](#strategia-di-migrazione)
4. [Struttura del Progetto Go](#struttura-del-progetto-go)
5. [Fasi di Migrazione](#fasi-di-migrazione)
6. [Timeline e Milestone](#timeline-e-milestone)
7. [Checklist Operative](#checklist-operative)
8. [Best Practices](#best-practices)
9. [Deployment & Operatività](#-deployment--operatività)
10. [Governance & Comunicazione](#-governance--comunicazione)
11. [Sfide e Soluzioni](#sfide-e-soluzioni)
12. [Riferimenti](#riferimenti)

---

## 📊 Panoramica del Progetto

### Stato Attuale (Bash)

**Metriche del progetto:**
- **Linee di codice:** ~20.370 righe
- **Moduli libreria:** 16 file
- **Script eseguibili:** 5 file
- **Funzioni totali:** 351+ funzioni
- **Parametri configurazione:** 90+ opzioni

**Struttura attuale:**
```
/opt/proxmox-backup/
├── script/
│   ├── proxmox-backup.sh          # Main orchestrator (642 righe)
│   ├── proxmox-restore.sh         # Restore functionality
│   ├── security-check.sh          # Security auditing
│   ├── fix-permissions.sh         # Permission management
│   └── server-id-manager.sh       # Server identity
├── lib/
│   ├── core.sh                    # Core functionality (965 righe)
│   ├── environment.sh             # Environment setup (693 righe)
│   ├── log.sh                     # Logging system (1,865 righe)
│   ├── utils.sh                   # Utilities (1,830 righe)
│   ├── backup_collect.sh          # Data collection
│   ├── backup_collect_pbspve.sh   # PVE/PBS specific
│   ├── backup_create.sh           # Backup creation
│   ├── backup_verify.sh           # Verification
│   ├── backup_manager.sh          # Lifecycle management
│   ├── storage.sh                 # Storage operations
│   ├── notify.sh                  # Notifications
│   ├── email_relay.sh             # Email delivery
│   ├── metrics.sh                 # Prometheus metrics
│   ├── metrics_collect.sh         # Metrics collection
│   ├── security.sh                # Security checks
│   └── utils_counting.sh          # Counting utilities
├── env/backup.env                 # Configuration (362 righe)
├── config/                        # System configs
├── backup/                        # Backup files
├── log/                           # Logs
└── secure_account/                # Credentials
```

**Caratteristiche principali:**
- ✅ Sistema di backup enterprise-grade per Proxmox VE/PBS
- ✅ Multi-storage support (local, secondary, cloud)
- ✅ Integrazione Prometheus per monitoring
- ✅ Notifiche Telegram ed Email
- ✅ Sistema di logging sofisticato con buffering
- ✅ Gestione retention policies
- ✅ Upload cloud paralleli (rclone)
- ✅ Verifica integrità con checksum
- ✅ Security auditing

---

## 🎯 Obiettivi della Migrazione

### Obiettivi Tecnici

1. **Performance**
   - ⚡ Esecuzione più veloce (Go compila in binario nativo)
   - ⚡ Compressione parallela più efficiente
   - ⚡ Upload cloud concorrenti con goroutines

2. **Affidabilità**
   - 🛡️ Type safety (catch errors at compile time)
   - 🛡️ Error handling esplicito
   - 🛡️ Testing automatico completo
   - 🛡️ Meno dipendenze esterne

3. **Manutenibilità**
   - 📦 Codice più strutturato e modulare
   - 📦 Dependency management (go.mod)
   - 📦 Documentazione auto-generata (godoc)
   - 📦 IDE support migliore

4. **Deployment**
   - 🚀 Single binary (no bash runtime dependencies)
   - 🚀 Cross-compilation per diverse architetture
   - 🚀 Versioning chiaro

### Obiettivi Non-Funzionali

- ✅ **Zero downtime**: sistema bash continua a funzionare durante migrazione
- ✅ **Backward compatibility**: supporto configurazione esistente
- ✅ **Graduale**: migrazione incrementale, modulo per modulo
- ✅ **Testabile**: ogni fase validata prima di procedere

---

## 🔄 Strategia di Migrazione

### Approccio: Bottom-Up Incrementale

```
┌─────────────────────────────────────────────────────────────┐
│                    STRATEGIA GENERALE                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. Costruisci fondamenta Go (config, logging, errors)      │
│  2. Crea orchestrator Go che chiama moduli bash            │
│  3. Migra moduli uno alla volta                             │
│  4. Testa in parallelo con versione bash                    │
│  5. Rimuovi gradualmente dipendenze bash                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Principi Guida

1. **Incrementalità**: Mai sostituire tutto in una volta
2. **Parallelismo**: Entrambe le versioni funzionanti durante transizione
3. **Testing continuo**: Confronto output bash vs Go
4. **Rollback facile**: Sempre possibile tornare indietro
5. **Priorità ai benefici**: Migra prima i moduli che danno più vantaggi

### Gestione del Rischio

| Rischio | Mitigazione |
|---------|-------------|
| Bug in versione Go | Mantieni bash funzionante per rollback |
| Comportamento diverso | Test side-by-side, confronto output |
| Dipendenze mancanti | Usa exec.Command() per chiamare tool esterni |
| Performance peggiori | Benchmark prima/dopo, profiling |
| Incompatibilità config | Parser backward-compatible per .env |

---

## 📁 Struttura del Progetto Go

### Layout Proposto

```
/opt/proxmox-backup/
├── go.mod                         # Go module definition
├── go.sum                         # Dependency checksums
├── Makefile                       # Build automation
├── README.md                      # Documentazione
├── MIGRATION_PLAN.md              # Questo file
│
├── cmd/                           # Executables
│   ├── proxmox-backup/
│   │   └── main.go               # Main entry point
│   ├── proxmox-restore/
│   │   └── main.go               # Restore command
│   └── proxmox-backup-tools/
│       └── main.go               # Utility tools
│
├── internal/                      # Private packages (uso interno)
│   ├── config/
│   │   ├── config.go             # Configuration struct
│   │   ├── loader.go             # YAML/ENV loader
│   │   └── validator.go          # Config validation
│   │
│   ├── logging/
│   │   ├── logger.go             # Logger interface
│   │   ├── writer.go             # Multi-storage writer
│   │   ├── formatter.go          # Log formatting
│   │   └── buffer.go             # Buffered logging
│   │
│   ├── types/
│   │   ├── exitcode.go           # Exit code constants
│   │   ├── status.go             # Status types
│   │   └── errors.go             # Custom error types
│   │
│   ├── environment/
│   │   ├── detect.go             # Proxmox type detection
│   │   └── validate.go           # Environment validation
│   │
│   ├── collect/
│   │   ├── collector.go          # Data collection interface
│   │   ├── pve.go                # PVE collector
│   │   └── pbs.go                # PBS collector
│   │
│   ├── archiver/
│   │   ├── archiver.go           # Archive creation
│   │   ├── compress.go           # Compression
│   │   └── checksum.go           # Checksum generation
│   │
│   ├── storage/
│   │   ├── storage.go            # Storage interface
│   │   ├── local.go              # Local storage
│   │   ├── secondary.go          # Secondary storage
│   │   ├── cloud.go              # Cloud storage (rclone)
│   │   └── rotation.go           # Retention policies
│   │
│   ├── notify/
│   │   ├── notifier.go           # Notifier interface
│   │   ├── telegram.go           # Telegram notifications
│   │   └── email.go              # Email notifications
│   │
│   ├── metrics/
│   │   ├── prometheus.go         # Prometheus exporter
│   │   └── collector.go          # Metrics collection
│   │
│   └── security/
│       ├── check.go              # Security checks
│       └── permissions.go        # Permission management
│
├── pkg/                           # Public packages (riusabili)
│   ├── utils/
│   │   ├── format.go             # Formatting utilities
│   │   ├── disk.go               # Disk operations
│   │   └── serverid.go           # Server ID generation
│   │
│   └── proxmox/
│       ├── client.go             # Proxmox API client
│       └── types.go              # Proxmox types
│
├── test/                          # Testing
│   ├── integration/              # Integration tests
│   ├── fixtures/                 # Test data
│   └── mocks/                    # Mock objects
│
├── scripts/                       # Scripts bash (da migrare)
│   ├── script/                   # Originali
│   └── lib/                      # Librerie originali
│
├── configs/
│   ├── backup.yaml.example       # Nuova config YAML
│   └── backup.env.example        # Old format (compatibilità)
│
└── build/                         # Build artifacts
    ├── proxmox-backup            # Binary compilato
    └── dist/                     # Release packages
```

### Convenzioni di Naming

- **Packages**: lowercase, single word (`logging`, `storage`)
- **Files**: lowercase, underscore (`storage_local.go`)
- **Interfaces**: suffisso `-er` (`Notifier`, `Collector`)
- **Structs**: PascalCase (`BackupConfig`, `LogWriter`)
- **Functions**: camelCase exported, lowercase internal

---

## 🚀 Fasi di Migrazione

## FASE 0: Setup Iniziale del Progetto

**Durata:** 1 settimana
**Obiettivo:** Creare l'infrastruttura base del progetto Go

### Attività

#### 0.1 Inizializzazione Go Module

**IMPORTANTE:** Usa il path GitHub anche per sviluppo locale (evita di dover cambiare import dopo):

```bash
cd /opt/proxmox-backup
go mod init github.com/tis24dev/proxmox-backup
```

> **Nota sul module path:**
> - `github.com/tis24dev/proxmox-backup` è il path che userai quando pubblicherai su GitHub
> - Funziona perfettamente anche in locale durante lo sviluppo
> - Gli import nei file Go saranno: `import "github.com/tis24dev/proxmox-backup/internal/config"`
> - Non serve avere il repository già su GitHub per usare questo path

#### 0.2 Struttura Directory

```bash
# Crea struttura Go (mantieni script/ e lib/ esistenti)
cd /opt/proxmox-backup
mkdir -p cmd/proxmox-backup
mkdir -p internal/{config,logging,types,environment}
mkdir -p pkg/{utils,proxmox}
mkdir -p test/{integration,fixtures,mocks}
mkdir -p configs
mkdir -p build
```

**Struttura finale del progetto:**
```
/opt/proxmox-backup/
├── script/                    # ← MANTIENI: Bash esistente (production)
│   ├── proxmox-backup.sh
│   └── lib/
├── env/                       # ← MANTIENI: Configurazione esistente
│   └── backup.env
├── backup/                    # ← CONDIVISO: Usato da Bash e Go
├── log/                       # ← CONDIVISO: Usato da Bash e Go
├── go.mod                     # ← NUOVO: Go module
├── go.sum                     # ← NUOVO: Go dependencies
├── Makefile                   # ← NUOVO: Build automation
├── .gitignore                 # ← NUOVO: Git ignore per Go
├── MIGRATION_PLAN.md          # ← Questo file
├── cmd/                       # ← NUOVO: Executables Go
│   └── proxmox-backup/
│       └── main.go
├── internal/                  # ← NUOVO: Private Go packages
│   ├── config/
│   ├── logging/
│   └── ...
└── pkg/                       # ← NUOVO: Public Go packages
    └── utils/
```

#### 0.3 File di Configurazione

**go.mod** (creato automaticamente dal comando init):
```go
module github.com/tis24dev/proxmox-backup

go 1.21

// Le dipendenze verranno aggiunte automaticamente quando usi 'go get'
```

**Makefile completo:**
```makefile
.PHONY: build test clean run build-release test-coverage lint fmt deps

# Build del progetto
build:
	@echo "Building proxmox-backup..."
	go build -o build/proxmox-backup ./cmd/proxmox-backup

# Build ottimizzato per release
build-release:
	@echo "Building release..."
	go build -ldflags="-s -w" -o build/proxmox-backup ./cmd/proxmox-backup

# Test
test:
	go test -v ./...

# Test con coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Lint
lint:
	go vet ./...
	@command -v golint >/dev/null 2>&1 && golint ./... || echo "golint not installed"

# Format code
fmt:
	go fmt ./...

# Clean build artifacts
clean:
	rm -rf build/
	rm -f coverage.out

# Run in development
run:
	go run ./cmd/proxmox-backup

# Install/update dependencies
deps:
	go mod download
	go mod tidy

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the project"
	@echo "  build-release - Build optimized release binary"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint          - Run linters"
	@echo "  fmt           - Format Go code"
	@echo "  clean         - Remove build artifacts"
	@echo "  run           - Run in development mode"
	@echo "  deps          - Download and tidy dependencies"
```

**.gitignore per Go:**
```gitignore
# Binari compilati
build/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with `go test -c`
*.test

# Output dei test di profiling
*.out

# Dependency directories
vendor/

# Go workspace file
go.work

# IDE files
.idea/
.vscode/
*.swp
*.swo
*~

# OS files
.DS_Store
Thumbs.db

# Backup/log files del sistema (NON committare in git)
backup/*.tar.gz
backup/*.tar.xz
backup/*.tar.gz.sha256
log/*.log
log/*.html
lock/*
secure_account/

# Metriche Prometheus (generate)
*.prom

# File temporanei
tmp/
temp/
```

#### 0.4 Setup Completo (Comandi da eseguire)

**Esegui questi comandi nella directory del progetto:**

```bash
# 1. Vai nella directory del progetto
cd /opt/proxmox-backup

# 2. Inizializza Go module
go mod init github.com/tis24dev/proxmox-backup

# 3. Crea struttura directory
mkdir -p cmd/proxmox-backup
mkdir -p internal/{config,logging,types,environment}
mkdir -p pkg/{utils,proxmox}
mkdir -p test/{integration,fixtures,mocks}
mkdir -p configs
mkdir -p build

# 4. Crea Makefile (copia il contenuto sopra)
cat > Makefile << 'EOF'
[contenuto Makefile sopra]
EOF

# 5. Crea .gitignore (copia il contenuto sopra)
cat > .gitignore << 'EOF'
[contenuto .gitignore sopra]
EOF

# 6. Verifica setup
go mod tidy
make help
```

#### 0.5 Git Strategy

**Opzione consigliata: Stesso repository con branch**

```bash
# Se non hai già inizializzato git
cd /opt/proxmox-backup
git init
git add .
git commit -m "Initial commit: Bash version"

# Crea branch per migrazione Go
git checkout -b migration-go

# Commit setup Go
git add go.mod .gitignore Makefile MIGRATION_PLAN.md
git add cmd/ internal/ pkg/
git commit -m "feat: initialize Go project structure

- Add go.mod with module path github.com/tis24dev/proxmox-backup
- Create directory structure (cmd, internal, pkg)
- Add Makefile for build automation
- Add .gitignore for Go project
- Add comprehensive migration plan"

# Durante sviluppo: commit incrementali sul branch migration-go
# Quando pronto: merge in main
```

**Workflow durante la migrazione:**

```bash
# Sviluppo sul branch Go
git checkout migration-go
# ... sviluppa ...
git add .
git commit -m "feat: implement config package"

# Test entrambe le versioni
./script/proxmox-backup.sh           # Bash (production)
./build/proxmox-backup                # Go (development)

# Quando un modulo Go è stabile
git checkout main
git merge migration-go

# Continua sviluppo
git checkout migration-go
```

**Pubblicazione su GitHub (quando pronto):**

```bash
# 1. Crea repository su GitHub: proxmox-backup

# 2. Aggiungi remote
git remote add origin https://github.com/tis24dev/proxmox-backup.git

# 3. Push del branch principale
git push -u origin main

# 4. Push del branch di sviluppo
git push -u origin migration-go
```

### Checklist Fase 0

- [ ] Go 1.21+ installato (verifica: `go version`)
- [ ] Directory `/opt/proxmox-backup` esistente
- [ ] Backup completo del sistema bash esistente
- [ ] `go mod init` eseguito con path corretto
- [ ] Struttura directory Go creata (cmd/, internal/, pkg/)
- [ ] Makefile creato e testato (`make help`)
- [ ] .gitignore creato
- [ ] Git inizializzato (se non già fatto)
- [ ] Branch `migration-go` creato
- [ ] Primo commit effettuato
- [ ] Comando `make build` funziona (anche se main.go non esiste ancora)
- [ ] MIGRATION_PLAN.md letto e compreso

---

## FASE 1: Infrastruttura Fondamentale

**Durata:** 2-3 settimane
**Obiettivo:** Costruire i package base necessari per tutto il resto

### 1.1 Configuration System

**File:** `internal/config/config.go`

**Obiettivi:**
- ✅ Struct Go per tutte le 90+ opzioni di configurazione
- ✅ Parser YAML (nuovo formato)
- ✅ Parser ENV (backward compatibility con `backup.env`)
- ✅ Validazione completa
- ✅ Valori di default

**Esempio implementazione:**
```go
package config

import (
    "fmt"
    "github.com/spf13/viper"
)

// Config rappresenta la configurazione completa del sistema
type Config struct {
    General  GeneralConfig  `mapstructure:"general"`
    Features FeaturesConfig `mapstructure:"features"`
    Paths    PathsConfig    `mapstructure:"paths"`
    Storage  StorageConfig  `mapstructure:"storage"`
    // ... altre sezioni
}

type GeneralConfig struct {
    BashVersion string `mapstructure:"bash_version"`
    DebugLevel  int    `mapstructure:"debug_level"`
    UseColor    bool   `mapstructure:"use_color"`
}

// LoadConfig carica la configurazione da file YAML o ENV
func LoadConfig(path string) (*Config, error) {
    v := viper.New()

    // Supporto YAML
    v.SetConfigFile(path)
    if err := v.ReadInConfig(); err != nil {
        // Fallback a ENV se YAML non esiste
        return loadFromEnv(path)
    }

    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }

    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("validate config: %w", err)
    }

    return &cfg, nil
}

// Validate verifica la correttezza della configurazione
func (c *Config) Validate() error {
    // Implementa validazione
    return nil
}
```

**Test da scrivere:**
```go
func TestLoadConfigYAML(t *testing.T) { ... }
func TestLoadConfigENV(t *testing.T) { ... }
func TestConfigValidation(t *testing.T) { ... }
func TestDefaultValues(t *testing.T) { ... }
```

### 1.2 Logging System

**File:** `internal/logging/logger.go`

**Obiettivi:**
- ✅ Multi-level logging (ERROR, WARNING, INFO, DEBUG, TRACE)
- ✅ Colori e emoji (come bash version)
- ✅ Buffered I/O per performance
- ✅ Multi-storage output
- ✅ Rotation support

**Esempio implementazione:**
```go
package logging

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

type Logger struct {
    zap    *zap.Logger
    buffer *Buffer
    config LogConfig
}

type LogConfig struct {
    Level      zapcore.Level
    UseColor   bool
    UseEmoji   bool
    BufferSize int
}

// New crea un nuovo logger
func New(cfg LogConfig) (*Logger, error) {
    zapCfg := zap.NewProductionConfig()
    zapCfg.Level = zap.NewAtomicLevelAt(cfg.Level)

    z, err := zapCfg.Build()
    if err != nil {
        return nil, err
    }

    return &Logger{
        zap:    z,
        buffer: NewBuffer(cfg.BufferSize),
        config: cfg,
    }, nil
}

// Error log error con emoji
func (l *Logger) Error(msg string, fields ...zap.Field) {
    if l.config.UseEmoji {
        msg = "❌ " + msg
    }
    l.zap.Error(msg, fields...)
}

// Warning log warning
func (l *Logger) Warning(msg string, fields ...zap.Field) {
    if l.config.UseEmoji {
        msg = "⚠️ " + msg
    }
    l.zap.Warn(msg, fields...)
}

// Info log info
func (l *Logger) Info(msg string, fields ...zap.Field) {
    if l.config.UseEmoji {
        msg = "ℹ️ " + msg
    }
    l.zap.Info(msg, fields...)
}
```

**Test da scrivere:**
```go
func TestLoggerCreation(t *testing.T) { ... }
func TestLogLevels(t *testing.T) { ... }
func TestBuffering(t *testing.T) { ... }
func TestEmoji(t *testing.T) { ... }
```

### 1.3 Error Handling & Exit Codes

**File:** `internal/types/exitcode.go`

**Obiettivi:**
- ✅ Gerarchia exit codes (SUCCESS, WARNING, ERROR)
- ✅ Custom error types
- ✅ Error wrapping e context

```go
package types

// Exit codes
const (
    ExitSuccess = 0
    ExitWarning = 1
    ExitError   = 2
)

// BackupError rappresenta un errore nel processo di backup
type BackupError struct {
    Op      string // Operazione che ha fallito
    Path    string // Path coinvolto (se applicabile)
    Err     error  // Error originale
    ExitCode int   // Exit code da usare
}

func (e *BackupError) Error() string {
    if e.Path != "" {
        return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
    }
    return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *BackupError) Unwrap() error {
    return e.Err
}
```

### 1.4 Utilities Essenziali

**File:** `pkg/utils/format.go`

**Obiettivi:**
- ✅ Formattazione size (bytes → human readable)
- ✅ Formattazione duration
- ✅ Operazioni file base
- ✅ Server ID generation (compatibile con bash)

```go
package utils

import (
    "fmt"
    "time"
)

// FormatBytes converte bytes in formato human-readable
func FormatBytes(bytes int64) string {
    const unit = 1024
    if bytes < unit {
        return fmt.Sprintf("%d B", bytes)
    }

    div, exp := int64(unit), 0
    for n := bytes / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }

    return fmt.Sprintf("%.1f %ciB",
        float64(bytes)/float64(div),
        "KMGTPE"[exp])
}

// FormatDuration formatta una durata
func FormatDuration(d time.Duration) string {
    // Implementazione simile a bash
    return d.String()
}
```

### Deliverables Fase 1

- [ ] Package `internal/config` completo e testato
- [ ] Package `internal/logging` completo e testato
- [ ] Package `internal/types` con exit codes ed errors
- [ ] Package `pkg/utils` con utilities base
- [ ] Test coverage >= 80%
- [ ] Documentazione godoc per tutti i package

---

## FASE 2: Orchestrator Go (Hybrid)

**Durata:** 2-3 settimane
**Obiettivo:** Convertire `proxmox-backup.sh` in Go mantenendo i moduli bash

### 2.1 Main Entry Point

**File:** `cmd/proxmox-backup/main.go`

**Struttura:**
```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/yourusername/proxmox-backup/internal/config"
    "github.com/yourusername/proxmox-backup/internal/logging"
    "github.com/yourusername/proxmox-backup/internal/types"
)

func main() {
    // Setup signal handling
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigCh
        cancel()
    }()

    // Run backup
    exitCode := run(ctx)
    os.Exit(exitCode)
}

func run(ctx context.Context) int {
    // 1. Load configuration
    cfg, err := config.LoadConfig("/opt/proxmox-backup/env/backup.env")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
        return types.ExitError
    }

    // 2. Setup logging
    logger, err := logging.New(logging.LogConfig{
        Level:    cfg.General.DebugLevel,
        UseColor: cfg.General.UseColor,
        UseEmoji: true,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
        return types.ExitError
    }
    defer logger.Close()

    // 3. Initialize backup orchestrator
    orchestrator := NewOrchestrator(cfg, logger)

    // 4. Run backup process
    if err := orchestrator.Run(ctx); err != nil {
        logger.Error("Backup failed", zap.Error(err))
        if berr, ok := err.(*types.BackupError); ok {
            return berr.ExitCode
        }
        return types.ExitError
    }

    logger.Info("Backup completed successfully")
    return types.ExitSuccess
}
```

### 2.2 Orchestrator Implementation

**File:** `cmd/proxmox-backup/orchestrator.go`

```go
package main

import (
    "context"
    "os/exec"
    "time"

    "github.com/yourusername/proxmox-backup/internal/config"
    "github.com/yourusername/proxmox-backup/internal/logging"
)

type Orchestrator struct {
    cfg    *config.Config
    logger *logging.Logger
    startTime time.Time
}

func NewOrchestrator(cfg *config.Config, logger *logging.Logger) *Orchestrator {
    return &Orchestrator{
        cfg:    cfg,
        logger: logger,
    }
}

func (o *Orchestrator) Run(ctx context.Context) error {
    o.startTime = time.Now()
    o.logger.Info("Starting backup process")

    // Phase 1: Environment validation
    if err := o.validateEnvironment(ctx); err != nil {
        return err
    }

    // Phase 2: Data collection (ancora in bash per ora)
    if err := o.collectData(ctx); err != nil {
        return err
    }

    // Phase 3: Backup creation (ancora in bash)
    if err := o.createBackup(ctx); err != nil {
        return err
    }

    // Phase 4: Storage management (ancora in bash)
    if err := o.manageStorage(ctx); err != nil {
        return err
    }

    // Phase 5: Notifications (ancora in bash)
    if err := o.sendNotifications(ctx); err != nil {
        return err
    }

    // Phase 6: Metrics export (ancora in bash)
    if err := o.exportMetrics(ctx); err != nil {
        return err
    }

    duration := time.Since(o.startTime)
    o.logger.Info("Backup completed",
        zap.Duration("duration", duration))

    return nil
}

// collectData chiama lo script bash per ora
func (o *Orchestrator) collectData(ctx context.Context) error {
    o.logger.Info("Collecting backup data...")

    cmd := exec.CommandContext(ctx,
        "/opt/proxmox-backup/scripts/lib/backup_collect.sh")

    output, err := cmd.CombinedOutput()
    if err != nil {
        o.logger.Error("Data collection failed",
            zap.String("output", string(output)))
        return fmt.Errorf("collect data: %w", err)
    }

    o.logger.Debug("Collection output", zap.String("output", string(output)))
    return nil
}

// Metodi simili per altre fasi...
```

### 2.3 Argument Parsing

**Usa cobra per CLI:**
```go
package main

import (
    "github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
    Use:   "proxmox-backup",
    Short: "Proxmox backup system",
    Long:  `Enterprise-grade backup system for Proxmox VE/PBS`,
    Run:   runBackup,
}

var (
    configFile  string
    checkMode   bool
    debugLevel  int
)

func init() {
    rootCmd.Flags().StringVarP(&configFile, "config", "c",
        "/opt/proxmox-backup/env/backup.env", "Config file path")
    rootCmd.Flags().BoolVar(&checkMode, "check", false,
        "Check configuration only")
    rootCmd.Flags().IntVarP(&debugLevel, "debug", "d", 0,
        "Debug level (0-5)")
}

func runBackup(cmd *cobra.Command, args []string) {
    // Implementazione
}
```

### Deliverables Fase 2

- [ ] Main entry point funzionante
- [ ] Orchestrator che chiama script bash
- [ ] Argument parsing con cobra
- [ ] Signal handling corretto
- [ ] Logging integrato
- [ ] Exit code management
- [ ] Compilazione riuscita: `make build`
- [ ] Test esecuzione: `./build/proxmox-backup --check`

---

## FASE 3: Environment & Core

**Durata:** 2 settimane
**Obiettivo:** Migrare moduli critici per l'orchestrator

### 3.1 Environment Detection

**File:** `internal/environment/detect.go`

**Migrazione di:** `lib/environment.sh`

**Obiettivi:**
- ✅ Rilevamento Proxmox type (PVE vs PBS)
- ✅ Verifica versioni
- ✅ Setup directory structure
- ✅ Validazione requisiti sistema

```go
package environment

import (
    "fmt"
    "os"
    "os/exec"
    "strings"
)

type ProxmoxType string

const (
    TypePVE ProxmoxType = "pve"
    TypePBS ProxmoxType = "pbs"
)

type Environment struct {
    Type    ProxmoxType
    Version string
}

// Detect rileva il tipo di ambiente Proxmox
func Detect() (*Environment, error) {
    // Controlla se è PBS
    if _, err := os.Stat("/usr/bin/proxmox-backup-manager"); err == nil {
        version, err := getPBSVersion()
        if err != nil {
            return nil, err
        }
        return &Environment{Type: TypePBS, Version: version}, nil
    }

    // Controlla se è PVE
    if _, err := os.Stat("/usr/bin/pveversion"); err == nil {
        version, err := getPVEVersion()
        if err != nil {
            return nil, err
        }
        return &Environment{Type: TypePVE, Version: version}, nil
    }

    return nil, fmt.Errorf("proxmox environment not detected")
}

func getPVEVersion() (string, error) {
    out, err := exec.Command("pveversion", "-v").Output()
    if err != nil {
        return "", err
    }
    // Parse output
    return strings.TrimSpace(string(out)), nil
}

func getPBSVersion() (string, error) {
    out, err := exec.Command("proxmox-backup-manager", "versions").Output()
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(out)), nil
}
```

### 3.2 Directory Setup

**File:** `internal/environment/setup.go`

```go
package environment

import (
    "fmt"
    "os"
    "path/filepath"
)

// SetupDirectories crea la struttura directory necessaria
func SetupDirectories(basePath string) error {
    dirs := []string{
        filepath.Join(basePath, "backup"),
        filepath.Join(basePath, "log"),
        filepath.Join(basePath, "lock"),
        filepath.Join(basePath, "config"),
        filepath.Join(basePath, "secure_account"),
    }

    for _, dir := range dirs {
        if err := os.MkdirAll(dir, 0755); err != nil {
            return fmt.Errorf("create directory %s: %w", dir, err)
        }
    }

    return nil
}

// ValidatePermissions verifica i permessi dei file critici
func ValidatePermissions(envFile string) error {
    info, err := os.Stat(envFile)
    if err != nil {
        return fmt.Errorf("stat %s: %w", envFile, err)
    }

    // Il file env deve essere 400 (r--------)
    mode := info.Mode().Perm()
    if mode != 0400 {
        return fmt.Errorf("invalid permissions %o, expected 0400", mode)
    }

    return nil
}
```

### Deliverables Fase 3

- [ ] Package `internal/environment` completo
- [ ] Detection PVE/PBS funzionante
- [ ] Setup directory automatico
- [ ] Validazione permessi
- [ ] Test su entrambi gli ambienti (PVE e PBS)
- [ ] Integrazione in orchestrator

---

## FASE 4: Backup Operations

**Durata:** 4-6 settimane
**Obiettivo:** Migrare la logica di backup core

### 4.1 Data Collection

**File:** `internal/collect/collector.go`

**Migrazione di:** `lib/backup_collect.sh` + `lib/backup_collect_pbspve.sh`

```go
package collect

import (
    "context"
    "fmt"
)

// Collector interface per data collection
type Collector interface {
    Collect(ctx context.Context) (*CollectionResult, error)
}

type CollectionResult struct {
    Files       []string
    TotalSize   int64
    FileCount   int
    ErrorCount  int
}

// PVECollector raccoglie dati da Proxmox VE
type PVECollector struct {
    logger *logging.Logger
    config *config.Config
}

func NewPVECollector(cfg *config.Config, log *logging.Logger) *PVECollector {
    return &PVECollector{
        config: cfg,
        logger: log,
    }
}

func (c *PVECollector) Collect(ctx context.Context) (*CollectionResult, error) {
    result := &CollectionResult{}

    // Raccolta configurazioni PVE
    if err := c.collectPVEConfig(ctx, result); err != nil {
        return nil, err
    }

    // Raccolta dati cluster
    if err := c.collectClusterData(ctx, result); err != nil {
        return nil, err
    }

    // Raccolta configurazioni VM/CT
    if err := c.collectVMConfigs(ctx, result); err != nil {
        return nil, err
    }

    return result, nil
}

func (c *PVECollector) collectPVEConfig(ctx context.Context, result *CollectionResult) error {
    // Implementazione raccolta da /etc/pve/
    paths := []string{
        "/etc/pve/datacenter.cfg",
        "/etc/pve/storage.cfg",
        "/etc/pve/user.cfg",
        // ... altri file
    }

    for _, path := range paths {
        if err := c.safeCopy(path, result); err != nil {
            c.logger.Warning("Failed to copy", zap.String("path", path))
            result.ErrorCount++
        }
    }

    return nil
}
```

### 4.2 Backup Creation & Compression

**File:** `internal/archiver/archiver.go`

**Migrazione di:** `lib/backup_create.sh`

```go
package archiver

import (
    "archive/tar"
    "compress/gzip"
    "context"
    "io"
    "os"
    "path/filepath"
)

type CompressionType string

const (
    CompressionXZ    CompressionType = "xz"
    CompressionZSTD  CompressionType = "zstd"
    CompressionGZIP  CompressionType = "gzip"
    CompressionBZIP2 CompressionType = "bzip2"
)

type Archiver struct {
    compression CompressionType
    level       int
    threads     int
    logger      *logging.Logger
}

func NewArchiver(cfg *config.CompressionConfig, log *logging.Logger) *Archiver {
    return &Archiver{
        compression: CompressionType(cfg.Type),
        level:       cfg.Level,
        threads:     cfg.Threads,
        logger:      log,
    }
}

func (a *Archiver) CreateArchive(ctx context.Context, srcDir, dstPath string) error {
    a.logger.Info("Creating archive",
        zap.String("source", srcDir),
        zap.String("destination", dstPath),
        zap.String("compression", string(a.compression)))

    // Crea file output
    outFile, err := os.Create(dstPath)
    if err != nil {
        return fmt.Errorf("create archive file: %w", err)
    }
    defer outFile.Close()

    // Setup compressione
    writer, err := a.setupCompression(outFile)
    if err != nil {
        return err
    }
    defer writer.Close()

    // Crea tar archive
    tw := tar.NewWriter(writer)
    defer tw.Close()

    // Walk directory tree
    return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // Check context cancellation
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        return a.addToTar(tw, srcDir, path, info)
    })
}

func (a *Archiver) setupCompression(w io.Writer) (io.WriteCloser, error) {
    switch a.compression {
    case CompressionGZIP:
        return gzip.NewWriterLevel(w, a.level)
    case CompressionXZ:
        // Usa libreria xz
        return nil, fmt.Errorf("xz not implemented yet")
    default:
        return nil, fmt.Errorf("unsupported compression: %s", a.compression)
    }
}
```

### 4.3 Backup Verification

**File:** `internal/verifier/verifier.go`

**Migrazione di:** `lib/backup_verify.sh`

```go
package verifier

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "os"
)

type Verifier struct {
    logger *logging.Logger
}

func NewVerifier(log *logging.Logger) *Verifier {
    return &Verifier{logger: log}
}

// VerifyChecksum verifica il checksum di un file
func (v *Verifier) VerifyChecksum(filePath string, expectedSum string) error {
    v.logger.Info("Verifying checksum", zap.String("file", filePath))

    actualSum, err := v.calculateChecksum(filePath)
    if err != nil {
        return err
    }

    if actualSum != expectedSum {
        return fmt.Errorf("checksum mismatch: expected %s, got %s",
            expectedSum, actualSum)
    }

    v.logger.Info("Checksum verified successfully")
    return nil
}

func (v *Verifier) calculateChecksum(filePath string) (string, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", err
    }

    return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateChecksum genera checksum per un file
func (v *Verifier) GenerateChecksum(filePath string) (string, error) {
    return v.calculateChecksum(filePath)
}
```

### Deliverables Fase 4

- [ ] Package `internal/collect` con PVE/PBS collectors
- [ ] Package `internal/archiver` con compression
- [ ] Package `internal/verifier` con checksum
- [ ] Test con backup reali
- [ ] Benchmark performance vs bash
- [ ] Integrazione in orchestrator

---

## FASE 5: Storage Management

**Durata:** 4-6 settimane (FASE PIÙ COMPLESSA)
**Obiettivo:** Multi-storage, cloud upload, retention

### 5.1 Storage Interface

**File:** `internal/storage/storage.go`

**Migrazione di:** `lib/storage.sh`

```go
package storage

import (
    "context"
    "io"
)

// Storage interface per operazioni storage
type Storage interface {
    // Upload carica un file
    Upload(ctx context.Context, src, dst string) error

    // Download scarica un file
    Download(ctx context.Context, src, dst string) error

    // List elenca i file
    List(ctx context.Context, path string) ([]FileInfo, error)

    // Delete elimina un file
    Delete(ctx context.Context, path string) error

    // Type ritorna il tipo di storage
    Type() StorageType
}

type StorageType string

const (
    TypeLocal     StorageType = "local"
    TypeSecondary StorageType = "secondary"
    TypeCloud     StorageType = "cloud"
)

type FileInfo struct {
    Path    string
    Size    int64
    ModTime time.Time
}
```

### 5.2 Local Storage

**File:** `internal/storage/local.go`

```go
package storage

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"
)

type LocalStorage struct {
    basePath string
    logger   *logging.Logger
}

func NewLocalStorage(basePath string, log *logging.Logger) *LocalStorage {
    return &LocalStorage{
        basePath: basePath,
        logger:   log,
    }
}

func (s *LocalStorage) Upload(ctx context.Context, src, dst string) error {
    dstPath := filepath.Join(s.basePath, dst)

    // Crea directory se necessario
    if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
        return fmt.Errorf("create directory: %w", err)
    }

    // Copia file
    srcFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()

    dstFile, err := os.Create(dstPath)
    if err != nil {
        return err
    }
    defer dstFile.Close()

    if _, err := io.Copy(dstFile, srcFile); err != nil {
        return err
    }

    return dstFile.Sync()
}

func (s *LocalStorage) Type() StorageType {
    return TypeLocal
}
```

### 5.3 Cloud Storage (rclone)

**File:** `internal/storage/cloud.go`

```go
package storage

import (
    "context"
    "fmt"
    "os/exec"
    "strings"
)

type CloudStorage struct {
    remote     string
    config     *config.CloudConfig
    logger     *logging.Logger
}

func NewCloudStorage(cfg *config.CloudConfig, log *logging.Logger) *CloudStorage {
    return &CloudStorage{
        remote: cfg.Remote,
        config: cfg,
        logger: log,
    }
}

func (s *CloudStorage) Upload(ctx context.Context, src, dst string) error {
    s.logger.Info("Uploading to cloud",
        zap.String("source", src),
        zap.String("remote", s.remote))

    remotePath := fmt.Sprintf("%s:%s", s.remote, dst)

    args := []string{
        "copy",
        src,
        remotePath,
        "--progress",
    }

    // Aggiungi bandwidth limit se configurato
    if s.config.BandwidthLimit != "" {
        args = append(args, "--bwlimit", s.config.BandwidthLimit)
    }

    cmd := exec.CommandContext(ctx, "rclone", args...)

    output, err := cmd.CombinedOutput()
    if err != nil {
        s.logger.Error("Upload failed",
            zap.String("output", string(output)))
        return fmt.Errorf("rclone upload: %w", err)
    }

    s.logger.Info("Upload completed successfully")
    return nil
}

func (s *CloudStorage) Type() StorageType {
    return TypeCloud
}
```

### 5.4 Rotation & Retention

**File:** `internal/storage/rotation.go`

```go
package storage

import (
    "context"
    "sort"
    "time"
)

type RotationPolicy struct {
    MaxBackups int
    MaxAge     time.Duration
}

type RotationManager struct {
    storage Storage
    policy  RotationPolicy
    logger  *logging.Logger
}

func NewRotationManager(s Storage, p RotationPolicy, log *logging.Logger) *RotationManager {
    return &RotationManager{
        storage: s,
        policy:  p,
        logger:  log,
    }
}

func (rm *RotationManager) Rotate(ctx context.Context, path string) error {
    // Lista backup esistenti
    files, err := rm.storage.List(ctx, path)
    if err != nil {
        return err
    }

    // Ordina per data (più recente prima)
    sort.Slice(files, func(i, j int) bool {
        return files[i].ModTime.After(files[j].ModTime)
    })

    // Rimuovi backup vecchi (oltre MaxBackups)
    if len(files) > rm.policy.MaxBackups {
        for _, f := range files[rm.policy.MaxBackups:] {
            rm.logger.Info("Removing old backup",
                zap.String("file", f.Path))

            if err := rm.storage.Delete(ctx, f.Path); err != nil {
                rm.logger.Error("Failed to delete",
                    zap.String("file", f.Path),
                    zap.Error(err))
            }
        }
    }

    // Rimuovi backup troppo vecchi (oltre MaxAge)
    if rm.policy.MaxAge > 0 {
        cutoff := time.Now().Add(-rm.policy.MaxAge)
        for _, f := range files {
            if f.ModTime.Before(cutoff) {
                rm.logger.Info("Removing expired backup",
                    zap.String("file", f.Path))

                if err := rm.storage.Delete(ctx, f.Path); err != nil {
                    rm.logger.Error("Failed to delete",
                        zap.String("file", f.Path),
                        zap.Error(err))
                }
            }
        }
    }

    return nil
}
```

### 5.5 Parallel Upload

**File:** `internal/storage/parallel.go`

```go
package storage

import (
    "context"
    "golang.org/x/sync/errgroup"
)

type ParallelUploader struct {
    storages []Storage
    logger   *logging.Logger
}

func NewParallelUploader(storages []Storage, log *logging.Logger) *ParallelUploader {
    return &ParallelUploader{
        storages: storages,
        logger:   log,
    }
}

func (pu *ParallelUploader) UploadToAll(ctx context.Context, src, dst string) error {
    g, ctx := errgroup.WithContext(ctx)

    for _, storage := range pu.storages {
        storage := storage // Capture for goroutine

        g.Go(func() error {
            pu.logger.Info("Starting upload",
                zap.String("type", string(storage.Type())))

            if err := storage.Upload(ctx, src, dst); err != nil {
                pu.logger.Error("Upload failed",
                    zap.String("type", string(storage.Type())),
                    zap.Error(err))
                return err
            }

            pu.logger.Info("Upload completed",
                zap.String("type", string(storage.Type())))
            return nil
        })
    }

    return g.Wait()
}
```

### Deliverables Fase 5

- [ ] Package `internal/storage` completo
- [ ] Interface Storage implementato
- [ ] Local, Secondary, Cloud storage
- [ ] Rotation policies
- [ ] Parallel upload con goroutines
- [ ] Test con storage reali
- [ ] Benchmark performance
- [ ] Integrazione in orchestrator

---

## FASE 6: Notifications & Metrics

**Durata:** 2-3 settimane
**Obiettivo:** Sistema di notifiche e metriche

### 6.1 Notification System

**File:** `internal/notify/notifier.go`

**Migrazione di:** `lib/notify.sh` + `lib/email_relay.sh`

```go
package notify

import (
    "context"
)

type Notifier interface {
    Send(ctx context.Context, msg *Message) error
    Type() NotifierType
}

type NotifierType string

const (
    TypeTelegram NotifierType = "telegram"
    TypeEmail    NotifierType = "email"
)

type Message struct {
    Title   string
    Body    string
    Status  BackupStatus
    Details map[string]string
}

type BackupStatus string

const (
    StatusSuccess BackupStatus = "success"
    StatusWarning BackupStatus = "warning"
    StatusError   BackupStatus = "error"
)
```

### 6.2 Telegram Notifier

**File:** `internal/notify/telegram.go`

```go
package notify

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

type TelegramNotifier struct {
    token  string
    chatID string
    logger *logging.Logger
}

func NewTelegramNotifier(token, chatID string, log *logging.Logger) *TelegramNotifier {
    return &TelegramNotifier{
        token:  token,
        chatID: chatID,
        logger: log,
    }
}

func (tn *TelegramNotifier) Send(ctx context.Context, msg *Message) error {
    // Formatta messaggio con emoji
    emoji := tn.getEmoji(msg.Status)
    text := fmt.Sprintf("%s *%s*\n\n%s", emoji, msg.Title, msg.Body)

    // Aggiungi dettagli
    if len(msg.Details) > 0 {
        text += "\n\n*Details:*\n"
        for k, v := range msg.Details {
            text += fmt.Sprintf("• %s: %s\n", k, v)
        }
    }

    // Invia via API Telegram
    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tn.token)

    payload := map[string]interface{}{
        "chat_id":    tn.chatID,
        "text":       text,
        "parse_mode": "Markdown",
    }

    jsonData, _ := json.Marshal(payload)

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("telegram API error: %s", resp.Status)
    }

    tn.logger.Info("Telegram notification sent successfully")
    return nil
}

func (tn *TelegramNotifier) getEmoji(status BackupStatus) string {
    switch status {
    case StatusSuccess:
        return "✅"
    case StatusWarning:
        return "⚠️"
    case StatusError:
        return "❌"
    default:
        return "ℹ️"
    }
}

func (tn *TelegramNotifier) Type() NotifierType {
    return TypeTelegram
}
```

### 6.3 Prometheus Metrics

**File:** `internal/metrics/prometheus.go`

**Migrazione di:** `lib/metrics.sh`

```go
package metrics

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    backupDuration = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "proxmox_backup_duration_seconds",
        Help: "Duration of last backup in seconds",
    })

    backupSize = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "proxmox_backup_size_bytes",
        Help: "Size of last backup in bytes",
    })

    backupStatus = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "proxmox_backup_status",
        Help: "Status of last backup (0=success, 1=warning, 2=error)",
    })

    backupTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "proxmox_backup_last_success_timestamp",
        Help: "Timestamp of last successful backup",
    })
)

type PrometheusExporter struct {
    textfileDir string
    logger      *logging.Logger
}

func NewPrometheusExporter(textfileDir string, log *logging.Logger) *PrometheusExporter {
    return &PrometheusExporter{
        textfileDir: textfileDir,
        logger:      log,
    }
}

func (pe *PrometheusExporter) Export(stats *BackupStats) error {
    // Update metrics
    backupDuration.Set(stats.Duration.Seconds())
    backupSize.Set(float64(stats.Size))
    backupStatus.Set(float64(stats.ExitCode))
    backupTimestamp.Set(float64(stats.Timestamp.Unix()))

    // Write to textfile
    filename := filepath.Join(pe.textfileDir, "proxmox_backup.prom")

    f, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("create metrics file: %w", err)
    }
    defer f.Close()

    // Write metrics in Prometheus format
    fmt.Fprintf(f, "# HELP proxmox_backup_duration_seconds Duration of last backup\n")
    fmt.Fprintf(f, "# TYPE proxmox_backup_duration_seconds gauge\n")
    fmt.Fprintf(f, "proxmox_backup_duration_seconds %.2f\n", stats.Duration.Seconds())

    fmt.Fprintf(f, "# HELP proxmox_backup_size_bytes Size of last backup\n")
    fmt.Fprintf(f, "# TYPE proxmox_backup_size_bytes gauge\n")
    fmt.Fprintf(f, "proxmox_backup_size_bytes %d\n", stats.Size)

    // ... altri metrics

    pe.logger.Info("Prometheus metrics exported",
        zap.String("file", filename))

    return nil
}

type BackupStats struct {
    Duration  time.Duration
    Size      int64
    ExitCode  int
    Timestamp time.Time
}
```

### Deliverables Fase 6

- [ ] Package `internal/notify` con Telegram ed Email
- [ ] Package `internal/metrics` con Prometheus exporter
- [ ] Test notifiche reali
- [ ] Test metriche Prometheus
- [ ] Integrazione in orchestrator

---

## 📅 Timeline e Milestone

### Timeline Complessiva

```
┌──────────────────────────────────────────────────────────────────────────────┐
│            PIANO TEMPORALE (40 SETTIMANE MAX CON SLACK 20%)                  │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  F0 Setup & Governance              ████ (2-3 sett)                          │
│  F1 Fondamenta Tecniche             ███████████ (5-6 sett)                   │
│  F2 Orchestrator Ibrido             █████████ (4-5 sett)                     │
│  F3 Environment & Core              ████████████ (6-8 sett)                  │
│  F4 Backup Operations               ██████████████████ (8-10 sett)           │
│  F5 Storage Management              ███████████████ (6-8 sett)               │
│  F6 Notifications & Hardening       ████████ (4-5 sett)                      │
│  Go-Live & Stabilizzazione          ███ (2-3 sett)                           │
│  Slack pianificato (20%)            ......... (6-8 sett)                     │
│                                                                              │
│  ORIZZONTE: 32-40 settimane nominali + slack → finestra 8-9 mesi             │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Allocazione con Slack

| Fase | Durata nominale | Slack (20%) | Durata massima | Owner primario |
|------|-----------------|-------------|----------------|----------------|
| 0 · Setup & Governance | 2 sett | +0.5 sett | 2.5 sett | Tech Lead |
| 1 · Fondamenta Tecniche | 5 sett | +1 sett | 6 sett | Dev A |
| 2 · Orchestrator Ibrido | 4 sett | +1 sett | 5 sett | Dev B |
| 3 · Environment & Core | 6 sett | +1.5 sett | 7.5 sett | Dev A |
| 4 · Backup Operations | 8 sett | +2 sett | 10 sett | Dev A+B |
| 5 · Storage Management | 6 sett | +1.5 sett | 7.5 sett | Dev B |
| 6 · Notifications & Hardening | 4 sett | +1 sett | 5 sett | DevOps |
| Go-Live & Stabilizzazione | 2 sett | +0.5 sett | 2.5 sett | Cross-team |

### Milestone Principali

| # | Milestone | Finestra (sett) | Deliverable chiave |
|---|-----------|-----------------|--------------------|
| **M0** | Setup completo | 2-3 | Progetto Go inizializzato, governance attiva |
| **M1** | Fondamenta Go | 7-9 | Config, logging, feature flag pronti |
| **M2** | Orchestrator ibrido | 11-14 | Binario Go che orchestra moduli Bash |
| **M3** | Environment & Core | 17-21 | Pre-flight Go equivalenti alla versione Bash |
| **M4** | Backup parity | 25-31 | Pipeline backup Go a parità con Bash |
| **M5** | Storage parity | 33-38 | Driver storage Go con retention validata |
| **M6** | Notifiche & metriche | 37-43 | Notify/metrics Go, security review completata |
| **M7** | Production ready | 39-45 | Rollout completato, runbook aggiornato |

### Checkpoint di Validazione

**Ogni fase deve superare questi checkpoint prima di procedere:**

1. ✅ **Code Review**: Codice revisionato e approvato (2 reviewer per moduli critici)
2. ✅ **Unit Tests**: Coverage target ≥ 80% sui package coinvolti
3. ✅ **Regression Harness**: Parity test Bash vs Go senza differenze non attese
4. ✅ **Integration/Soak Test**: Pipeline end-to-end e job lunghi in staging superati
5. ✅ **Performance**: Benchmark Go vs Bash dentro margini concordati (±15%)
6. ✅ **Canary & Observability**: Canary stabile (≥ 1 settimana) con metriche/alerting verdi
7. ✅ **Documentation**: godoc, runbook e changelog aggiornati

---

## ✅ Checklist Operative

### Checklist Pre-Migrazione

- [ ] Backup completo del sistema attuale
- [ ] Git repository inizializzato
- [ ] Ambiente di test configurato
- [ ] Go 1.21+ installato
- [ ] Dipendenze sistema verificate
- [ ] Team informato e allineato

### Checklist Per Fase

**Ogni fase deve completare:**

- [ ] Package creato con struttura corretta
- [ ] Codice scritto seguendo Go best practices
- [ ] Unit test scritti (coverage >= 80%)
- [ ] Integration test funzionanti
- [ ] Documentazione godoc completa
- [ ] Performance benchmark eseguiti
- [ ] Side-by-side test vs bash passati
- [ ] Code review completata
- [ ] Integrazione in orchestrator
- [ ] README/docs aggiornati

### Checklist Pre-Release

- [ ] Tutti i test passano
- [ ] Performance >= versione bash
- [ ] Documentazione completa
- [ ] Migration guide scritta
- [ ] Rollback plan testato
- [ ] Monitoring configurato
- [ ] Alerts configurati
- [ ] Team training completato

---

## 📚 Best Practices

### Coding Standards

1. **Segui Go conventions:**
   - `gofmt` per formattazione
   - `golint` per linting
   - `go vet` per static analysis

2. **Error handling:**
   ```go
   // ✅ GOOD: Error wrapping con context
   if err != nil {
       return fmt.Errorf("operation failed: %w", err)
   }

   // ❌ BAD: Error senza context
   if err != nil {
       return err
   }
   ```

3. **Logging:**
   ```go
   // ✅ GOOD: Structured logging
   logger.Info("Backup completed",
       zap.Duration("duration", d),
       zap.Int64("size", size))

   // ❌ BAD: String concatenation
   logger.Info("Backup completed in " + d.String())
   ```

4. **Context propagation:**
   ```go
   // ✅ GOOD: Context passato ovunque
   func DoBackup(ctx context.Context) error {
       return collector.Collect(ctx)
   }
   ```

### Strategia di Parità Funzionale

- Mantieni un harness in `test/parity/` che esegue in parallelo pipeline Bash e Go sugli stessi input, collezionando log, artefatti e checksum.
- Normalizza l'output (es. timestampe, path temporanei) e confronta automaticamente i risultati: qualsiasi delta non previsto blocca la fase.
- Traccia i golden results in repository e aggiorna i file solo via pull request con approvazione del Tech Lead.
- Integra il parity testing in CI con job notturni e target manuale (`PARITY=1 make test-parity`) prima di promuovere una release.
- Estendi il harness con profili di carico (backup grandi, incrementali, error path) per anticipare regressioni funzionali e prestazionali.

### Testing Best Practices

1. **Table-driven tests:**
   ```go
   func TestFormatBytes(t *testing.T) {
       tests := []struct {
           name     string
           input    int64
           expected string
       }{
           {"zero", 0, "0 B"},
           {"bytes", 500, "500 B"},
           {"kilobytes", 1024, "1.0 KiB"},
       }

       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               got := FormatBytes(tt.input)
               if got != tt.expected {
                   t.Errorf("got %s, want %s", got, tt.expected)
               }
           })
       }
   }
   ```

2. **Mocking interfaces:**
   ```go
   type MockStorage struct {
       UploadFunc func(ctx context.Context, src, dst string) error
   }

   func (m *MockStorage) Upload(ctx context.Context, src, dst string) error {
       if m.UploadFunc != nil {
           return m.UploadFunc(ctx, src, dst)
       }
       return nil
   }
   ```

3. **Test helpers:**
   ```go
   func setupTest(t *testing.T) (string, func()) {
       tmpDir, err := os.MkdirTemp("", "test")
       if err != nil {
           t.Fatal(err)
       }

       cleanup := func() {
           os.RemoveAll(tmpDir)
       }

       return tmpDir, cleanup
   }
   ```

### Performance Best Practices

1. **Use buffered I/O:**
   ```go
   // ✅ GOOD: Buffered
   w := bufio.NewWriter(file)
   defer w.Flush()
   ```

2. **Goroutines per parallelismo:**
   ```go
   // ✅ GOOD: errgroup per gestione errori
   g, ctx := errgroup.WithContext(ctx)
   for _, item := range items {
       item := item
       g.Go(func() error {
           return process(ctx, item)
       })
   }
   return g.Wait()
   ```

3. **Context con timeout:**
   ```go
   ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
   defer cancel()
   ```

### Security Best Practices

1. **File permissions:**
   ```go
   // Config files: 0400 (r--------)
   os.WriteFile(path, data, 0400)

   // Directories: 0755 (rwxr-xr-x)
   os.MkdirAll(dir, 0755)
   ```

2. **Secrets management:**
   ```go
   // ✅ GOOD: Non loggare secrets
   logger.Info("Connecting to Telegram") // No token!

   // ❌ BAD:
   logger.Info("Token: " + token) // Never do this!
   ```

3. **Input validation:**
   ```go
   // ✅ GOOD: Validate sempre gli input
   if !filepath.IsAbs(path) {
       return fmt.Errorf("path must be absolute")
   }
   ```

---

## 🚀 Deployment & Operatività

### Packaging & Distribuzione

- Genera pacchetti `.deb`/`.rpm` e, in parallelo, un container image di riferimento per ambienti containerizzati.
- Includi nel pacchetto sia il binario Go sia gli script Bash legacy: un wrapper (`bin/proxmox-backup`) sceglie runtime in base ai feature flag.
- Pubblica artefatti firmati e mantieni SBOM (Syft + attestazioni) per ogni release.

### Esecuzione Controllata

- Aggiorna il servizio systemd (`/etc/systemd/system/proxmox-backup.service`) inserendo variabile `MODE=go|bash` e health check periodici.
- Per i job pianificati (cron), sostituisci l'invocazione diretta dello script con `bin/switch-mode run-backup` che legge i flag correnti.
- Documenta le dipendenze runtime nel runbook (directory, permessi, tool esterni) e verifica con `systemd-analyze verify`.

### Rollout Graduale

1. **Staging** (>= 1 settimana): tutti i job eseguiti in parallelo Bash+Go con confronto automatico.
2. **Canary 10%** (2 settimane): nodi selezionati usano pipeline Go, con fallback automatico se aumentano errori o durate >15%.
3. **Ramp-up 30% → 70% → 100%** (≥ 1 settimana per step): promozione condizionata a metriche verdi e nessun incidente P1/P2.
4. **Stabilizzazione** (2 settimane): monitoraggio intensivo, raccolta feedback operatori, cleanup degli script residuali.

### Monitoring & SLO

- Esporta metriche di durata, successo e throughput sia per Bash sia per Go durante la fase ibrida.
- Definisci SLO: p95 durata backup < SLA attuale, failure rate < 1%. Attiva alerting su differenziali Go-Bash > 10%.
- Condividi dashboard Grafana con team operazioni e definisci check giornaliero durante il rollout.

### Feature Flag Matrix

| Flag | Descrizione | Default | Effetto se disattivo |
|------|-------------|---------|----------------------|
| `enable_go_cli` | CLI Go abilitata per comandi read-only | Off in Fase 0 | CLI punta agli script Bash |
| `enable_go_orchestrator` | Orchestratore Go gestisce la pipeline | Off fino a Fase 2 | Orchestrator Bash rimane attivo |
| `enable_go_backup` | Task di backup eseguiti in Go | Off fino a Fase 4 | Funzioni `backup_*` restano in Bash |
| `enable_go_storage` | Gestione storage e retention in Go | Off fino a Fase 5 | `storage.sh` mantiene il controllo |
| `enable_go_notify` | Sistema notifiche Go | Off fino a Fase 6 | `notify.sh` + `email_relay.sh` usati |
| `enable_go_metrics` | Esportazione metriche Go | Off fino a Fase 6 | `metrics*.sh` restano attivi |

### Rollback Playbook (Estratto)

1. Verifica stato feature flag: `bin/switch-mode status`.
2. Switch immediato: `bin/switch-mode --target=bash` sul nodo interessato.
3. Riavvia servizio: `systemctl restart proxmox-backup` e conferma job successivo.
4. Analizza ultimi due run (log strutturati + metriche) e allega diff al ticket incidente.
5. Aggiorna registro incidenti e pianifica post-mortem/retrospettiva entro 48 ore.

---

## 🧭 Governance & Comunicazione

- **RACI aggiornato:** Tech Lead (R), Product Owner (A), DevOps (C), Operatori e Support (I); pubblicato in `docs/governance/raci.md`.
- **Ritmo operativo:** stand-up bisettimanali, sync tecnico settimanale, steering mensile con stakeholder business.
- **Reporting:** dashboard condivisa (JIRA/Notion) con stato fasi, burn-up chart, incident trend e indicatori SLO.
- **Change management:** ogni fase chiusa richiede retro + lesson learned, aggiornamento changelog e approvazione CAB.
- **Enablement:** training operatori, guida debugging aggiornata, newsletter interna post-release per comunicare cambiamenti.

---

## ⚠️ Sfide e Soluzioni

### Sfida 1: Chiamate a Comandi Esterni

**Problema:** Molto codice bash chiama comandi esterni (`rclone`, `pvesm`, `pct`, ecc.)

**Soluzione:**
```go
// Usa exec.Command con context e timeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
defer cancel()

cmd := exec.CommandContext(ctx, "rclone", "copy", src, dst)
output, err := cmd.CombinedOutput()
```

**Alternative:**
- SDK nativi dove possibile (es. AWS SDK per S3)
- Wrapper Go per comandi comuni
- Interfacce per facilitare testing e mocking

### Sfida 2: String Processing Complesso

**Problema:** Bash eccelle in manipolazione testo, Go richiede più codice

**Soluzione:**
```go
// Usa pacchetti standard library
import (
    "strings"
    "regexp"
    "text/template"
)

// Template per output formattato
tmpl := template.Must(template.New("status").Parse(`
Backup Status: {{.Status}}
Duration: {{.Duration}}
Size: {{.Size}}
`))
```

### Sfida 3: Stato Globale

**Problema:** Bash usa molte variabili globali, Go preferisce dependency injection

**Soluzione:**
```go
// ✅ GOOD: Struct con dipendenze
type Orchestrator struct {
    config  *config.Config
    logger  *logging.Logger
    storage Storage
}

func NewOrchestrator(cfg *config.Config, log *logging.Logger, s Storage) *Orchestrator {
    return &Orchestrator{
        config:  cfg,
        logger:  log,
        storage: s,
    }
}
```

### Sfida 4: Backward Compatibility Configurazione

**Problema:** Utenti hanno configurazioni `.env` esistenti

**Soluzione:**
```go
// Parser che supporta entrambi i formati
func LoadConfig(path string) (*Config, error) {
    // Prova prima YAML
    if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
        return loadYAML(path)
    }

    // Fallback a ENV
    return loadEnv(path)
}

func loadEnv(path string) (*Config, error) {
    // Leggi file .env
    // Converti in struct Config
    // Mantieni compatibilità con vecchi nomi variabili
}
```

### Sfida 5: Testing Operazioni Distruttive

**Problema:** Difficile testare operazioni come delete, rotation

**Soluzione:**
```go
// Usa filesystem virtuale per testing
import "testing/fstest"

func TestRotation(t *testing.T) {
    fs := fstest.MapFS{
        "backup1.tar.gz": &fstest.MapFile{},
        "backup2.tar.gz": &fstest.MapFile{},
    }

    // Test su filesystem virtuale
}
```

### Sfida 6: Performance Regression

**Problema:** Versione Go potrebbe essere più lenta per alcune operazioni

**Soluzione:**
```go
// Benchmark continui
func BenchmarkCompression(b *testing.B) {
    for i := 0; i < b.N; i++ {
        compress(data)
    }
}

// Profiling
import _ "net/http/pprof"
go tool pprof http://localhost:6060/debug/pprof/profile
```

---

## 📖 Riferimenti

### Documentazione Go

- [Go Documentation](https://go.dev/doc/)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Librerie Utili

- [cobra](https://github.com/spf13/cobra) - CLI framework
- [viper](https://github.com/spf13/viper) - Configuration
- [zap](https://github.com/uber-go/zap) - Fast logging
- [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) - Goroutine error handling
- [prometheus/client_golang](https://github.com/prometheus/client_golang) - Metrics

### Tool e Utilities

- `gofmt` - Formattazione codice
- `golint` - Linting
- `go vet` - Static analysis
- `go test -cover` - Test coverage
- `go tool pprof` - Profiling

### Best Practices

- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Go Project Layout](https://github.com/golang-standards/project-layout)

---

## 📝 Note Finali

### Flessibilità del Piano

Questo piano è **flessibile e adattabile**. Non è necessario seguirlo rigidamente:

- ✅ Puoi **cambiare l'ordine** delle fasi se necessario
- ✅ Puoi **saltare moduli** poco prioritari inizialmente
- ✅ Puoi **parallelizzare** lo sviluppo di moduli indipendenti
- ✅ Puoi **fermarti a qualsiasi milestone** e avere comunque un sistema funzionante

### Approccio Incrementale

**Ricorda:** L'obiettivo è avere **sempre un sistema funzionante**:

1. Dopo Fase 2: Hai un binario Go che funziona (chiama bash)
2. Dopo Fase 4: Hai backup operations in Go nativo
3. Dopo Fase 5: Hai storage management completo
4. Dopo Fase 6: Migrazione completa

Puoi **fermarti in qualsiasi momento** e continuare a usare la versione ibrida.

### Supporto e Comunicazione

Durante la migrazione:

- 📝 **Documenta tutto**: Decisioni, problemi, soluzioni
- 🧪 **Testa continuamente**: Non aspettare la fine
- 🔄 **Itera rapidamente**: Small commits, frequent integration
- 💬 **Comunica**: Tieni aggiornato il team sui progressi

### Prossimi Passi

Per iniziare:

1. ✅ Rivedi questo piano e adattalo alle tue necessità
2. ✅ Completa Fase 0 (setup progetto)
3. ✅ Inizia Fase 1 (infrastruttura)
4. ✅ Testa, testa, testa!

---

## 🔧 FAQ e Troubleshooting

### Domande Frequenti

**Q: Posso usare il module path `github.com/tis24dev/proxmox-backup` anche se non ho ancora pubblicato su GitHub?**

A: Sì! Il module path è solo un identificatore. Go usa il filesystem locale per risolvere gli import durante lo sviluppo. Quando pubblicherai su GitHub, tutto funzionerà automaticamente.

**Q: Devo migrare tutto in una volta?**

A: No! L'approccio consigliato è incrementale. Dopo la Fase 2 avrai un binario Go funzionante che chiama ancora gli script bash. Puoi fermarti lì e migrare i moduli uno alla volta quando hai tempo.

**Q: Come testo che la versione Go produca gli stessi risultati del Bash?**

A: Esegui entrambe le versioni in parallelo e confronta:
```bash
# Esegui Bash
./script/proxmox-backup.sh --check > /tmp/bash-output.txt

# Esegui Go
./build/proxmox-backup --check > /tmp/go-output.txt

# Confronta
diff /tmp/bash-output.txt /tmp/go-output.txt
```

**Q: Cosa succede se trovo un bug nella versione Go?**

A: Hai sempre il rollback alla versione Bash. È per questo che manteniamo entrambe le versioni durante la migrazione:
```bash
# Torna a usare Bash
./script/proxmox-backup.sh

# Debug della versione Go
./build/proxmox-backup --debug 5
```

**Q: Quanto spazio su disco serve per sviluppo Go?**

A: Molto poco! Il binario compilato è ~10-20 MB. Durante sviluppo il modulo Go + dependencies occupano ~50-100 MB.

**Q: Posso sviluppare su una macchina diversa dal server Proxmox?**

A: Sì! Puoi sviluppare in locale e compilare per Proxmox:
```bash
# Su macchina di sviluppo
GOOS=linux GOARCH=amd64 go build -o proxmox-backup ./cmd/proxmox-backup

# Copia su Proxmox
scp proxmox-backup root@proxmox-server:/opt/proxmox-backup/build/
```

**Q: Devo imparare Go prima di iniziare?**

A: Consigliato! Risorse veloci:
- [Tour of Go](https://go.dev/tour/) (2-3 ore)
- [Go by Example](https://gobyexample.com/) (esempi pratici)
- [Effective Go](https://go.dev/doc/effective_go) (best practices)

**Q: Come gestisco i segreti (token Telegram, password) durante lo sviluppo?**

A: Mantieni il file `env/backup.env` con permessi 400. Il parser Go leggerà da lì, esattamente come fa bash. Non committare mai segreti in git!

---

### Troubleshooting Comune

**Problema: `go: cannot find main module`**

```bash
# Soluzione: Assicurati di essere nella directory con go.mod
cd /opt/proxmox-backup
ls go.mod  # Deve esistere
```

**Problema: `package xxx is not in GOROOT`**

```bash
# Soluzione: Scarica le dipendenze
go mod download
go mod tidy
```

**Problema: `import cycle not allowed`**

```bash
# Soluzione: Hai un'importazione circolare
# Esempio: package A importa B, e B importa A
# Riorganizza il codice o crea un terzo package per le parti condivise
```

**Problema: Build fallisce con "undefined: xxx"**

```bash
# Soluzione: Manca un import o una funzione non esportata
# Funzioni Go devono iniziare con maiuscola per essere esportate
# BAD:  func myFunc()  // non visibile da altri package
# GOOD: func MyFunc()  // visibile da altri package
```

**Problema: Il binario Go è lento**

```bash
# Soluzione 1: Compila con ottimizzazioni
make build-release  # Usa -ldflags="-s -w"

# Soluzione 2: Profiling
go run ./cmd/proxmox-backup -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

**Problema: Gli import non funzionano in VS Code**

```bash
# Soluzione: Installa Go extension e gopls
# 1. Installa extension: Go (by Go Team at Google)
# 2. Ricarica VS Code
# 3. Esegui: Go: Install/Update Tools
```

**Problema: `permission denied` eseguendo il binario**

```bash
# Soluzione: Aggiungi permesso di esecuzione
chmod +x build/proxmox-backup

# O esegui direttamente con go run
go run ./cmd/proxmox-backup
```

**Problema: Il binario non trova i file di configurazione**

```bash
# Soluzione: Usa path assoluti o verifica working directory
# Nel codice Go:
cfg, err := config.LoadConfig("/opt/proxmox-backup/env/backup.env")

# Per debug:
import "os"
fmt.Println("Working dir:", os.Getwd())
```

---

## 📞 Supporto e Contributi

### Come Contribuire

Questo progetto è open source! Contributi benvenuti:

1. **Report bugs**: Apri issue su GitHub con dettagli e log
2. **Feature requests**: Suggerisci miglioramenti
3. **Pull requests**: Contribuisci codice (segui le convenzioni Go)
4. **Documentazione**: Migliora questo piano o aggiungi esempi

### Risorse Utili

- **Go Documentation**: https://go.dev/doc/
- **Go Packages**: https://pkg.go.dev/
- **Awesome Go**: https://github.com/avelino/awesome-go
- **Go Forum**: https://forum.golangbridge.org/

### Community

- **GitHub Issues**: Per bug e feature request
- **GitHub Discussions**: Per domande e discussioni
- **Pull Requests**: Per contributi codice

---

## 📈 Tracking Progresso

Usa questa tabella per tracciare il tuo progresso:

| Fase | Obiettivo | Status | Data Inizio | Data Fine | Note |
|------|-----------|--------|-------------|-----------|------|
| **0** | Setup iniziale | ⬜ | | | |
| **1** | Infrastruttura Go | ⬜ | | | |
| **2** | Orchestrator ibrido | ⬜ | | | |
| **3** | Environment & Core | ⬜ | | | |
| **4** | Backup Operations | ⬜ | | | |
| **5** | Storage Management | ⬜ | | | |
| **6** | Notifications & Metrics | ⬜ | | | |

**Legenda:**
- ⬜ Non iniziato
- 🟨 In corso
- ✅ Completato
- ⚠️ Bloccato

### Milestone Tracking

```bash
# Crea file per tracking (opzionale)
cat > .migration-progress << 'EOF'
CURRENT_PHASE=0
PHASE_0_STATUS=not_started
PHASE_1_STATUS=not_started
# ... etc
EOF

# Aggiorna quando completi una fase
sed -i 's/PHASE_0_STATUS=not_started/PHASE_0_STATUS=completed/' .migration-progress
```

---

**Buona migrazione! 🚀**

*Ultimo aggiornamento: 2025-11-05*
*Versione Piano: 1.0*
