package lease_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"leasemanager/lease"
)

const testTTL = 10 * time.Second

func newTestManager(t *testing.T) (*lease.Manager, *lease.ManualClock) {
	t.Helper()
	clock := lease.NewManualClock(time.Unix(1_700_000_000, 0))
	m, err := lease.NewManager(testTTL, lease.WithClock(clock))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, clock
}

func mustAcquire(t *testing.T, m *lease.Manager, key, owner string) lease.Lease {
	t.Helper()
	l, err := m.Acquire(key, owner)
	if err != nil {
		t.Fatalf("Acquire(%q, %q): %v", key, owner, err)
	}
	return l
}

func TestAcquireAndGet(t *testing.T) {
	m, clock := newTestManager(t)
	start := clock.Now()

	l := mustAcquire(t, m, "res-1", "alice")
	if l.Key != "res-1" || l.Owner != "alice" {
		t.Fatalf("lease = %+v, want key=res-1 owner=alice", l)
	}
	if l.Token == "" {
		t.Fatal("token must not be empty")
	}
	if want := start.Add(testTTL); !l.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", l.ExpiresAt, want)
	}

	got, ok := m.Get("res-1")
	if !ok || got != l {
		t.Fatalf("Get = %+v, %v; want %+v, true", got, ok, l)
	}
	if _, ok := m.Get("missing"); ok {
		t.Fatal("Get on unknown key must report false")
	}
	if m.Len() != 1 {
		t.Fatalf("Len = %d, want 1", m.Len())
	}
	if m.TTL() != testTTL {
		t.Fatalf("TTL = %v, want %v", m.TTL(), testTTL)
	}
}

func TestAcquireWhileHeld(t *testing.T) {
	m, _ := newTestManager(t)
	mustAcquire(t, m, "res-1", "alice")

	if _, err := m.Acquire("res-1", "bob"); !errors.Is(err, lease.ErrLeaseHeld) {
		t.Fatalf("second Acquire err = %v, want ErrLeaseHeld", err)
	}
	if _, err := m.Acquire("res-1", "alice"); !errors.Is(err, lease.ErrLeaseHeld) {
		t.Fatalf("re-Acquire by same owner err = %v, want ErrLeaseHeld", err)
	}
	mustAcquire(t, m, "res-2", "bob")
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
	}
}

func TestAcquireAfterExpiry(t *testing.T) {
	m, clock := newTestManager(t)
	first := mustAcquire(t, m, "res-1", "alice")

	clock.Advance(testTTL + time.Second)
	if _, ok := m.Get("res-1"); ok {
		t.Fatal("expired lease must not be returned by Get")
	}
	if m.Len() != 0 {
		t.Fatalf("Len = %d, want 0 after expiry", m.Len())
	}

	second := mustAcquire(t, m, "res-1", "bob")
	if second.Token == first.Token {
		t.Fatal("re-acquire after expiry must issue a new token")
	}
	if got, ok := m.Get("res-1"); !ok || got.Owner != "bob" {
		t.Fatalf("Get = %+v, %v; want owner bob", got, ok)
	}
}

func TestTokenNotReused(t *testing.T) {
	m, _ := newTestManager(t)
	seen := map[lease.Token]bool{}
	for i := 0; i < 3; i++ {
		l := mustAcquire(t, m, "res-1", "alice")
		if seen[l.Token] {
			t.Fatalf("token %q reused on cycle %d", l.Token, i)
		}
		seen[l.Token] = true
		if err := m.Release("res-1", "alice", l.Token); err != nil {
			t.Fatalf("Release: %v", err)
		}
	}
}

func TestRenew(t *testing.T) {
	m, clock := newTestManager(t)
	l := mustAcquire(t, m, "res-1", "alice")

	clock.Advance(6 * time.Second)
	renewed, err := m.Renew("res-1", "alice", l.Token)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if want := clock.Now().Add(testTTL); !renewed.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", renewed.ExpiresAt, want)
	}
	if renewed.Token != l.Token {
		t.Fatal("Renew must keep the same token")
	}

	clock.Advance(9 * time.Second) // past the original deadline
	if _, ok := m.Get("res-1"); !ok {
		t.Fatal("lease must survive its original deadline after Renew")
	}
	clock.Advance(2 * time.Second) // past the renewed deadline
	if _, ok := m.Get("res-1"); ok {
		t.Fatal("lease must expire at the renewed deadline")
	}
}

