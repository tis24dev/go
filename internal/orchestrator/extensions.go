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
	for _, target := range o.storageTargets {
		if err := target.Sync(ctx, stats); err != nil {
			return &BackupError{
				Phase: "storage",
				Err:   fmt.Errorf("storage target failed: %w", err),
				Code:  types.ExitStorageError,
			}
		}
	}

	for _, channel := range o.notificationChannels {
		if err := channel.Notify(ctx, stats); err != nil {
			return &BackupError{
				Phase: "notification",
				Err:   fmt.Errorf("notification channel failed: %w", err),
				Code:  types.ExitNetworkError,
			}
		}
	}

	return nil
}
