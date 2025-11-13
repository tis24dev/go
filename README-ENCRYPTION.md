# Encryption Guide

## Overview
The Go pipeline encrypts archives **streaming through AGE**: the tar stream is handed directly to `age.Encrypt`, so no plaintext `.tar` files are ever written to disk. Checksums, manifests and bundle metadata remain in clear text to allow pre‑restore validation, but the data payload (`*.tar.<algo>.age`) is unreadable without the matching AGE identity or passphrase.

Key features:
- Supports multiple AGE recipients (from config, files, or the interactive wizard).
- Deterministic passphrase mode (scrypt‑derived, passphrase never stored).
- Wizard can be re‑run on demand (`./build/proxmox-backup --newkey`).
- `--decrypt` workflow guides the user through bundle selection and key entry.

---

## Requirements
- **AGE recipients** must be provided via one of the supported sources:
  - `AGE_RECIPIENT`/`AGE_RECIPIENT_FILE` in `configs/backup.env`.
  - `identity/age/recipient.txt` (created by the wizard).
- **Dependencies**: handled during the new “dependency check” block (rclone for cloud, sendmail for email, compression tools as configured). Missing AES recipients or AGE binaries trigger warnings and interactive wizard.
- **Secret storage**: KEEP the private key or passphrase **offline**. The server only stores public recipients.

---

## Configure AGE Recipients

### Option 1: Static configuration
Set one or more recipients in `configs/backup.env`:
```
AGE_RECIPIENT="age1...."
AGE_RECIPIENT="age1...."
AGE_RECIPIENT_FILE=/opt/proxmox-backup-go/identity/age/recipient.txt
```
The orchestrator deduplicates and caches them.

### Option 2: Interactive wizard
Triggered automatically when encryption is enabled but no recipients exist, or explicitly via:
```
./build/proxmox-backup --newkey
```
Wizard options (English in script, summary here):
1. **Use existing AGE public key** – paste an `age1…` recipient; stored on disk.
2. **Generate from personal passphrase** – passphrase never stored; deterministic key pair derived by scrypt. The wizard enforces:
   - ≥12 characters.
   - At least 3 of: lowercase, uppercase, digits, symbols.
   - Rejects common passwords (e.g., “password”, “123456”).
3. **Generate from private key** – paste `AGE-SECRET-KEY-1…`; derivation occurs locally, private key is not stored.
4. **Exit setup** – abort.

During the wizard:
- Input is read without echo (`term.ReadPassword`).
- Memory buffers are zeroed as soon as possible (best effort; strings in Go may still leave traces in GC-managed memory, so treat passphrases as sensitive regardless).
- Recipient file (`identity/age/recipient.txt`) is auto-created inside `BASE_DIR` with permissions `0700/0600`.

### Rotating recipients
Run `./build/proxmox-backup --newkey`. The tool:
1. Backs up the existing recipient file (`recipient.txt.bak-YYYYMMDD-HHMMSS`).
2. Launches the wizard again.
3. Updates `AGE_RECIPIENT_FILE` if it was empty.

---

## Running Encrypted Backups
1. Enable encryption in `backup.env`:
   ```
   ENCRYPT_ARCHIVE=true
   ```
2. Provide recipients via config or wizard.
3. Launch backups as usual:
   ```
   ./build/proxmox-backup
   ```
4. Resulting artifact:
   - Archive: `hostname-backup-YYYYMMDD-HHMMSS.tar.<compression>.age`
   - Bundle (if enabled): `...tar.<compression>.age.bundle.tar`
   - Checksums & metadata remain in clear text (`.sha256`, `.metadata`).

The archiver streams tar+compression directly into the AGE writer; no plaintext `.tar` remains on disk. `VerifyArchive` skips deep inspection on encrypted payloads but still checks existence/size.

---

## Decrypting Backups
Use the dedicated workflow:
```
./build/proxmox-backup --decrypt
```
Steps:
1. **Dependency/Security checks** run as usual.
2. **Select source path** (Local, Secondary, Cloud – automatically derived from `backup.env`).
3. Tool scans for bundles/archives, parses their manifests, and shows:
   ```
   [index] YYYY-MM-DD HH:MM:SS • ENCRYPTED / PLAIN • Tool vX.Y.Z • pbs/pve/pbs+pve vVERSION
   ```
