package usercache

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

func TestModuleMetadata(t *testing.T) {
	m := New()

	if m.ID() != "usercache" {
		t.Errorf("expected ID usercache, got %s", m.ID())
	}
	if m.Name() != "Desktop & Browser Caches" {
		t.Errorf("expected Name 'Desktop & Browser Caches', got %s", m.Name())
	}
	if m.Category() != "user_cache" {
		t.Errorf("expected Category user_cache, got %s", m.Category())
	}
	if m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be false")
	}
	if !m.IsAvailable() {
		t.Errorf("expected IsAvailable to be true")
	}
}

func TestScanTrash(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)

	ctx := context.Background()

	// Initially empty: should return 0 items
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected 0 items for empty trash, got %d", len(res.Items))
	}

	// Create trash files
	filesDir := filepath.Join(tmpHome, ".local", "share", "Trash", "files")
	infoDir := filepath.Join(tmpHome, ".local", "share", "Trash", "info")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := []byte("hello world deleted file")
	if err := os.WriteFile(filepath.Join(filesDir, "test.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}
	infoContent := []byte("[Trash Info]\nPath=/home/user/test.txt\n")
	if err := os.WriteFile(filepath.Join(infoDir, "test.txt.trashinfo"), infoContent, 0644); err != nil {
		t.Fatal(err)
	}

	res, err = m.Scan(ctx)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	var trashItem *core.CleanableItem
	for _, it := range res.Items {
		if it.ID == "usercache:trash" {
			itemCopy := it
			trashItem = &itemCopy
			break
		}
	}

	if trashItem == nil {
		t.Fatalf("expected usercache:trash item to be found")
	}

	expectedSize := int64(len(content) + len(infoContent))
	if trashItem.Size != expectedSize {
		t.Errorf("expected trash size %d, got %d", expectedSize, trashItem.Size)
	}
	if trashItem.Safety != core.SafetySafe {
		t.Errorf("expected trash safety SAFE, got %s", trashItem.Safety)
	}
	if !trashItem.Selected {
		t.Errorf("expected trash item to be selected by default")
	}
}

func TestScanThumbnails(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)
	ctx := context.Background()

	thumbDirs := []string{"normal", "large", "fail"}
	var expectedSize int64
	for _, sub := range thumbDirs {
		dir := filepath.Join(tmpHome, ".cache", "thumbnails", sub)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		data := []byte("thumbnail fake png data")
		if err := os.WriteFile(filepath.Join(dir, "thumb.png"), data, 0644); err != nil {
			t.Fatal(err)
		}
		expectedSize += int64(len(data))
	}

	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	var thumbItem *core.CleanableItem
	for _, it := range res.Items {
		if it.ID == "usercache:thumbnails" {
			itemCopy := it
			thumbItem = &itemCopy
			break
		}
	}

	if thumbItem == nil {
		t.Fatalf("expected thumbnails item to be found")
	}
	if thumbItem.Size != expectedSize {
		t.Errorf("expected thumbnail size %d, got %d", expectedSize, thumbItem.Size)
	}
	if thumbItem.Safety != core.SafetySafe {
		t.Errorf("expected thumbnail safety SAFE, got %s", thumbItem.Safety)
	}
}

