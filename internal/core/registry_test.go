package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type mockModule struct {
	id                string
	name              string
	category          string
	description       string
	available         bool
	requiresElevation bool
	scanFunc          func(ctx context.Context) (ScanResult, error)
	cleanFunc         func(ctx context.Context, items []CleanableItem, opts CleanOptions) (CleanResult, error)
}

func (m *mockModule) ID() string                { return m.id }
func (m *mockModule) Name() string              { return m.name }
func (m *mockModule) Category() string          { return m.category }
func (m *mockModule) Description() string       { return m.description }
func (m *mockModule) IsAvailable() bool         { return m.available }
func (m *mockModule) RequiresElevation() bool   { return m.requiresElevation }
func (m *mockModule) Scan(ctx context.Context) (ScanResult, error) {
	if m.scanFunc != nil {
		return m.scanFunc(ctx)
	}
	return ScanResult{}, nil
}
func (m *mockModule) Clean(ctx context.Context, items []CleanableItem, opts CleanOptions) (CleanResult, error) {
	if m.cleanFunc != nil {
		return m.cleanFunc(ctx, items, opts)
	}
	return CleanResult{}, nil
}

func TestRegistryBasicOperations(t *testing.T) {
	reg := NewRegistry()

	// Test nil registration
	if err := reg.Register(nil); !errors.Is(err, ErrNilModule) {
		t.Fatalf("expected ErrNilModule, got %v", err)
	}

	// Test empty ID registration
	emptyMod := &mockModule{id: ""}
	if err := reg.Register(emptyMod); err == nil {
		t.Fatal("expected error on empty ID, got nil")
	}

	// Register mod1
	mod1 := &mockModule{id: "mod1", name: "Module 1", available: true}
	if err := reg.Register(mod1); err != nil {
		t.Fatalf("unexpected error registering mod1: %v", err)
	}

	// Register duplicate mod1
	if err := reg.Register(mod1); !errors.Is(err, ErrModuleExists) {
		t.Fatalf("expected ErrModuleExists, got %v", err)
	}

	// Register mod2 (unavailable)
	mod2 := &mockModule{id: "mod2", name: "Module 2", available: false}
	if err := reg.Register(mod2); err != nil {
		t.Fatalf("unexpected error registering mod2: %v", err)
	}

	// Get testing
	if got, ok := reg.Get("mod1"); !ok || got.ID() != "mod1" {
		t.Errorf("expected to find mod1, got %v, ok=%v", got, ok)
	}
	if _, ok := reg.Get("nonexistent"); ok {
		t.Error("expected ok=false for nonexistent module")
	}

	// Test Modules() ordering
	all := reg.Modules()
	if len(all) != 2 || all[0].ID() != "mod1" || all[1].ID() != "mod2" {
		t.Errorf("unexpected Modules() order: %v", all)
	}

	// Test AvailableModules()
	avail := reg.AvailableModules()
	if len(avail) != 1 || avail[0].ID() != "mod1" {
		t.Errorf("unexpected AvailableModules(): %v", avail)
	}

	// Test Unregister
	if removed := reg.Unregister("nonexistent"); removed {
		t.Error("expected false when unregistering nonexistent module")
	}
	if removed := reg.Unregister("mod1"); !removed {
		t.Error("expected true when unregistering mod1")
	}
	if _, ok := reg.Get("mod1"); ok {
		t.Error("expected mod1 to be gone")
	}
	if len(reg.Modules()) != 1 {
		t.Errorf("expected 1 module remaining, got %d", len(reg.Modules()))
	}
}

func TestRegistryConcurrentScanAll(t *testing.T) {
	reg := NewRegistry()

	var running int32
	var maxConcurrent int32

	createMod := func(id string, sleep time.Duration, size int64) *mockModule {
		return &mockModule{
			id:        id,
			name:      id,
			available: true,
			scanFunc: func(ctx context.Context) (ScanResult, error) {
				curr := atomic.AddInt32(&running, 1)
				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if curr <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, curr) {
						break
					}
				}
				time.Sleep(sleep)
				atomic.AddInt32(&running, -1)
				return ScanResult{
					Items: []CleanableItem{
						{ID: id + "_item", Size: size, Selected: true},
					},
				}, nil
			},
		}
	}

	_ = reg.Register(createMod("fast1", 20*time.Millisecond, 100))
	_ = reg.Register(createMod("fast2", 20*time.Millisecond, 200))
	_ = reg.Register(createMod("fast3", 20*time.Millisecond, 300))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	results, err := reg.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results["fast1"].TotalSize != 100 {
		t.Errorf("expected total size 100 for fast1, got %d", results["fast1"].TotalSize)
	}

	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Logf("concurrency reached: %d", atomic.LoadInt32(&maxConcurrent))
	}
}

func TestRegistryScanPanicAndErrorHandling(t *testing.T) {
	reg := NewRegistry()

	panicMod := &mockModule{
		id:        "panic_mod",
		name:      "Panicking Module",
		available: true,
		scanFunc: func(ctx context.Context) (ScanResult, error) {
			panic("intentional scan panic")
		},
	}

	errorMod := &mockModule{
		id:        "error_mod",
		name:      "Erroring Module",
		available: true,
		scanFunc: func(ctx context.Context) (ScanResult, error) {
			return ScanResult{}, errors.New("simulated scan error")
		},
	}

	goodMod := &mockModule{
		id:        "good_mod",
		name:      "Good Module",
		available: true,
		scanFunc: func(ctx context.Context) (ScanResult, error) {
			return ScanResult{
				Items: []CleanableItem{{ID: "item1", Size: 500}},
			}, nil
		},
	}

	_ = reg.Register(panicMod)
	_ = reg.Register(errorMod)
	_ = reg.Register(goodMod)

	results, err := reg.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected ScanAll error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results["panic_mod"].Error == nil {
		t.Error("expected error for panic_mod, got nil")
	}
	if results["error_mod"].Error == nil {
		t.Error("expected error for error_mod, got nil")
	}
	if results["good_mod"].Error != nil {
		t.Errorf("unexpected error for good_mod: %v", results["good_mod"].Error)
	}
	if results["good_mod"].TotalSize != 500 {
		t.Errorf("expected 500 bytes for good_mod, got %d", results["good_mod"].TotalSize)
	}
}

func TestRegistryScanContextCancellation(t *testing.T) {
	reg := NewRegistry()

	slowMod := &mockModule{
		id:        "slow",
		name:      "Slow",
		available: true,
		scanFunc: func(ctx context.Context) (ScanResult, error) {
			select {
			case <-ctx.Done():
				return ScanResult{}, ctx.Err()
			case <-time.After(500 * time.Millisecond):
				return ScanResult{}, nil
			}
		},
	}
	_ = reg.Register(slowMod)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := reg.ScanAll(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}