4. Choose archive, then specify destination folder (default `./decrypt`).
5. Enter passphrase or `AGE-SECRET-KEY-1…` (input hidden, `0` to exit). The prompt:
   - Repeats on wrong key (non-destructive).
   - Zeroes the typed bytes after each attempt.
6. On success:
   - Produces a fresh bundle: `<name>.tar.<algo>.decrypted.bundle.tar` containing plaintext archive + checksum + metadata.
   - Warns before overwriting existing files.

**Note:** The decrypt workflow is intentionally interactive to avoid unattended key prompts.

---

## Restoring Backups
```
./build/proxmox-backup --restore
```
Workflow:
1. Identical dependency/security checks and bundle discovery as `--decrypt`.
2. Plaintext staging in a secure temp directory (encrypted bundles trigger the same key prompt; plaintext bundles skip it).
3. Restore destination is always `/` (system root). Every file is written back to the same absolute path it had during the backup; root privileges are required.
4. Confirmation: type `RESTORE` to proceed or `0` to abort. The tool logs the selected backup, targets and Proxmox version.
5. Extraction: uses `tar` with the appropriate compression flags (`-xzpf`, `-xJpf`, `--use-compress-program=zstd`, etc.) to apply the archive onto `/`.
6. Cleanup: the staged plaintext bundle is deleted automatically after the restore completes (or when the workflow exits).

**Important:** this is a full in-place restore of the archived configuration data. Ensure the target system is the intended destination and take a manual snapshot before running the command.

---

## Emergency Scenarios & Best Practices

| Scenario | Guidance |
| --- | --- |
| Lost passphrase/private key | There is **no recovery**. Keep at least two offline copies (password manager, printed paper). For passphrase-derived keys, the passphrase *is* the private key. |
| Migrating to a new server | Copy only the `identity/age/recipient.txt` (public). Keep private keys/passphrases off the server. |
| Verifying backup integrity | Periodically run `--decrypt`, choose a recent archive, and confirm it decrypts and matches expected files. |
| Rotating keys | Use `--newkey`. Re-run backups so new archives target the new recipient; keep old identities until legacy archives are no longer needed. |
| Automation | Headless/non-interactive runs must set `AGE_RECIPIENT`/`AGE_RECIPIENT_FILE`. Otherwise the wizard will abort with: `Encryption setup requires interaction...`. |
| Testing | `go test ./...` now includes encryption tests (stream write + decrypt, wrong-key behavior). Run after modifying archiver/encryption code. |

---

## Security Notes
- Passphrases/private keys are read with `term.ReadPassword`; buffers are zeroed immediately after use, but Go strings cannot always be purged—avoid piping secrets into logs or environment variables.
- The dependency preflight reports missing tools upfront (tar, xz, zstd, rclone, sendmail, etc.).
- Streams are encrypted before touching disk; recipients are deduplicated, and the orchestrator caches them per run.
- Keep AGE private keys offline. Store recipient file (`recipient.txt`) with `0700/0600` permissions (auto-enforced during security checks).

---

## Quick Reference
| Action | Command |
| --- | --- |
| Launch wizard (first run or missing recipients) | `./build/proxmox-backup` (encryption enabled, no keys) |
| Force wizard / rotate key | `./build/proxmox-backup --newkey` |
| Run encrypted backup | `./build/proxmox-backup` |
| Decrypt bundle | `./build/proxmox-backup --decrypt` |
| Restore bundle to system | `./build/proxmox-backup --restore` |
| Set recipients via config | `AGE_RECIPIENT="age1..."` in `configs/backup.env` |
| Use passphrase-derived key | Choose wizard option 2; remember the passphrase! |
| Use existing private key | Wizard option 3, paste `AGE-SECRET-KEY-1...` |

Keep this README close to your operational runbooks. Encryption failures are almost always due to missing recipients or lost secrets; proactive checks and documented procedures prevent downtime during restores.
