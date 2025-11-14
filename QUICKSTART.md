# 🚀 Quick Start - Migrazione Bash → Go

> Guida rapida per iniziare subito la migrazione. Per dettagli completi vedi [MIGRATION_PLAN.md](MIGRATION_PLAN.md)

---

## ⏱️ Setup in 5 Minuti

### 1. Prerequisites

```bash
# Verifica versione Go
go version  # Deve essere >= 1.21

# Se non installato:
# wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
# sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
# export PATH=$PATH:/usr/local/go/bin
```

### 2. Inizializza Progetto

```bash
cd /opt/proxmox-backup

# Inizializza Go module
go mod init github.com/tis24dev/proxmox-backup

# Crea struttura directory
mkdir -p cmd/proxmox-backup
mkdir -p internal/{config,logging,types,environment}
mkdir -p pkg/{utils,proxmox}
mkdir -p test/{integration,fixtures,mocks}
mkdir -p configs build
```

### 3. Crea File Base

**Makefile:**
```bash
cat > Makefile << 'EOF'
.PHONY: build test clean run

build:
	@echo "Building proxmox-backup..."
	go build -o build/proxmox-backup ./cmd/proxmox-backup

test:
	go test -v ./...

clean:
	rm -rf build/

run:
	go run ./cmd/proxmox-backup

deps:
	go mod tidy
EOF
```

**.gitignore:**
```bash
cat > .gitignore << 'EOF'
build/
*.test
*.out
vendor/
backup/*.tar.gz
log/*.log
lock/*
secure_account/
.DS_Store
EOF
```

### 4. Test Setup

```bash
# Verifica che tutto funzioni
go mod tidy
make help 2>/dev/null || echo "Makefile creato (help target opzionale)"
```

---

## 📝 Primo Codice Go

Crea un main.go minimale per testare:

```bash
cat > cmd/proxmox-backup/main.go << 'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Proxmox Backup - Go Version")
	fmt.Println("Version: 0.1.0-dev")

	// Test lettura file esistente
	if _, err := os.Stat("/opt/proxmox-backup/env/backup.env"); err == nil {
		fmt.Println("✓ Configuration file found")
	} else {
		fmt.Println("✗ Configuration file not found")
	}

	fmt.Println("\nStatus: Setup completed!")
}
EOF
```

Compila e testa:

```bash
make build
./build/proxmox-backup
```

Output atteso:
```
Proxmox Backup - Go Version
Version: 0.1.0-dev
✓ Configuration file found

Status: Setup completed!
```

---

## 🎯 Cosa Fare Dopo

### Fase 0: Setup Completo ✅

- [x] Go installato
- [x] Struttura directory creata
- [x] go.mod inizializzato
- [x] Makefile creato
- [x] Primo build funzionante

### Fase 1: Prossimi Passi

1. **Leggi il piano completo**: [MIGRATION_PLAN.md](MIGRATION_PLAN.md)
2. **Inizia Fase 1**: Implementa package `internal/config`
3. **Testa continuamente**: Confronta output Go vs Bash

---

## 🛠️ Comandi Utili

### Build & Run

```bash
# Build development
make build

# Run senza build
make run

# Build ottimizzato
go build -ldflags="-s -w" -o build/proxmox-backup ./cmd/proxmox-backup
```

### Dependencies

```bash
# Aggiungi dipendenza
go get github.com/spf13/cobra@latest

# Aggiorna go.mod
go mod tidy

# Lista dipendenze
go list -m all
```

### Testing

```bash
# Test tutto
go test ./...

# Test con verbose
go test -v ./...

# Test coverage
go test -cover ./...

# Test specifico package
go test ./internal/config
```

### Git Workflow

```bash
# Commit setup iniziale
git add go.mod Makefile .gitignore cmd/
git commit -m "feat: initialize Go project structure"

# Crea branch per sviluppo
git checkout -b migration-go

# Sviluppa...
git add .
git commit -m "feat: implement config package"
```

---

## 📚 Risorse Utili

