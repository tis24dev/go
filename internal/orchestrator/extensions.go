package orchestrator

import (
	"context"
	"fmt"

	"github.com/tis24dev/proxmox-backup/internal/types"
)

// StorageTarget rappresenta una destinazione esterna (es. storage secondario, cloud).
type StorageTarget interface {
	Sync(ctx context.Context, stats *BackupStats) error
}

// NotificationChannel rappresenta un canale di notifica (es. Telegram, email).
type NotificationChannel interface {
	Notify(ctx context.Context, stats *BackupStats) error
}

// RegisterStorageTarget aggiunge una destinazione da eseguire dopo il backup.
func (o *Orchestrator) RegisterStorageTarget(target StorageTarget) {
	if target == nil {
		return
	}
	o.storageTargets = append(o.storageTargets, target)
}

// RegisterNotificationChannel aggiunge un canale di notifica da eseguire dopo il backup.
func (o *Orchestrator) RegisterNotificationChannel(channel NotificationChannel) {
	if channel == nil {
		return
	}
	o.notificationChannels = append(o.notificationChannels, channel)
}

func (o *Orchestrator) dispatchPostBackup(ctx context.Context, stats *BackupStats) error {
	// Phase 1: Storage operations (critical - failures abort backup)
	for _, target := range o.storageTargets {
		if err := target.Sync(ctx, stats); err != nil {
			return &BackupError{
				Phase: "storage",
				Err:   fmt.Errorf("storage target failed: %w", err),
				Code:  types.ExitStorageError,
			}
		}
	}

	// Phase 2: Notifications (non-critical - failures don't abort backup)
	// Notification errors are logged but never propagated
	if len(o.notificationChannels) > 0 {
		fmt.Println()
		o.logStep(7, "Notifications - dispatching channels")
	}
	for _, channel := range o.notificationChannels {
		_ = channel.Notify(ctx, stats) // Ignore errors - notifications are non-critical
	}

	return nil
}
