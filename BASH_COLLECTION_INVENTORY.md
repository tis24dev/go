# Bash Backup Collection Inventory (dettagliato)

Documento di riferimento per ricostruire con precisione tutto ciò che lo script legacy `reference/script/proxmox-backup.sh` raccoglie prima di generare l’archivio. La ricognizione incrocia:

- `reference/lib/backup_collect.sh`
- `reference/lib/backup_collect_pbspve.sh`
- `reference/lib/backup_create.sh`
- `reference/lib/metrics_collect.sh`, `security.sh`, `storage.sh`

L’obiettivo è fornire una checklist strutturata per garantire parità funzionale con la pipeline Go.

---

## 0. Bootstrap, rilevamento e preparazione

| Aspetto | Comportamento Bash | File/Funzione |
| --- | --- | --- |
| Individuazione percorsi | `SCRIPT_DIR`, `BASE_DIR` dedotti via `readlink -f`; `BASE_DIR` contiene repository script, log e backup. | `reference/script/proxmox-backup.sh` righe 8-25 |
| Configurazione | Caricamento `env/backup.env`, poi `set -euo pipefail`. | `reference/script/proxmox-backup.sh` righe 41-90 |
| Logger bootstrap | Ogni messaggio prima dell’avvio del logger principale viene accodato in `BOOTSTRAP_LOG_FILE` (`/tmp/proxmox-backup-bootstrap-*.log`). | `reference/script/proxmox-backup.sh` righe 28-78 |
| Rilevamento PVE/PBS | `detect_system_type` (in `environment.sh`) usa comandi `pveversion`, `proxmox-backup-manager`, flag `.pve-ignore.*` e crea file di debug se fallisce. | `reference/lib/environment.sh` |
| Cartelle base | `ensure_directories` crea/valida `BACKUP_PATH`, `SECONDARY_BACKUP_PATH`, `CLOUD_BACKUP_PATH`, `LOG_PATH`, `LOCK_PATH` e `.temp`. | `reference/lib/core.sh` |
| Esclusioni automatiche | Ogni percorso backup/log/lock viene aggiunto agli exclude per evitare auto-ricorsione. | `reference/lib/backup_collect.sh` (`collect_backup_data`) |

---

## 1. Raccolta Proxmox VE (`collect_pve_configs`)

### 1.1 Strutture filesystem
- `/etc/pve/` completo (con `nodes/<node>/{qemu-server,lxc}`, ACL, `priv/`, `jobs.cfg`, `replication.cfg`, `backup/`, `firewall/`, `ceph/`).
- `/var/lib/pve-cluster/{config.db,data.mdb,journal}` e `corosync.conf` quando il nodo è in cluster (`is_pve_cluster_configured`).
- `VZDUMP_CONFIG_PATH` (`/etc/vzdump.conf`) e directory `vzdump.cron`.
- File Ceph (`/etc/pve/ceph.conf`, keyrings) quando configurati.

### 1.2 Output comandi/API (in `commands/pve/`)
- `pveversion -v`, `pvenode config`, `pvecm status`, `pvecm nodes`.
- `pvesh get /version`, `/nodes`, `/nodes/<node>/qemu`, `/nodes/<node>/lxc`, `/nodes/<node>/tasks --typefilter=vzdump`.
- `pvesm status --verbose` (testo + JSON), `pvesh get /cluster/backup`, `/cluster/ha/status`.
- `qm list`, `pct list`, `qm status <id>`, `pct status <id>` per ogni VM/CT trovata.
- Job e scheduling: `crontab -l`, `systemctl list-timers --all`, `pvesh get /cluster/ceph/*` dove applicabile.

### 1.3 Controlli e flag
- Ogni blocco è protetto da flag ENV (`BACKUP_CLUSTER_CONFIG`, `BACKUP_PVE_FIREWALL`, `BACKUP_VM_CONFIGS`, `BACKUP_VZDUMP_CONFIG`) e usa `safe_copy`/`handle_collection_error`.
- I file sensibili (es. `priv/`, `ceph/keys`) rispettano la logica “include se presente, altrimenti warning debug”.

---

## 2. Raccolta Proxmox Backup Server (`collect_pbs_configs`)

### 2.1 Configurazioni statiche
- `/etc/proxmox-backup/` completo: `datastore.cfg`, `remote.cfg`, `sync.cfg`, `verification.cfg`, `tape.cfg`, `media-pool.cfg`, `prune.cfg`, `disk.cfg`, `traffic-control.cfg`, `network.cfg`.
- `/usr/lib/systemd/system/proxmox-backup*` e override locali (se esistono).
- Metadati PXAR: `/var/lib/proxmox-backup/{pxar_metadata,selected_pxar,small_pxar,task-log-cache}`.

### 2.2 Comandi manager
- `proxmox-backup-manager version`, `node config`, `datastore list/status/show`.
- `proxmox-backup-manager user/acl/token`, `remote list`, `sync-job list`, `verification-job list`, `prune-job list`, `task list --limit`.
- Tape: `proxmox-tape drive/changer/pool`.

