package ops

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/config"
)

// backupPrefix marks the files this service owns.
//
// Listing, rotation and deletion all filter on it, and deletion treats it as a
// safety check, so the three must agree: a drifting literal here would let one
// of them act on a file the others do not consider a backup.
const backupPrefix = "sqlflow-"

// BackupInfo represents a single backup file's metadata.
type BackupInfo struct {
	Filename   string    `json:"filename"`
	Filepath   string    `json:"filepath"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
	Compressed bool      `json:"compressed"`
}

// BackupService dumps the platform database on a schedule, with rotation and
// optional compression.
//
// It holds a DSN rather than a database handle: pg_dump opens its own
// connection, and taking a *db.DB would suggest the dump goes through the pool
// when it does not.
type BackupService struct {
	mu     sync.Mutex
	dsn    string
	cfg    config.BackupConfig
	cancel context.CancelFunc
	done   chan struct{}
}

// NewBackupService creates a new BackupService.
func NewBackupService(dsn string, cfg config.BackupConfig) *BackupService {
	return &BackupService{
		dsn: dsn,
		cfg: cfg,
	}
}

// Start begins the automatic backup scheduler.
// It runs in a background goroutine and can be stopped via Stop().
func (s *BackupService) Start() {
	if !s.cfg.Enabled {
		log.Println("[INFO] backup service is disabled by config")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()

		log.Printf("[INFO] backup scheduler started: interval=%s, dir=%s, keep=%d, compress=%v",
			s.cfg.Interval, s.cfg.Dir, s.cfg.Keep, s.cfg.Compress)

		// Run an initial backup on startup after a short delay
		select {
		case <-time.After(5 * time.Second):
			if err := s.RunBackup(); err != nil {
				log.Printf("[ERROR] initial backup failed: %v", err)
			}
		case <-ctx.Done():
			return
		}

		for {
			select {
			case <-ticker.C:
				if err := s.RunBackup(); err != nil {
					log.Printf("[ERROR] scheduled backup failed: %v", err)
				}
			case <-ctx.Done():
				log.Println("[INFO] backup scheduler stopped")
				return
			}
		}
	}()
}

// Stop gracefully stops the backup scheduler.
func (s *BackupService) Stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.done != nil {
		<-s.done
		s.done = nil
	}
}

// RunBackup performs a single backup operation.
func (s *BackupService) RunBackup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.cfg.Dir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	// Millisecond precision, not seconds: a manual trigger landing in the same
	// second as the scheduled one produced the same name, and the second run
	// silently overwrote the first backup.
	ts := time.Now().UTC().Format("20060102-150405.000")
	filename := backupPrefix + ts + ".sql"
	if s.cfg.Compress {
		filename += ".gz"
	}
	destPath := filepath.Join(s.cfg.Dir, filename)

	// O_EXCL, so a name collision is an error rather than a destroyed backup.
	// The file is created here rather than inside dump() so that the cleanup
	// below can only ever remove a file this call made: a failure to create
	// means one already existed, and deleting that would be the very loss the
	// exclusive open exists to prevent.
	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}

	if err := s.dump(destFile, destPath); err != nil {
		_ = destFile.Close()
		// A partial dump is worse than none: it looks like a backup and would
		// restore into a truncated database.
		_ = os.Remove(destPath)
		return fmt.Errorf("pg_dump: %w", err)
	}
	if err := destFile.Close(); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("close backup file: %w", err)
	}

	if err := s.rotate(); err != nil {
		log.Printf("[WARN] backup rotation failed: %v", err)
	}

	log.Printf("[INFO] backup created successfully: %s", filename)
	return nil
}

// dump streams pg_dump's output into destPath, gzipping it on the way when
// compression is enabled.
//
// Streaming rather than dumping to a file and compressing it afterwards: the
// platform database is the one an operator restores from after losing a disk,
// and the two-step form needs room for the uncompressed copy at the moment
// disk space is most likely to be the problem.
func (s *BackupService) dump(destFile *os.File, destPath string) error {
	conn, err := parsePostgresDSN(s.dsn)
	if err != nil {
		return err
	}

	args := []string{
		"--host=" + conn.host,
		"--port=" + conn.port,
		"--username=" + conn.user,
		"--dbname=" + conn.database,
		"--format=plain",
		// Without this a dump taken while a query is running can capture a
		// state no single transaction ever saw.
		"--serializable-deferrable",
	}
	for _, schema := range conn.schemas {
		// The platform can be deployed into a named schema; dumping the whole
		// database would then also carry whatever else shares it.
		args = append(args, "--schema="+schema)
	}

	cmd := exec.Command("pg_dump", args...)
	// The password goes in the environment, never in the argument list: argv is
	// readable by every user on the host through ps, while a process's
	// environment is not.
	cmd.Env = append(os.Environ(), "PGPASSWORD="+conn.password)
	if conn.sslMode != "" {
		cmd.Env = append(cmd.Env, "PGSSLMODE="+conn.sslMode)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pg_dump: %w", err)
	}

	var writeErr error
	if s.cfg.Compress {
		gz := gzip.NewWriter(destFile)
		gz.Name = filepath.Base(strings.TrimSuffix(destPath, ".gz"))
		_, writeErr = io.Copy(gz, stdout)
		if closeErr := gz.Close(); writeErr == nil {
			writeErr = closeErr
		}
	} else {
		_, writeErr = io.Copy(destFile, stdout)
	}

	// Wait runs regardless: skipping it on a write error would leave the child
	// as a zombie and hide the reason pg_dump gave up.
	waitErr := cmd.Wait()
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if writeErr != nil {
		return fmt.Errorf("write backup: %w", writeErr)
	}
	return destFile.Sync()
}

// rotate removes old backup files, keeping only the most recent N.
func (s *BackupService) rotate() error {
	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		return err
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, backupPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			Filename:   name,
			Filepath:   filepath.Join(s.cfg.Dir, name),
			Size:       info.Size(),
			CreatedAt:  info.ModTime(),
			Compressed: strings.HasSuffix(name, ".gz"),
		})
	}

	// Sort by creation time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	// Remove backups exceeding the keep limit
	if len(backups) > s.cfg.Keep {
		for _, b := range backups[s.cfg.Keep:] {
			if err := os.Remove(b.Filepath); err != nil {
				log.Printf("[WARN] failed to remove old backup %s: %v", b.Filename, err)
			} else {
				log.Printf("[INFO] removed old backup: %s", b.Filename)
			}
		}
	}

	return nil
}

// ListBackups returns information about all existing backup files.
func (s *BackupService) ListBackups() ([]BackupInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, backupPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			Filename:   name,
			Filepath:   filepath.Join(s.cfg.Dir, name),
			Size:       info.Size(),
			CreatedAt:  info.ModTime(),
			Compressed: strings.HasSuffix(name, ".gz"),
		})
	}

	// Sort by creation time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// DeleteBackup removes a specific backup file by filename.
func (s *BackupService) DeleteBackup(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate filename to prevent path traversal
	if !strings.HasPrefix(filename, backupPrefix) {
		return fmt.Errorf("invalid backup filename")
	}
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("invalid backup filename: path separators not allowed")
	}

	filePath := filepath.Join(s.cfg.Dir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", filename)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}

	log.Printf("[INFO] backup deleted: %s", filename)
	return nil
}

// BackupDir returns the configured backup directory path.
func (s *BackupService) BackupDir() string {
	return s.cfg.Dir
}
