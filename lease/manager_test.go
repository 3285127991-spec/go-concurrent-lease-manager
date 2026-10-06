package lease_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"leasemanager/lease"
)

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newTestManager(t *testing.T, ttl time.Duration) (*lease.Manager, *lease.ManualClock) {
	t.Helper()
	clk := lease.NewManualClock(start)
	m, err := lease.NewManager(ttl, lease.WithClock(clk))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, clk
}

func TestNewManagerRejectsInvalidTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		if _, err := lease.NewManager(ttl); !errors.Is(err, lease.ErrInvalidTTL) {
			t.Fatalf("ttl=%v: want ErrInvalidTTL, got %v", ttl, err)
		}
	}
}

func TestAcquireAndGet(t *testing.T) {
	m, _ := newTestManager(t, 10*time.Second)

	l, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if l.Key != "res" || l.Owner != "alice" {
		t.Fatalf("unexpected lease: %+v", l)
	}
	if l.Token == "" {
		t.Fatal("token must not be empty")
	}
	if want := start.Add(10 * time.Second); !l.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", l.ExpiresAt, want)
	}

	got, ok := m.Get("res")
	if !ok || got != l {
		t.Fatalf("Get = %+v, %v; want %+v, true", got, ok, l)
	}
	if _, ok := m.Get("missing"); ok {
		t.Fatal("Get on missing key must report false")
	}
	if n := m.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1", n)
	}
	if ttl := m.TTL(); ttl != 10*time.Second {
		t.Fatalf("TTL = %v, want 10s", ttl)
	}
}

func TestAcquireRejectsEmptyKeyAndOwner(t *testing.T) {
	m, _ := newTestManager(t, 10*time.Second)

	if _, err := m.Acquire("", "alice"); !errors.Is(err, lease.ErrEmptyKey) {
		t.Fatalf("empty key: want ErrEmptyKey, got %v", err)
	}
	if _, err := m.Acquire("res", ""); !errors.Is(err, lease.ErrEmptyOwner) {
		t.Fatalf("empty owner: want ErrEmptyOwner, got %v", err)
	}
}

func TestAcquireDuplicateFails(t *testing.T) {
	m, _ := newTestManager(t, 10*time.Second)

	if _, err := m.Acquire("res", "alice"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := m.Acquire("res", "bob"); !errors.Is(err, lease.ErrLeaseHeld) {
		t.Fatalf("second Acquire: want ErrLeaseHeld, got %v", err)
	}
	if _, err := m.Acquire("other", "bob"); err != nil {
		t.Fatalf("Acquire of different key: %v", err)
	}
	if n := m.Len(); n != 2 {
		t.Fatalf("Len = %d, want 2", n)
	}
}

func TestAcquireAfterExpiry(t *testing.T) {
	m, clk := newTestManager(t, 10*time.Second)

	first, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	clk.Advance(11 * time.Second)

	if _, ok := m.Get("res"); ok {
		t.Fatal("expired lease must not be returned by Get")
	}
	if n := m.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0 after expiry", n)
	}

	second, err := m.Acquire("res", "bob")
	if err != nil {
		t.Fatalf("Acquire after expiry: %v", err)
	}
	if second.Token == first.Token {
		t.Fatal("re-acquire after expiry must mint a new token")
	}
	if want := start.Add(21 * time.Second); !second.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", second.ExpiresAt, want)
	}
}

func TestTokenNotReusedAcrossReacquire(t *testing.T) {
	m, _ := newTestManager(t, 10*time.Second)

	first, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := m.Release("res", "alice", first.Token); err != nil {
		t.Fatalf("Release: %v", err)
	}
	second, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("re-Acquire: %v", err)
	}
	if first.Token == second.Token {
		t.Fatalf("token %q reused", first.Token)
	}
}

func TestRenewExtendsExpiry(t *testing.T) {
	m, clk := newTestManager(t, 10*time.Second)

	l, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	clk.Advance(6 * time.Second)

	renewed, err := m.Renew("res", "alice", l.Token)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if renewed.Token != l.Token {
		t.Fatal("Renew must keep the same token")
	}
	if want := start.Add(16 * time.Second); !renewed.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", renewed.ExpiresAt, want)
	}

	clk.Advance(9 * time.Second) // past the original deadline
	if _, ok := m.Get("res"); !ok {
		t.Fatal("lease must still be active after renew")
	}
	clk.Advance(2 * time.Second) // past the renewed deadline
	if _, ok := m.Get("res"); ok {
		t.Fatal("lease must expire at the renewed deadline")
	}
}