### 2.3 Datastore & PXAR discovery
- `detect_all_datastores` combina auto-detect API, config file e path standard (`/var/lib/proxmox-backup/datastore`, `/var/lib/vz/dump`, `/mnt/pve`), registrando commento e provenienza.

---

## 3. Snapshot di sistema comune (`collect_system_info` e affini)

### 3.1 File copiati
- **Rete**: `/etc/network/interfaces`, `interfaces.d`, `/etc/hosts`, `/etc/resolv.conf`, `/etc/timezone`, `/etc/hostname`.
- **APT**: intera `/etc/apt/` (sources, preferences, keyrings, auth, listchanges, apt.conf*, trusted.gpg.d).
- **Cron**: `/etc/crontab`, `/etc/cron.{d,daily,weekly,monthly,hourly}`, `/var/spool/cron/crontabs`.
- **Systemd**: `/etc/systemd/system/` + drop-in `.d`, override timer, services locali.
- **Firewall & kernel**: `/etc/iptables/`, `/etc/nftables.conf`, `/etc/nftables.d`, `/etc/sysctl.conf`, `/etc/sysctl.d`, `/etc/modules`, `/etc/modprobe.d`.
- **Logrotate/rsyslog**: `/etc/logrotate.conf`, `/etc/logrotate.d`, `/etc/rsyslog.conf`, `/etc/rsyslog.d`.
- **SSL**: `/etc/ssl/certs`, `/etc/ssl/private`, `**/openssl.cnf`, certificati PVE/PBS (`pve-root-ca.*`, `pveproxy-ssl.*`, `pbs*.pem`).

### 3.2 Output comandi (in `var/lib/proxmox-backup-info/`)
- Rete: `ip addr`, `ip route`, `ip -s link`.
- Firewall runtime: `iptables-save`, `ip6tables-save`, `nft list ruleset`.
- Sistema: `uname -a`, `/etc/os-release`, `hostnamectl`.
- Storage: `df -h`, `lsblk -f`, `mount`, `findmnt --all --json`.
- CPU/RAM: `lscpu`, `free -h`.
- Hardware: `lspci -vv`, `lsusb`, `dmidecode`, `sensors`, `smartctl -a /dev/<disk>`.
- Pacchetti: `dpkg -l`, `apt-cache policy`, `apt-mark showmanual`.
- Servizi: `systemctl list-units --type=service --all`, `systemctl list-unit-files`, `journalctl --list-boots`.
- Filesystem avanzati (dietro flag):
  - ZFS (`BACKUP_ZFS_CONFIG`): `zpool status/list`, `zfs list`, `zfs get all`.
  - LVM: `pvs`, `vgs`, `lvs`.
  - Ceph: `ceph status`, `ceph osd tree`.

### 3.3 Metadati generali
- `backup_metadata.txt`: VERSION, BACKUP_TYPE (`pbs`/`pve`/`hybrid`), hostname (`hostname -f`), timestamp ISO, feature flag (chunking/dedup/prefilter/cloud/upload eseguiti).

---

## 4. Sicurezza, utenti e artefatti locali

### 4.1 File critici (`collect_critical_files`)
- Copia controllata di `/etc/passwd`, `/etc/shadow`, `/etc/group`, `/etc/gshadow`, `/etc/sudoers`, `/etc/sudoers.d`, `/etc/fstab`, `/etc/issue`, `/etc/pam.d`.
- Uso di `find` con esclusioni stringenti (pattern proibiti: `.cursor`, `.vscode`, `node_modules`, ecc.) per evitare leak di directory inutili.

### 4.2 Utente root
- File: `.bashrc`, `.profile`, `.bash_history*`, `.lesshst`, `.selected_editor`, `.forward`, `pkg-list.txt`, `test-cron.log`, `authorized_keys`, `known_hosts`, `wget-hsts`.
- Directory: `.ssh/`, `.config/` (incluso `.config/.wrangler/logs/*.log`), `go/`, `my-worker/`, altri workspace.

### 4.3 `/home/<utente>`
- Per ogni utente reale: copia sia dei file “top-level” sia delle directory (anche nascoste), compresi eventuali binari custom (`~/rsync`, `~/rsync-ssl`) e `.ssh`.

### 4.4 Chiavi e certificati
- `/etc/ssh/ssh_host_*`, `/root/.ssh`, `/home/*/.ssh`.
- Certificati TLS PVE/PBS, CA locali e key private (protette da permessi 0640) se disponibili.

---

## 5. Componenti opzionali e percorsi custom

