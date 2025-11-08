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

- ✅ Go 1.19.8 installed
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