func TestRenewRejectsWrongOwnerOrToken(t *testing.T) {
	m, _ := newTestManager(t, 10*time.Second)

	l, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := m.Renew("res", "bob", l.Token); !errors.Is(err, lease.ErrOwnerMismatch) {
		t.Fatalf("wrong owner: want ErrOwnerMismatch, got %v", err)
	}
	if _, err := m.Renew("res", "alice", "bogus"); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("wrong token: want ErrTokenMismatch, got %v", err)
	}
	if _, err := m.Renew("missing", "alice", "bogus"); !errors.Is(err, lease.ErrLeaseNotFound) {
		t.Fatalf("missing key: want ErrLeaseNotFound, got %v", err)
	}
}

func TestRelease(t *testing.T) {
	m, _ := newTestManager(t, 10*time.Second)

	l, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := m.Release("res", "alice", "bogus"); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("wrong token: want ErrTokenMismatch, got %v", err)
	}
	if err := m.Release("res", "bob", l.Token); !errors.Is(err, lease.ErrOwnerMismatch) {
		t.Fatalf("wrong owner: want ErrOwnerMismatch, got %v", err)
	}
	if err := m.Release("res", "alice", l.Token); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, ok := m.Get("res"); ok {
		t.Fatal("released lease must not be returned by Get")
	}
	if n := m.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0 after release", n)
	}
	if err := m.Release("res", "alice", l.Token); !errors.Is(err, lease.ErrLeaseNotFound) {
		t.Fatalf("second Release: want ErrLeaseNotFound, got %v", err)
	}
}

func TestOldTokenCannotOperateNewLease(t *testing.T) {
	m, clk := newTestManager(t, 10*time.Second)

	first, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := m.Release("res", "alice", first.Token); err != nil {
		t.Fatalf("Release: %v", err)
	}
	second, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("re-Acquire: %v", err)
	}
	if _, err := m.Renew("res", "alice", first.Token); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("old token Renew: want ErrTokenMismatch, got %v", err)
	}
	if err := m.Release("res", "alice", first.Token); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("old token Release: want ErrTokenMismatch, got %v", err)
	}

	clk.Advance(11 * time.Second) // let the second lease expire
	third, err := m.Acquire("res", "alice")
	if err != nil {
		t.Fatalf("Acquire after expiry: %v", err)
	}
	if err := m.Release("res", "alice", second.Token); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("expired token Release: want ErrTokenMismatch, got %v", err)
	}
	if got, ok := m.Get("res"); !ok || got.Token != third.Token {
		t.Fatal("current lease must survive stale-token operations")
	}
}

func TestSnapshotSortedAndIsolated(t *testing.T) {
	m, clk := newTestManager(t, 10*time.Second)

	for _, key := range []string{"gamma", "alpha", "beta"} {
		if _, err := m.Acquire(key, "alice"); err != nil {
			t.Fatalf("Acquire %s: %v", key, err)
		}
	}

	snap := m.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Snapshot len = %d, want 3", len(snap))
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if snap[i].Key != want {
			t.Fatalf("Snapshot[%d].Key = %q, want %q", i, snap[i].Key, want)
		}
	}

	snap[0].Owner = "mallory"
	if got, _ := m.Get("alpha"); got.Owner != "alice" {
		t.Fatal("mutating the snapshot must not affect the manager")
	}

	clk.Advance(11 * time.Second)
	if _, err := m.Acquire("beta", "bob"); err != nil {
		t.Fatalf("re-Acquire expired key: %v", err)
	}
	if n := m.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1 (only re-acquired key is active)", n)
	}
	clk.Advance(11 * time.Second)
	if n := m.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0 after all leases expire", n)
	}
	if got := m.Snapshot(); len(got) != 0 {
		t.Fatalf("Snapshot after expiry = %v, want empty", got)
	}
}

func TestConcurrentAcquireAndMixedOps(t *testing.T) {
	m, err := lease.NewManager(time.Minute)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	const workers = 16
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := m.Acquire("shared", fmt.Sprintf("owner-%d", id))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case !errors.Is(err, lease.ErrLeaseHeld):
			t.Fatalf("unexpected Acquire error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent Acquire successes = %d, want exactly 1", successes)
	}

	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", id%4)
			owner := fmt.Sprintf("writer-%d", id)
			if l, err := m.Acquire(key, owner); err == nil {
				if _, err := m.Renew(key, owner, l.Token); err == nil {
					_ = m.Release(key, owner, l.Token)
				}
			}
		}(i)
		go func() {
			defer wg.Done()
			_, _ = m.Get("shared")
			_ = m.Snapshot()
			_ = m.Len()
		}()
	}
	wg.Wait()

	if got := len(m.Snapshot()); got != m.Len() {
		t.Fatalf("Len = %d but Snapshot has %d entries", m.Len(), got)
	}
}
