package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWriteLockNil(t *testing.T) {
	err := WriteLock(t.TempDir(), nil)
	if err == nil {
		t.Fatalf("expected error writing nil lock, got nil")
	}
}

func TestReadLockMissing(t *testing.T) {
	_, err := ReadLock(t.TempDir())
	if err == nil {
		t.Fatalf("expected error reading missing lock, got nil")
	}
}

func TestLockRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		lock *Lock
	}{
		{
			name: "full lock round-trip",
			lock: &Lock{
				SourceURL: "https://models.dev/catalog.json",
				SyncedAt:  time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
				SHA256:    "abcdef0123456789",
				Bytes:     12345,
				TTLHours:  24,
			},
		},
		{
			name: "zero-value lock round-trip",
			lock: &Lock{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			destDir := t.TempDir()
			if err := WriteLock(destDir, tc.lock); err != nil {
				t.Fatalf("WriteLock error: %v", err)
			}

			got, err := ReadLock(destDir)
			if err != nil {
				t.Fatalf("ReadLock error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.lock) {
				t.Fatalf("lock mismatch:\n got: %+v\nwant: %+v", got, tc.lock)
			}

			tmpPath := filepath.Join(destDir, lockTmpFileName)
			if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
				t.Fatalf("expected no temp lock file left behind, got %v", err)
			}
		})
	}
}

func TestWriteLockCreatesDestDir(t *testing.T) {
	base := t.TempDir()
	destDir := filepath.Join(base, "nested", "catalog")
	lock := &Lock{SourceURL: "https://models.dev/catalog.json"}
	if err := WriteLock(destDir, lock); err != nil {
		t.Fatalf("WriteLock error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, lockFileName)); err != nil {
		t.Fatalf("expected lock file in created dir: %v", err)
	}
}

func TestReadLockErrors(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(destDir string) error
	}{
		{
			name:    "missing lock file",
			prepare: func(_ string) error { return nil },
		},
		{
			name: "corrupted lock file JSON",
			prepare: func(destDir string) error {
				return os.WriteFile(filepath.Join(destDir, lockFileName), []byte("not json"), 0644)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			destDir := t.TempDir()
			if err := tc.prepare(destDir); err != nil {
				t.Fatalf("prepare error: %v", err)
			}
			if _, err := ReadLock(destDir); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestLockExpired(t *testing.T) {
	cases := []struct {
		name    string
		lock    *Lock
		now     time.Time
		expired bool
	}{
		{
			name:    "nil lock is expired",
			lock:    nil,
			now:     time.Now(),
			expired: true,
		},
		{
			name: "fresh lock is not expired",
			lock: &Lock{
				SyncedAt: time.Now().Add(-1 * time.Hour),
				TTLHours: 24,
			},
			now:     time.Now(),
			expired: false,
		},
		{
			name: "stale lock is expired",
			lock: &Lock{
				SyncedAt: time.Now().Add(-25 * time.Hour),
				TTLHours: 24,
			},
			now:     time.Now(),
			expired: true,
		},
		{
			name: "zero TTL is expired",
			lock: &Lock{
				SyncedAt: time.Now(),
				TTLHours: 0,
			},
			now:     time.Now(),
			expired: true,
		},
		{
			name: "negative TTL is expired",
			lock: &Lock{
				SyncedAt: time.Now(),
				TTLHours: -1,
			},
			now:     time.Now(),
			expired: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.lock.Expired(tc.now); got != tc.expired {
				t.Fatalf("Expired(%v) = %v, want %v", tc.now, got, tc.expired)
			}
		})
	}
}
