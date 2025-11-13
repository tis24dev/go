# Fix: Ctrl+C Hanging During AGE Encryption Setup

## Problem Description

When pressing Ctrl+C during the interactive AGE encryption setup menu, the program would:
1. Print "Received signal interrupt, initiating graceful shutdown..."
2. Hang indefinitely without exiting
3. Require SIGKILL to terminate

### Root Cause

The global signal handler in `main.go` was designed for graceful backup cancellation:
- It cancels the context
- It closes stdin to interrupt blocking I/O operations

However, during interactive setup:
- The context-aware input reading spawns a goroutine
- When stdin is closed, the goroutine remains blocked on `bufio.Reader.ReadString('\n')`
- The goroutine becomes orphaned (channel listener exits on context cancellation)
- Go runtime waits for all goroutines to complete before exiting
- The program hangs waiting for the orphaned goroutine

## Solution Implemented

### Local Signal Handler During Interactive Setup

The fix adds a **local SIGINT handler** that is active only during the AGE setup wizard phase.

**File Modified**: `/opt/proxmox-backup-go/internal/orchestrator/encryption.go`

**Changes Made**:

1. **Added imports** (lines 10, 13):
   ```go
   "os/signal"
   "syscall"
   ```

2. **Local context and signal handler in `runAgeSetupWizard`** (lines 108-126):
   ```go
   // Create a child context for the wizard to handle Ctrl+C locally
   wizardCtx, wizardCancel := context.WithCancel(ctx)
   defer wizardCancel()

   // Register local SIGINT handler for wizard - treat Ctrl+C as "Exit setup" (option 3)
   sigChan := make(chan os.Signal, 1)
   signal.Notify(sigChan, syscall.SIGINT)
   defer signal.Stop(sigChan) // Cleanup: restore normal signal handling after wizard

   // Handle SIGINT as "exit wizard" instead of "graceful shutdown"
   go func() {
       select {
       case <-sigChan:
           fmt.Println("\n^C detected - exiting setup...")
           wizardCancel()
       case <-wizardCtx.Done():
           // Wizard completed normally or parent context cancelled
       }
   }()
   ```

3. **Updated all wizard functions to use `wizardCtx`** instead of `ctx`:
   - `promptYesNo(wizardCtx, ...)` (line 131)
   - `promptOption(wizardCtx, ...)` (line 148)
   - `promptPublicRecipient(wizardCtx, ...)` (line 159)
   - `promptPrivateKeyRecipient(wizardCtx)` (line 161)
   - `promptYesNo(wizardCtx, ...)` (line 169)

4. **Enhanced `mapInputError` function** (lines 356-370):
   Added detection for more stdin closure scenarios:
   ```go
   errStr := strings.ToLower(err.Error())
   if strings.Contains(errStr, "use of closed file") ||
       strings.Contains(errStr, "bad file descriptor") ||
       strings.Contains(errStr, "file already closed") {
       return ErrAgeRecipientSetupAborted
   }
   ```

## How It Works

### During Interactive Setup Phase

1. User runs program, encryption setup wizard starts
2. Local SIGINT handler is registered (catches Ctrl+C first)
3. User presses Ctrl+C at the menu
4. **Local handler** catches SIGINT before the global one
5. Prints "^C detected - exiting setup..."
6. Cancels `wizardCtx` (NOT stdin closure)
7. Input reading functions receive context cancellation
8. Returns `ErrAgeRecipientSetupAborted`
9. `defer signal.Stop(sigChan)` removes local handler
10. Program exits cleanly with appropriate error message

### During Backup Execution Phase

1. Wizard completes (or was skipped)
2. Local signal handler is removed (via `defer`)
3. **Global handler** in `main.go` handles SIGINT
4. Provides graceful shutdown for running backups
5. Normal cancellation behavior preserved

## Key Benefits

1. **Separation of Concerns**: Interactive vs backup phases have different signal handling
2. **No Orphaned Goroutines**: Context cancelled before stdin closure
3. **Natural UX**: Ctrl+C behaves as "exit menu" (same as option 3)
4. **Backward Compatible**: Backup cancellation unchanged
5. **Automatic Cleanup**: `defer` ensures signal handler is always removed

## Testing

### Manual Test

1. Enable encryption without existing recipient file:
   ```bash
   export ENCRYPT_ARCHIVE=true
   export AGE_RECIPIENT_FILE=/tmp/test_recipients.txt
   rm -f /tmp/test_recipients.txt
   ```

2. Run interactively:
   ```bash
   ./build/proxmox-backup -c configs/backup.env
   ```

3. When menu appears, press Ctrl+C

**Expected Result**:
```
[1] Paste existing AGE public recipient
[2] Derive public recipient from AGE private key (not stored)
[3] Exit setup
Select an option [1-3]: ^C
^C detected - exiting setup...
Encryption setup aborted by user. Exiting...
```

**Old Behavior** (before fix):
```
Select an option [1-3]: ^C
Received signal interrupt, initiating graceful shutdown...
[HANGS FOREVER - requires SIGKILL]
```

### Verification Points

- [ ] Program exits immediately after Ctrl+C
- [ ] Shows "^C detected - exiting setup..." message
- [ ] Exit code is appropriate (not 0)
- [ ] No orphaned processes remain
- [ ] Backup execution Ctrl+C still works normally

## Related Files

- `/opt/proxmox-backup-go/internal/orchestrator/encryption.go` (modified)
- `/opt/proxmox-backup-go/cmd/proxmox-backup/main.go` (unchanged - global handler preserved)

## Implementation Stats

- Lines added: ~30
- Lines modified: ~10
- Files changed: 1
- Build status: ✅ Successful
- Backward compatibility: ✅ Maintained

## Future Considerations

This pattern could be applied to other interactive prompts if needed, such as:
- PBS token registration prompts
- Configuration wizards
- Interactive confirmation dialogs

The key principle: **Interactive user input should treat Ctrl+C as "cancel/exit" not "shutdown"**.