func TestRenewRejectsBadOwnerOrToken(t *testing.T) {
	m, _ := newTestManager(t)
	l := mustAcquire(t, m, "res-1", "alice")

	if _, err := m.Renew("res-1", "mallory", l.Token); !errors.Is(err, lease.ErrOwnerMismatch) {
		t.Fatalf("wrong owner err = %v, want ErrOwnerMismatch", err)
	}
	if _, err := m.Renew("res-1", "alice", l.Token+"-stale"); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("wrong token err = %v, want ErrTokenMismatch", err)
	}
	if _, err := m.Renew("missing", "alice", l.Token); !errors.Is(err, lease.ErrLeaseNotFound) {
		t.Fatalf("missing key err = %v, want ErrLeaseNotFound", err)
	}
	if got, ok := m.Get("res-1"); !ok || !got.ExpiresAt.Equal(l.ExpiresAt) {
		t.Fatalf("failed Renew must not change the lease, got %+v, %v", got, ok)
	}
}

func TestRelease(t *testing.T) {
	m, _ := newTestManager(t)
	l := mustAcquire(t, m, "res-1", "alice")

	if err := m.Release("res-1", "alice", l.Token); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, ok := m.Get("res-1"); ok {
		t.Fatal("released lease must not be returned by Get")
	}
	if m.Len() != 0 {
		t.Fatalf("Len = %d, want 0 after Release", m.Len())
	}

	next := mustAcquire(t, m, "res-1", "bob")
	if _, err := m.Renew("res-1", "alice", l.Token); !errors.Is(err, lease.ErrOwnerMismatch) {
		t.Fatalf("old owner/token err = %v, want ErrOwnerMismatch", err)
	}
	if err := m.Release("res-1", "bob", l.Token); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("old token err = %v, want ErrTokenMismatch", err)
	}
	if err := m.Release("res-1", "bob", next.Token); err != nil {
		t.Fatalf("Release with current token: %v", err)
	}
}

func TestReleaseRejectsBadOwnerOrToken(t *testing.T) {
	m, _ := newTestManager(t)
	l := mustAcquire(t, m, "res-1", "alice")

	if err := m.Release("res-1", "mallory", l.Token); !errors.Is(err, lease.ErrOwnerMismatch) {
		t.Fatalf("wrong owner err = %v, want ErrOwnerMismatch", err)
	}
	if err := m.Release("res-1", "alice", l.Token+"-stale"); !errors.Is(err, lease.ErrTokenMismatch) {
		t.Fatalf("wrong token err = %v, want ErrTokenMismatch", err)
	}
	if err := m.Release("missing", "alice", l.Token); !errors.Is(err, lease.ErrLeaseNotFound) {
		t.Fatalf("missing key err = %v, want ErrLeaseNotFound", err)
	}
	if _, ok := m.Get("res-1"); !ok {
		t.Fatal("failed Release must keep the lease active")
	}
}

func TestSnapshot(t *testing.T) {
	m, clock := newTestManager(t)
	mustAcquire(t, m, "res-c", "carol")
	mustAcquire(t, m, "res-a", "alice")
	mustAcquire(t, m, "res-b", "bob")

	snap := m.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Snapshot len = %d, want 3", len(snap))
	}
	for i, want := range []string{"res-a", "res-b", "res-c"} {
		if snap[i].Key != want {
			t.Fatalf("Snapshot[%d].Key = %q, want %q", i, snap[i].Key, want)
		}
	}

	snap[0].Owner = "tampered"
	if got, _ := m.Get("res-a"); got.Owner != "alice" {
		t.Fatalf("mutating the snapshot changed internal state: %+v", got)
	}

	clock.Advance(testTTL + time.Second)
	if snap := m.Snapshot(); len(snap) != 0 {
		t.Fatalf("Snapshot after expiry len = %d, want 0", len(snap))
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		if _, err := lease.NewManager(ttl); !errors.Is(err, lease.ErrInvalidTTL) {
			t.Fatalf("NewManager(%v) err = %v, want ErrInvalidTTL", ttl, err)
		}
	}
	m, _ := newTestManager(t)
	if _, err := m.Acquire("", "alice"); !errors.Is(err, lease.ErrEmptyKey) {
		t.Fatalf("empty key err = %v, want ErrEmptyKey", err)
	}
	if _, err := m.Acquire("res-1", ""); !errors.Is(err, lease.ErrEmptyOwner) {
		t.Fatalf("empty owner err = %v, want ErrEmptyOwner", err)
	}
}

func TestConcurrentAcquire(t *testing.T) {
	m, _ := newTestManager(t)
	const goroutines = 16

	var wg sync.WaitGroup
	wins := make(chan lease.Lease, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			if l, err := m.Acquire("res-1", owner); err == nil {
				wins <- l
			}
		}(fmt.Sprintf("owner-%d", i))
	}
	wg.Wait()
	close(wins)

	var winners []lease.Lease
	for l := range wins {
		winners = append(winners, l)
	}
	if len(winners) != 1 {
		t.Fatalf("successful Acquires = %d, want exactly 1", len(winners))
	}
	if got, ok := m.Get("res-1"); !ok || got.Token != winners[0].Token {
		t.Fatalf("Get = %+v, %v; want the winning lease", got, ok)
	}
	if m.Len() != 1 {
		t.Fatalf("Len = %d, want 1", m.Len())
	}
}
