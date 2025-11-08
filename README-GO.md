# Proxmox Backup - Go Version

> Enterprise-grade backup system for Proxmox VE/PBS, reimplemented in Go.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
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
- ✅ Telegram & Email notifications
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

---

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or higher
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

---

## 📚 Documentation

- **[MIGRATION_PLAN.md](MIGRATION_PLAN.md)** - Comprehensive migration guide (Italian)
- **Code Documentation** - Run `godoc -http=:6060` and visit http://localhost:6060

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

### Phase 4: Backup Operations (4-6 weeks)
- Data collection
- Archive creation & compression
- Integrity verification

### Phase 5: Storage Management (4-6 weeks)
- Multi-storage support
- Cloud integration (rclone)
- Retention policies
- Parallel uploads

### Phase 6: Notifications & Metrics (2-3 weeks)
- Telegram notifications
- Email notifications
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
- [ ] **Phase 0**: Project setup (In Progress)
- [ ] **Phase 1**: Core infrastructure
- [ ] **Phase 2**: Hybrid orchestrator
- [ ] **Phase 3**: Environment detection
- [ ] **Phase 4**: Backup operations
- [ ] **Phase 5**: Storage management
- [ ] **Phase 6**: Notifications & metrics
- [ ] Performance benchmarks
- [ ] Complete test coverage (>80%)
- [ ] Documentation (godoc)
- [ ] First stable release (v1.0.0)

---

**Last updated**: 2025-11-05
**Version**: 0.1.0-dev