### Documentazione Go (se sei nuovo a Go)

- **Impara Go in 30min**: https://go.dev/tour/
- **Go by Example**: https://gobyexample.com/
- **Effective Go**: https://go.dev/doc/effective_go

### Librerie che Useremo

```bash
# CLI framework
go get github.com/spf13/cobra@latest

# Configuration
go get github.com/spf13/viper@latest

# Logging
go get go.uber.org/zap@latest

# YAML parsing
go get gopkg.in/yaml.v3@latest
```

---

## ⚙️ Config avanzata (pipeline Go)

`configs/backup.env` contiene i flag esclusivi della versione Go. I principali:

### Install wizard (`--install`)

Per generare rapidamente il file `configs/backup.env` su un nodo nuovo:

```bash
cd /opt/proxmox-backup-go
make build
./build/proxmox-backup --install
```

Cosa fa il wizard:
1. Salva sempre l’env dentro `<repo>/configs/backup.env` (i percorsi relativi vengono risolti rispetto alla cartella sopra l’eseguibile; niente file finisce in `build/`).
2. Chiede se vuoi abilitare lo storage secondario, lo storage cloud, Telegram, email e la crittografia.
3. Ogni input viene sanitizzato (niente newline o caratteri di controllo) e i commenti inline del template rimangono intatti anche dopo la sostituzione dei valori.
4. Se abiliti la crittografia, al termine parte subito il wizard AGE così la chiave viene generata subito e non rimani in uno stato “a metà”.

Al termine puoi modificare a mano il file generato per tutte le opzioni avanzate.

### PXAR metadata
- `PXAR_SCAN_ENABLE`, `PXAR_STOP_ON_CAP`, `PXAR_SCAN_MAX_ROOTS`, `PXAR_ENUM_READDIR_WORKERS`, `PXAR_ENUM_BUDGET_MS` governano quante directory/file vengono enumerati.
- `PXAR_FILE_INCLUDE_PATTERN` / `PXAR_FILE_EXCLUDE_PATTERN` sono liste (spazi/virgole) di glob per includere o escludere file durante la raccolta.

### Override percorsi
- `PVE_CONFIG_PATH`, `PVE_CLUSTER_PATH`, `COROSYNC_CONFIG_PATH`, `VZDUMP_CONFIG_PATH` puntano ai path reali quando lavori su mirror montati o snapshot offline.
- `PBS_DATASTORE_PATH` accetta più percorsi manuali per includere datastore PBS aggiuntivi oltre a quelli rilevati automaticamente.

### Cloud / rclone
- `CLOUD_REMOTE_PATH` aggiunge un prefisso deterministico dentro al remote rclone (`remote:prefisso/...`).
- `CLOUD_UPLOAD_MODE` (`sequential` | `parallel`), `CLOUD_PARALLEL_MAX_JOBS` e `CLOUD_PARALLEL_VERIFICATION` controllano worker pool e verifiche dei file associati.
- `CLOUD_LOG_PATH` deve contenere remote e path finale (es. `myremote:/logs`); a differenza dei backup, non viene combinato con `CLOUD_REMOTE_PATH`.
- `MAX_*_BACKUPS` si applicano anche alla rotazione dei log (con 1 backup al giorno hai lo stesso numero di log).

### Retention Policy - GFS (Grandfather-Father-Son)

La versione Go supporta **due strategie di retention**:

**1. Simple Retention (default)** - Basata su conteggio:
```bash
MAX_LOCAL_BACKUPS=15        # Mantieni 15 backup più recenti
MAX_SECONDARY_BACKUPS=15
MAX_CLOUD_BACKUPS=15
```

**2. GFS Retention** - Distribuzione temporale intelligente:
```bash
# Si attiva automaticamente quando imposti almeno una variabile RETENTION_*
RETENTION_DAILY=7           # Mantieni ultimi 7 giorni
RETENTION_WEEKLY=4          # Mantieni 4 backup settimanali (1 per settimana ISO)
RETENTION_MONTHLY=12        # Mantieni 12 backup mensili (1 per mese)
RETENTION_YEARLY=3          # Mantieni 3 backup annuali (1 per anno)
```

