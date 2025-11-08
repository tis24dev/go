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

- [ ] Go 1.21+ installato (`go version`)
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
