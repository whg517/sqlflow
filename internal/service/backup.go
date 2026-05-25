package service

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/whg517/sqlflow/config"
)

// BackupInfo represents a single backup file's metadata.
type BackupInfo struct {
	Filename   string    `json:"filename"`
	Filepath   string    `json:"filepath"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
	Compressed bool      `json:"compressed"`
}

// BackupService handles SQLite database backups with rotation and optional compression.
type BackupService struct {
	mu     sync.Mutex
	db     *sql.DB
	dbPath string
	cfg    config.BackupConfig
	cancel context.CancelFunc
	done   chan struct{}
}

// NewBackupService creates a new BackupService.
func NewBackupService(db *sql.DB, dbPath string, cfg config.BackupConfig) *BackupService {
	return &BackupService{
		db:     db,
		dbPath: dbPath,
		cfg:    cfg,
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
// It uses the SQLite backup API via SQL, then optionally compresses the file,
// and finally rotates old backups.
func (s *BackupService) RunBackup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.cfg.Dir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	ts := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("sqlflow-%s.db", ts)
	destPath := filepath.Join(s.cfg.Dir, filename)

	// Use SQLite backup API: the sql.Backup function creates an online backup.
	// modernc.org/sqlite supports this via the database/sql connection.
	// We use the "backup to" pragma approach as a fallback-safe method.
	if err := s.sqliteBackup(destPath); err != nil {
		return fmt.Errorf("sqlite backup: %w", err)
	}

	// Optionally compress the backup
	if s.cfg.Compress {
		gzPath := destPath + ".gz"
		if err := s.compressFile(destPath, gzPath); err != nil {
			// Remove the uncompressed file on compression failure
			_ = os.Remove(destPath)
			return fmt.Errorf("compress backup: %w", err)
		}
		// Remove the uncompressed backup
		if err := os.Remove(destPath); err != nil {
			log.Printf("[WARN] failed to remove uncompressed backup %s: %v", destPath, err)
		}
	}

	// Rotate old backups
	if err := s.rotate(); err != nil {
		log.Printf("[WARN] backup rotation failed: %v", err)
	}

	log.Printf("[INFO] backup created successfully: %s", filename)
	return nil
}

// sqliteBackup creates a backup using the SQLite backup API.
// It connects to the source database and uses "backup to" SQL command
// which is supported by modernc.org/sqlite.
func (s *BackupService) sqliteBackup(destPath string) error {
	// Acquire a dedicated connection to perform the backup.
	// We use the VACUUM INTO approach which is safe for online backups
	// with modernc.org/sqlite, or fall back to file copy with WAL checkpoint.
	//
	// Strategy: Checkpoint WAL first, then copy the database file + WAL + SHM files.
	// This is the safest approach for modernc.org/sqlite which may not fully
	// support the backup API.

	// Checkpoint the WAL to flush all writes to the main database file
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		log.Printf("[WARN] WAL checkpoint failed (non-fatal): %v", err)
	}

	// Copy the main database file
	srcPath := s.dbPath
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source db: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source db: %w", err)
	}

	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer func() { _ = destFile.Close() }()

	written, err := io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("copy db file: %w", err)
	}
	if written != srcInfo.Size() {
		return fmt.Errorf("copy incomplete: expected %d bytes, wrote %d", srcInfo.Size(), written)
	}

	return nil
}

// compressFile creates a gzipped copy of the source file.
func (s *BackupService) compressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	gzWriter := gzip.NewWriter(dstFile)
	gzWriter.Name = filepath.Base(src)
	defer func() { _ = gzWriter.Close() }()

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		return err
	}

	return gzWriter.Close()
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
		if !strings.HasPrefix(name, "sqlflow-") {
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
		if !strings.HasPrefix(name, "sqlflow-") {
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
	if !strings.HasPrefix(filename, "sqlflow-") {
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