**Vantaggi GFS:**
- ✅ Migliore copertura storica rispetto al conteggio semplice
- ✅ Distribuzione automatica nel tempo (daily→weekly→monthly→yearly)
- ✅ Numerazione settimana ISO 8601 standard
- ✅ Logging predittivo: mostra cosa verrà eliminato prima della cancellazione
- ✅ Configurabile per destinazione (local/secondary/cloud diversi)

**Esempio output GFS:**
```
✓ Local storage initialized (present 25 backups)
  Policy: GFS (daily=7, weekly=4, monthly=12, yearly=3)
  Total: 25/-
  Daily: 7/7
  Weekly: 4/4
  Monthly: 12/12
  Yearly: 2/3

GFS classification → daily: 7/7, weekly: 4/4, monthly: 12/12, yearly: 2/3, to_delete: 0
```

> ⚠️ **Non** modificare `reference/env/backup.env`: copia `configs/backup.env`, applica le modifiche Go-only e versiona solo quel file.

---

## 🐛 Problemi Comuni

### "go: cannot find main module"

```bash
cd /opt/proxmox-backup  # Assicurati di essere nella root del progetto
```

### "package xxx not found"

```bash
go mod tidy  # Scarica le dipendenze
```

### Build fallisce

```bash
make clean
make build
```

---

## 📂 Struttura Directory (Risultato Finale)

```
/opt/proxmox-backup/
├── go.mod                      ← Go module definition
├── Makefile                    ← Build automation
├── .gitignore                  ← Git ignore
├── MIGRATION_PLAN.md           ← Piano dettagliato
├── QUICKSTART.md               ← Questa guida
├── README-GO.md                ← Overview progetto Go
│
├── script/                     ← Mantieni: Bash esistente
│   ├── proxmox-backup.sh
│   └── lib/
│
├── cmd/                        ← Nuovo: Executables Go
│   └── proxmox-backup/
│       └── main.go
│
├── internal/                   ← Nuovo: Private packages
│   ├── config/
│   ├── logging/
│   └── ...
│
├── pkg/                        ← Nuovo: Public packages
│   └── utils/
│
└── build/                      ← Binari compilati
    └── proxmox-backup
```

---

## ✅ Checklist Setup

- [ ] Go 1.25+ installato (`go version`)
- [ ] Directory progetto: `/opt/proxmox-backup`
- [ ] Backup sistema bash esistente
- [ ] `go mod init` eseguito
- [ ] Struttura directory creata
- [ ] Makefile creato
- [ ] .gitignore creato
- [ ] main.go minimale creato
- [ ] `make build` funziona
- [ ] `./build/proxmox-backup` esegue
- [ ] Git branch `migration-go` creato

---

## 🎓 Prossimo Step: Fase 1

Ora sei pronto per iniziare la **Fase 1: Infrastruttura Fondamentale**

Implementerai:
1. Package `internal/config` - Gestione configurazione
2. Package `internal/logging` - Sistema di logging
3. Package `internal/types` - Exit codes ed error types
4. Package `pkg/utils` - Utility functions

Vedi [MIGRATION_PLAN.md - Fase 1](MIGRATION_PLAN.md#fase-1-infrastruttura-fondamentale) per dettagli.

---

## 💡 Tips

1. **Non fretta**: Migra un modulo alla volta
2. **Testa sempre**: Confronta Go vs Bash ad ogni step
3. **Documenta**: Scrivi godoc per ogni funzione pubblica
4. **Commit frequenti**: Small commits, frequent pushes
5. **Chiedi aiuto**: Usa GitHub Issues per domande

---

**Setup completato! Sei pronto per iniziare la migrazione! 🚀**

*Per domande o problemi, vedi [MIGRATION_PLAN.md - FAQ](MIGRATION_PLAN.md#faq-e-troubleshooting)*