func TestScanFirefox_And_LockDetection(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)
	ctx := context.Background()

	profileName := "abcdef12.default-release"
	cache2Dir := filepath.Join(tmpHome, ".cache", "mozilla", "firefox", profileName, "cache2")
	if err := os.MkdirAll(cache2Dir, 0755); err != nil {
		t.Fatal(err)
	}

	cacheData := []byte("firefox cached asset 1234567890")
	if err := os.WriteFile(filepath.Join(cache2Dir, "entry1"), cacheData, 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Scan when browser is NOT locked
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var ffItem *core.CleanableItem
	for _, it := range res.Items {
		if strings.HasPrefix(it.ID, "usercache:firefox:") {
			itemCopy := it
			ffItem = &itemCopy
			break
		}
	}

	if ffItem == nil {
		t.Fatalf("expected Firefox cache item to be discovered")
	}
	if ffItem.Size != int64(len(cacheData)) {
		t.Errorf("expected size %d, got %d", len(cacheData), ffItem.Size)
	}
	if !ffItem.Selected {
		t.Errorf("expected unlocked Firefox item to be selected")
	}
	if ffItem.Metadata["locked"] != "false" {
		t.Errorf("expected locked=false metadata")
	}

	// 2. Lock the profile via parent.lock in profile dir
	profileDir := filepath.Join(tmpHome, ".mozilla", "firefox", profileName)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(profileDir, "parent.lock")
	if err := os.WriteFile(lockFile, []byte("locked"), 0644); err != nil {
		t.Fatal(err)
	}

	resLocked, err := m.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range resLocked.Items {
		if strings.HasPrefix(it.ID, "usercache:firefox:") {
			if it.Selected {
				t.Errorf("expected locked Firefox profile item to NOT be selected")
			}
			if it.Metadata["locked"] != "true" {
				t.Errorf("expected locked=true metadata when parent.lock exists")
			}
		}
	}

	// 3. Attempt Clean on locked Firefox profile: must skip unlinking
	cleanRes, err := m.Clean(ctx, []core.CleanableItem{*ffItem}, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("unexpected fatal clean error: %v", err)
	}
	if len(cleanRes.Errors) == 0 {
		t.Fatalf("expected clean to record an error for locked browser profile")
	}
	if len(cleanRes.CleanedItems) != 0 {
		t.Errorf("expected 0 cleaned items for locked profile")
	}

	// Verify file was NOT deleted
	if _, err := os.Stat(filepath.Join(cache2Dir, "entry1")); os.IsNotExist(err) {
		t.Errorf("file in cache2 was deleted despite parent.lock!")
	}
}

func TestScanChromiumAndChrome(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)
	ctx := context.Background()

	chromCache := filepath.Join(tmpHome, ".cache", "chromium", "Default", "Cache")
	if err := os.MkdirAll(chromCache, 0755); err != nil {
		t.Fatal(err)
	}
	chromeCache := filepath.Join(tmpHome, ".cache", "google-chrome", "Default", "Cache")
	if err := os.MkdirAll(chromeCache, 0755); err != nil {
		t.Fatal(err)
	}

	payload := []byte("chromium cache data")
	if err := os.WriteFile(filepath.Join(chromCache, "data_0"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chromeCache, "data_1"), payload, 0644); err != nil {
		t.Fatal(err)
	}

	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	foundChromium := false
	foundChrome := false
	for _, it := range res.Items {
		if it.ID == "usercache:chromium:default" {
			foundChromium = true
			if it.Size != int64(len(payload)) {
				t.Errorf("expected chromium size %d, got %d", len(payload), it.Size)
			}
		}
		if it.ID == "usercache:google-chrome:default" {
			foundChrome = true
			if it.Size != int64(len(payload)) {
				t.Errorf("expected chrome size %d, got %d", len(payload), it.Size)
			}
		}
	}

	if !foundChromium {
		t.Errorf("expected chromium item to be detected")
	}
	if !foundChrome {
		t.Errorf("expected google-chrome item to be detected")
	}

	// Test SingletonLock
	configDir := filepath.Join(tmpHome, ".config", "chromium")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "SingletonLock"), []byte("lock"), 0644); err != nil {
		t.Fatal(err)
	}

	resLocked, _ := m.Scan(ctx)
	for _, it := range resLocked.Items {
		if it.ID == "usercache:chromium:default" {
			if it.Selected {
				t.Errorf("expected locked Chromium to not be selected by default")
			}
			if it.Metadata["locked"] != "true" {
				t.Errorf("expected locked=true metadata for Chromium with SingletonLock")
			}
		}
	}
}

