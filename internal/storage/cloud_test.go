package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/config"
)

type commandCall struct {
	name string
	args []string
}

type queuedResponse struct {
	name string
	args []string
	out  string
	err  error
}

type commandQueue struct {
	t     *testing.T
	queue []queuedResponse
	calls []commandCall
}

func (q *commandQueue) exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	q.calls = append(q.calls, commandCall{name: name, args: append([]string(nil), args...)})
	if len(q.queue) == 0 {
		q.t.Fatalf("unexpected command: %s %v", name, args)
	}
	resp := q.queue[0]
	q.queue = q.queue[1:]

	if resp.name != "" && resp.name != name {
		q.t.Fatalf("expected command %s, got %s", resp.name, name)
	}
	if resp.args != nil {
		if len(resp.args) != len(args) {
			q.t.Fatalf("expected args %v, got %v", resp.args, args)
		}
		for i := range resp.args {
			if resp.args[i] != args[i] {
				q.t.Fatalf("expected args %v, got %v", resp.args, args)
			}
		}
	}
	return []byte(resp.out), resp.err
}

func newCloudStorageForTest(cfg *config.Config) *CloudStorage {
	cs, _ := NewCloudStorage(cfg, newTestLogger())
	return cs
}

func TestCloudStorageUploadWithRetryEventuallySucceeds(t *testing.T) {
	cfg := &config.Config{
		CloudEnabled:           true,
		CloudRemote:            "remote",
		RcloneRetries:          3,
		RcloneTimeoutOperation: 5,
	}
	cs := newCloudStorageForTest(cfg)

	queue := &commandQueue{
		t: t,
		queue: []queuedResponse{
			{name: "rclone", err: errors.New("copy failed")},
			{name: "rclone", err: errors.New("copy failed again")},
			{name: "rclone", out: "ok"},
		},
	}
	cs.execCommand = queue.exec
	cs.sleep = func(time.Duration) {}

	if err := cs.uploadWithRetry(context.Background(), "/tmp/local.tar", "remote:local.tar"); err != nil {
		t.Fatalf("uploadWithRetry() error = %v", err)
	}
	if len(queue.calls) != 3 {
		t.Fatalf("expected 3 upload attempts, got %d", len(queue.calls))
	}
}

func TestCloudStorageListParsesBackups(t *testing.T) {
	cfg := &config.Config{
		CloudEnabled: true,
		CloudRemote:  "remote",
	}
	cs := newCloudStorageForTest(cfg)
	queue := &commandQueue{
		t: t,
		queue: []queuedResponse{
			{
				name: "rclone",
				args: []string{"lsl", "remote:"},
				out: strings.TrimSpace(`
99999 2024-11-12 12:00:00 host-backup-20241112.tar.zst
12000 2024-11-10 08:00:00 proxmox-backup-legacy.tar.gz
555 random line ignored
`),
			},
		},
	}
	cs.execCommand = queue.exec

	backups, err := cs.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("List() = %d backups, want 2", len(backups))
	}
	if backups[0].BackupFile != "host-backup-20241112.tar.zst" {
		t.Fatalf("expected newest backup first, got %s", backups[0].BackupFile)
	}
	if backups[1].BackupFile != "proxmox-backup-legacy.tar.gz" {
		t.Fatalf("expected legacy backup second, got %s", backups[1].BackupFile)
	}
}

func TestCloudStorageApplyRetentionDeletesOldest(t *testing.T) {
	cfg := &config.Config{
		CloudEnabled:          true,
		CloudRemote:           "remote",
		CloudBatchSize:        1,
		CloudBatchPause:       0,
		BundleAssociatedFiles: false,
	}
	cs := newCloudStorageForTest(cfg)
	cs.sleep = func(time.Duration) {}

	listOutput := strings.TrimSpace(`
100 2024-11-12 10:00:00 gamma-backup-3.tar.zst
100 2024-11-11 10:00:00 beta-backup-2.tar.zst
100 2024-11-10 10:00:00 alpha-backup-1.tar.zst
`)
	recountOutput := strings.TrimSpace(`
100 2024-11-12 10:00:00 gamma-backup-3.tar.zst
100 2024-11-11 10:00:00 beta-backup-2.tar.zst
`)

	queue := &commandQueue{
		t: t,
		queue: []queuedResponse{
			{name: "rclone", args: []string{"lsl", "remote:"}, out: listOutput},
			{name: "rclone", args: []string{"deletefile", "remote:alpha-backup-1.tar.zst"}},
			{name: "rclone", args: []string{"deletefile", "remote:alpha-backup-1.tar.zst.sha256"}},
			{name: "rclone", args: []string{"deletefile", "remote:alpha-backup-1.tar.zst.metadata"}},
			{name: "rclone", args: []string{"deletefile", "remote:alpha-backup-1.tar.zst.metadata.sha256"}},
			{name: "rclone", args: []string{"lsl", "remote:"}, out: recountOutput},
		},
	}
	cs.execCommand = queue.exec

	deleted, err := cs.ApplyRetention(context.Background(), 2)
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("ApplyRetention() deleted = %d, want 1", deleted)
	}
}
