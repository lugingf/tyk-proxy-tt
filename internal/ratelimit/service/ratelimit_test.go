package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
)

type fakeStore struct {
	lastKey    string
	lastWindow time.Duration

	n   int64
	err error
}

func (f *fakeStore) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	f.lastKey = key
	f.lastWindow = window
	return f.n, f.err
}

func TestNewRateLimitWithOptions_DefaultWindowIsOneMinute(t *testing.T) {
	fs := &fakeStore{n: 1}
	rl := NewRateLimitWithOptions(fs, Options{Window: 0})

	_, _ = rl.Allow(context.Background(), "k", 10)

	if fs.lastWindow != time.Minute {
		t.Fatalf("expected window %s, got %s", time.Minute, fs.lastWindow)
	}
}

func TestNewRateLimit_DefaultWindowIsOneMinute(t *testing.T) {
	fs := &fakeStore{n: 1}
	rl := NewRateLimit(fs)

	_, _ = rl.Allow(context.Background(), "k", 10)

	if fs.lastWindow != time.Minute {
		t.Fatalf("expected window %s, got %s", time.Minute, fs.lastWindow)
	}
}

func TestNewRateLimitWithOptions_CustomWindow(t *testing.T) {
	fs := &fakeStore{n: 1}
	rl := NewRateLimitWithOptions(fs, Options{Window: 10 * time.Second})

	_, _ = rl.Allow(context.Background(), "k", 10)

	if fs.lastWindow != 10*time.Second {
		t.Fatalf("expected window %s, got %s", 10*time.Second, fs.lastWindow)
	}
}

func TestAllow_EmptyKey(t *testing.T) {
	fs := &fakeStore{n: 1}
	rl := NewRateLimitWithOptions(fs, Options{Window: time.Second})

	ok, err := rl.Allow(context.Background(), "", 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
	// ensure store not called
	if fs.lastKey != "" || fs.lastWindow != 0 {
		t.Fatalf("store should not be called, got key=%q window=%v", fs.lastKey, fs.lastWindow)
	}
}

func TestAllow_InvalidLimit(t *testing.T) {
	fs := &fakeStore{n: 1}
	rl := NewRateLimitWithOptions(fs, Options{Window: time.Second})

	ok, err := rl.Allow(context.Background(), "k", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if ok {
		t.Fatal("expected ok=false")
	}
	// ensure store not called
	if fs.lastKey != "" || fs.lastWindow != 0 {
		t.Fatalf("store should not be called, got key=%q window=%v", fs.lastKey, fs.lastWindow)
	}
}

func TestAllow_StoreError_IsWrapped(t *testing.T) {
	root := errors.New("boom")
	fs := &fakeStore{n: 0, err: root}
	rl := NewRateLimitWithOptions(fs, Options{Window: time.Second})

	ok, err := rl.Allow(context.Background(), "k", 10)
	if ok {
		t.Fatal("expected ok=false")
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// pkg/errors.Wrap supports Cause
	if pkgerrors.Cause(err) != root {
		t.Fatalf("expected wrapped cause %v, got %v", root, pkgerrors.Cause(err))
	}
	if err.Error() == root.Error() {
		t.Fatalf("expected wrapped error message, got %q", err.Error())
	}
}

func TestAllow_AllowedWithinLimit(t *testing.T) {
	fs := &fakeStore{n: 5}
	rl := NewRateLimitWithOptions(fs, Options{Window: time.Second})

	ok, err := rl.Allow(context.Background(), "k", 5)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
}

func TestAllow_DeniedOverLimit(t *testing.T) {
	fs := &fakeStore{n: 6}
	rl := NewRateLimitWithOptions(fs, Options{Window: time.Second})

	ok, err := rl.Allow(context.Background(), "k", 5)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestAllow_PassesKeyAndWindowToStore(t *testing.T) {
	fs := &fakeStore{n: 1}
	rl := NewRateLimitWithOptions(fs, Options{Window: 3 * time.Second})

	_, _ = rl.Allow(context.Background(), "my-key", 10)

	if fs.lastKey != "my-key" {
		t.Fatalf("expected key %q, got %q", "my-key", fs.lastKey)
	}
	if fs.lastWindow != 3*time.Second {
		t.Fatalf("expected window %s, got %s", 3*time.Second, fs.lastWindow)
	}
}
