# Phase 3 – Ottimizzazioni (Revisione 2)

- **Data**: 2025-11-06
- **Responsabile**: Codex (GPT-5)
- **Ambito**: Allineamento finale delle ottimizzazioni richieste per la Fase 3

## Interventi eseguiti

1. **Gestione esplicita degli errori di compressione**  
   - `internal/orchestrator/bash.go`: mappatura degli errori `CompressionError` su `ExitCompressionError` con fase `compression` e codice d’uscita 11.  
   - Effetto: il main termina ora con un codice diverso in caso di failure dei tool esterni (`xz`, `zstd`), evitando di confonderli con errori di creazione tar.

2. **Timestamp manifest e durata backup corretti**  
   - `internal/orchestrator/bash.go`: il manifest usa `time.Now().UTC()` per `CreatedAt` e le statistiche salvano `EndTime` dopo la generazione del manifest.  
   - Effetto: rimozione del timestamp zero nel manifest e durata coerente con l’intera pipeline (checksum + manifest inclusi).

3. **Allineamento compressione richiesta vs effettiva**  
   - `internal/backup/archiver.go`, `internal/orchestrator/bash.go`, `cmd/proxmox-backup/main.go`: introdotto `RequestedCompression`, normalizzazione del livello e fallback automatico a gzip con estensioni corrette.  
   - Effetto: statistiche, manifest e CLI riportano l’algoritmo realmente usato; in assenza di `xz/zstd` il file prodotto è `.tar.gz` e l’utente viene informato del downgrade.

4. **Glob avanzati per gli exclude**  
   - `internal/backup/collector.go`: un’unica normalizzazione del percorso e matcher che gestisce anche pattern `**`, con test dedicati in `collector_test.go`.  
   - Effetto: gli exclude supportano directory annidate (es. `etc/pve/**`, `**/cache/**`) senza penalizzare le prestazioni.

5. **Skeleton post-backup per storage e notifiche**  
   - `internal/orchestrator/extensions.go`: interfacce `StorageTarget`/`NotificationChannel`, registrazione in orchestrator e dispatch con `BackupError` specifici.  
   - Effetto: baseline pronta per la Fase 4 (storage secondario, cloud, alerting) senza impattare l’esecuzione corrente.

6. **Validazione configurazioni future**  
   - `cmd/proxmox-backup/main.go:86-154, 279-305`: nuova `validateFutureFeatures()` che blocca combinazioni incoerenti (es. `SecondaryEnabled` senza path, Telegram senza token/chat).  
   - Effetto: gli utenti ricevono un errore immediato anziché scoprire la misconfigurazione in fasi successive.

7. **Bootstrap cartelle richieste (parità Bash)**  
   - `internal/checks/checks.go`: `CheckDirectories()` ora crea automaticamente `BACKUP_PATH`, `LOG_PATH` e la directory del lock file (con rispettivo log o anteprima in dry-run).  
   - Effetto: il binario Go riproduce il bootstrap dello script Bash senza costringere l’utente a creare le directory manualmente.

8. **Rilevamento Proxmox avanzato**  
   - `internal/environment/detect.go`: replica la catena di fallback dello script Bash (`pveversion`, `proxmox-backup-manager`, file di versione, sources APT, struttura directory), restituendo tipo e versione anche in installazioni non standard.  
   - Effetto: si evita il ritorno a `unknown` in container/chroot e si mantiene la stessa affidabilità del detector originario.

9. **Bootstrap log e cleanup finale**  
   - `internal/logging/bootstrap.go`, `cmd/proxmox-backup/main.go`: cattura l’output preliminare e lo riversa nel logger principale; produce anche i file di debug quando il detector fallisce.  
   - `cmd/proxmox-backup/main.go`: rimuove lock temporanei orfani al termine (coerente con il cleanup Bash).  
   - Effetto: nessuna informazione iniziale va persa e i log raccolgono i dettagli utili per la diagnosi.

## Verifica delle ottimizzazioni richieste

| Voce | Stato | Dettagli principali |
|------|-------|---------------------|
| Exclude pattern | ✅ | `internal/backup/collector.go:52-159` implementa `CollectorConfig.ExcludePatterns`, validazione e matching su percorsi relativi/assoluti. |
| Config validation | ✅ | `CollectorConfig.Validate()` e `ArchiverConfig.Validate()` garantiscono configurazioni coerenti; `cmd/proxmox-backup/main.go:117-146` invoca `checkerConfig.Validate()`. |
| Stima spazio disco | ✅ | `internal/checks/checks.go:311-355` introduce `SafetyFactor` e `CheckDiskSpaceForEstimate`. |
| Temp dir (creazione / cleanup) | ✅ | `RunGoBackup` usa `os.MkdirTemp("", …)` fuori dal backup path con `defer os.RemoveAll`. |
| Streaming archiver | ✅ | `internal/backup/archiver.go:131-236` streamma tar -> `xz`/`zstd` via `io.Pipe`, con fallback elegante. |
| Symlink durante la raccolta | ✅ | `safeCopyFile` ricrea i link simbolici mantenendo i target (`os.Readlink` + `os.Symlink`). |
| Exclude patterns configurabili | ✅ | `config.LoadConfig` popola `Config.ExcludePatterns` da `BACKUP_EXCLUDE_PATTERNS` ed è passato all’orchestrator. |
| Header CLI aggiornato | ✅ | `cmd/proxmox-backup/main.go:47-70` stampa “Phase: 3 - Environment & Core Pipeline”. |
| Ratio nel JSON | ✅ | `SaveStatsReport` esporta `compression_ratio` (valore decimale) e `compression_ratio_percent`. |
| ExitCompressionError | ✅ | Gestito nella nuova logica descritta sopra; `types.ExitCompressionError` = 11. |
| DirsCreated | ✅ | `Collector.ensureDir` incrementa `stats.DirsCreated`; `SaveStatsReport` lo riporta come `directories_created`. |
| Test e2e | ✅ | `internal/orchestrator/go_pipeline_test.go` copre il flusso completo (collect → archive → manifest) + scenario di fallback `xz→gzip`. |
| Skeleton storage/notifiche | ✅ | `internal/orchestrator/extensions.go` definisce interfacce e hook `dispatchPostBackup`, con test mirati (`bash_test.go:218-283`). |
| Config avanzata | ✅ | `cmd/proxmox-backup/main.go:279-305` blocca configurazioni incomplete per storage/notifiche/metriche. |

## Test eseguiti

```bash
cd /opt/proxmox-backup-go
go test ./...
```

**Esito**: ✅ Tutti i pacchetti compilano e i test (unit + e2e orchestrator) passano.

## Note operative

- La pipeline Go ora distingue chiaramente i failure di compressione dagli altri errori di archiviazione.  
- I manifest generati riportano timestamp validi, evitando warning nei consumatori JSON esterni.  
- Il report JSON (`backup-stats-*.json`) contiene valori aggiornati per durata, ratio e checksum, utili per il parity harness della Fase 4.
- Il fallback automatico verso gzip è tracciato nei log/JSON, così automation e monitoring possono adattarsi senza sorprese.  
- Gli exclude pattern supportano wildcard ricorsive (`**`) e non moltiplicano più le `Rel()` per ogni pattern.  
- Gli hook post-backup permettono di innestare rapidamente le integrazioni della Fase 4 (storage secondario, cloud, notifiche) mantenendo separata la responsabilità del core.