func TestScanIdleLogs(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)
	m.SetLogIdleDays(30)
	ctx := context.Background()

	logDir := filepath.Join(tmpHome, ".local", "state", "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}

	oldLog := filepath.Join(logDir, "old-app.log")
	recentLog := filepath.Join(logDir, "recent-app.log")

	data := []byte("log data 123456789")
	if err := os.WriteFile(oldLog, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recentLog, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Backdate oldLog to 45 days ago
	oldTime := time.Now().Add(-45 * 24 * time.Hour)
	if err := os.Chtimes(oldLog, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var foundOld, foundRecent bool
	for _, it := range res.Items {
		if it.Path == oldLog {
			foundOld = true
			if it.Safety != core.SafetyCaution {
				t.Errorf("expected SafetyCaution for log item, got %s", it.Safety)
			}
			if it.Selected {
				t.Errorf("expected Caution items to NOT be selected by default")
			}
		}
		if it.Path == recentLog {
			foundRecent = true
		}
	}

	if !foundOld {
		t.Errorf("expected old log to be scanned")
	}
	if foundRecent {
		t.Errorf("recent log should not be marked as cleanable")
	}
}

func TestClean_DryRunPreservesFiles(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)
	ctx := context.Background()

	filesDir := filepath.Join(tmpHome, ".local", "share", "Trash", "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(filesDir, "file_to_keep.txt")
	if err := os.WriteFile(filePath, []byte("dry run test"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := m.Scan(ctx)
	if err != nil || len(res.Items) == 0 {
		t.Fatalf("scan failed: %v", err)
	}

	cleanRes, err := m.Clean(ctx, res.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if len(cleanRes.CleanedItems) != len(res.Items) {
		t.Errorf("expected %d cleaned items in dry run, got %d", len(res.Items), len(cleanRes.CleanedItems))
	}
	if cleanRes.FreedBytes != res.TotalSize {
		t.Errorf("expected %d freed bytes in dry run, got %d", res.TotalSize, cleanRes.FreedBytes)
	}

	// Verify file is still present
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("file was deleted during dry run!")
	}
}

func TestClean_RealExecution(t *testing.T) {
	tmpHome := t.TempDir()
	m := NewWithHome(tmpHome)
	ctx := context.Background()

	// 1. Trash
	filesDir := filepath.Join(tmpHome, ".local", "share", "Trash", "files")
	infoDir := filepath.Join(tmpHome, ".local", "share", "Trash", "info")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		t.Fatal(err)
	}
	delFile := filepath.Join(filesDir, "doc.pdf")
	if err := os.WriteFile(delFile, []byte("pdf data"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Thumbnails
	thumbSub := filepath.Join(tmpHome, ".cache", "thumbnails", "normal")
	if err := os.MkdirAll(thumbSub, 0755); err != nil {
		t.Fatal(err)
	}
	thumbFile := filepath.Join(thumbSub, "img.png")
	if err := os.WriteFile(thumbFile, []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Firefox Cache
	ffCache := filepath.Join(tmpHome, ".cache", "mozilla", "firefox", "default-release", "cache2")
	if err := os.MkdirAll(ffCache, 0755); err != nil {
		t.Fatal(err)
	}
	ffFile := filepath.Join(ffCache, "cache_entry")
	if err := os.WriteFile(ffFile, []byte("cache"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Items) == 0 {
		t.Fatalf("expected items to be found")
	}

	cleanRes, err := m.Clean(ctx, res.Items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("clean error: %v", err)
	}

	if len(cleanRes.Errors) > 0 {
		t.Errorf("unexpected clean errors: %v", cleanRes.Errors)
	}
	if cleanRes.FreedBytes != res.TotalSize {
		t.Errorf("expected freed bytes %d, got %d", res.TotalSize, cleanRes.FreedBytes)
	}

	// Verify target files are unlinked
	if _, err := os.Stat(delFile); !os.IsNotExist(err) {
		t.Errorf("trash file was not unlinked")
	}
	if _, err := os.Stat(thumbFile); !os.IsNotExist(err) {
		t.Errorf("thumbnail file was not unlinked")
	}
	if _, err := os.Stat(ffFile); !os.IsNotExist(err) {
		t.Errorf("firefox cache file was not unlinked")
	}

	// Verify directories themselves still exist
	if _, err := os.Stat(filesDir); os.IsNotExist(err) {
		t.Errorf("trash files directory was destroyed")
	}
	if _, err := os.Stat(thumbSub); os.IsNotExist(err) {
		t.Errorf("thumbnail subdirectory was destroyed")
	}
	if _, err := os.Stat(ffCache); os.IsNotExist(err) {
		t.Errorf("firefox cache directory was destroyed")
	}
}