| Feature | Variabile ENV | Cosa include |
| --- | --- | --- |
| Script repository | `BACKUP_SCRIPT_DIR` | Copia `BASE_DIR` in `script-repository/<basename>` escludendo `backup/`, `log/`, `.git`, `.venv`, cache di lingua. |
| Percorsi extra | `CUSTOM_BACKUP_PATHS` | Lista separata da `;` di file/dir aggiuntivi, copiati integralmente salvo blacklist globale (`BACKUP_BLACKLIST`). |
| Prefilter | `ENABLE_PREFILTER` / `PREFILTER_PATTERNS` | Rimozione preventiva di file temporanei, swap, cache docker. |
| Dedup | `ENABLE_DEDUPLICATION` | Hardlink dei duplicati nello staging per risparmiare spazio. |
| Smart chunking | `ENABLE_SMART_CHUNKING`, `CHUNK_SIZE_MB` | `split` dei file grandi e registrazione in `chunked_files/`. |
| Chunk toggles | `ENABLE_CHUNKING`, `ENABLE_DEDUP_CHUNKING` | Attivano pipeline chunk/dedup nel modulo di creazione archivio. |

---

## 6. Generazione dell’archivio (`backup_create.sh`)

1. **Packaging**: `create_backup_archive` costruisce `BACKUP_FILE` (`HOST-TYPE-YYYYMMDD-HHMMSS.tar`) e prepara i file `.manifest`.
2. **Compressione**: supporto `none`, `gzip` (`-9 -n`), `xz` (`-T0 -q`), `zstd` (`-T0 --long=27`). Gestione errori dedicata (`handle_compression_failure`).
3. **Prefilter/Dedup/Chunking**: eseguiti in sequenza se abilitati per ridurre dimensioni e favorire backup incrementali.
4. **Verifica**: `verify_backup` riestrae dall’archivio, confronta checksum (`checksums.sha256`) e scrive `verify.log`.
5. **Statistiche/JSON**: `SaveStatsReport` produce JSON con `filesProcessed`, `dirsCreated`, `dataCollectedBytes`, `compressionRatio`, `archivePath`.

---

## 7. Artefatti runtime e diagnostica

- **Bootstrap log**: file temporaneo con tutti i log pre-inizializzazione, poi inglobato nel log principale.
- **Detection debug**: se la detection PVE/PBS fallisce, viene creato `/tmp/proxmox_detection_debug_<ts>.log` con comandi e output.
- **Export debug**: `export_failure_state` salva log, JSON e bootstrap in `backup-debug/<timestamp>` quando una fase critica fallisce.
- **Metriche Prometheus**: se `PROMETHEUS_ENABLED=true`, `metrics_collect.sh` scrive `backup_operation_duration_seconds` e contatori file.
- **Cleanup esteso**: `cleanup_temp_resources` rimuove temp dir, lock orfani, chunk residui, file metrics se non necessari.

---

## 8. Condizioni d’errore e exit code

| Scenario | Gestione | Exit code |
| --- | --- | --- |
| Controllo spazio dischi | `check_disk_space` valuta `PRIMARY/SECONDARY/CLOUD` (solo se `*_ENABLED=true`), rispetta soglie minime dedicate. | `EXIT_ERROR` |
| Lock già presente/impossibile da pulire | `acquire_lock` → fallimento critico. | `EXIT_ERROR` |
| Collettori | `handle_collection_error` differenzia `warning` vs `critical`; log numerati. | Warning → `EXIT_WARNING`, Critical → `EXIT_ERROR` |
| Tar/compressione | Errori in `create_backup_archive` o `compress_archive` propagano `EXIT_ERROR`. | `EXIT_ERROR` |
| Verifica | `verify_backup` fallito marca lo step come KO e restituisce `EXIT_ERROR`. | `EXIT_ERROR` |
| Upload secondario/cloud | Se `ALLOW_PARTIAL_UPLOAD=true`, l’errore resta warning; altrimenti `EXIT_ERROR`. | Warning/Errore |

---

## 9. Checklist di parità (Go vs Bash)

1. **Auto-esclusione directory**: i path di backup/log/lock (primario, secondario, cloud) non devono mai apparire nell’archivio, nemmeno come directory vuote.
2. **/root e /home**: devono comparire sia file che directory (es. `rsync`, `rsync-ssl`, `.config/.wrangler/logs`).
3. **`var/lib/proxmox-backup-info`**:
   - Presenza di `iptables.txt`, `ip6tables.txt`, `zfs_get_all.txt`, `lspci.txt` (verbose), `lsblk.txt` completo, `backup_metadata.txt` con stesso formato.
4. **Configurazioni PVE/PBS**: tutte le directory e gli output `pvesh` / `proxmox-backup-manager` elencati sopra devono essere replicati.
5. **SSL**: `etc/ssl/openssl.cnf` va copiato sempre.
6. **PXAR**: `selected_pxar/` e `small_pxar/` devono essere creati anche se i datastore non contengono file.
7. **Chunk/dedup/prefilter**: gli stessi flag devono riflettersi nel JSON di statistica e nel log CLI.
8. **Diagnostica**: pipeline Go deve produrre bootstrap log e pacchetto debug equivalenti per garantire supportabilità.

Questa matrice consente di validare rapidamente se la pipeline Go copre tutto ciò che il Bash colleziona nelle fasi 0‑4, evitando regressioni prima di procedere alle fasi successive.
