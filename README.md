# leasemanager

A concurrency-safe, in-memory lease manager written in pure Go (standard
library only). Each resource key holds at most one active lease; leases
expire after a fixed TTL and can be acquired, renewed, released, and queried
from many goroutines at once.

## Layout

- `lease/` — the library: `Manager`, `Lease`, `Clock` (wall clock +
  `ManualClock` for sleep-free tests)
- `cmd/demo/` — minimal CLI walkthrough of Acquire / Get / Renew / Release /
  re-Acquire
- `lease/manager_test.go` — automated tests, no sleeping, no third-party deps

## Usage

```go
m, err := lease.NewManager(30 * time.Second) // ttl must be positive
if err != nil { /* lease.ErrInvalidTTL */ }

l, err := m.Acquire("res-1", "alice")        // fails with ErrLeaseHeld while held
l, err = m.Renew("res-1", "alice", l.Token)  // extends ExpiresAt by one ttl
err = m.Release("res-1", "alice", l.Token)   // owner + token must both match

cur, ok := m.Get("res-1")   // active lease only
all := m.Snapshot()         // active leases, sorted by key, detached copies
n := m.Len()                // number of active leases
ttl := m.TTL()              // configured lease duration
```

Semantics:

- Expired leases are treated as absent everywhere; re-acquiring an expired
  or released key always issues a fresh, never-reused `Token`.
- `Renew`/`Release` validate both owner and token, so a stale token cannot
  touch a newer lease on the same key.
- All operations are atomic under a single mutex; concurrent `Acquire` calls
  on one key succeed at most once.
- Tests inject `lease.NewManualClock(start)` via `lease.WithClock` to
  control time without sleeping.

## Run

```sh
go run ./cmd/demo        # minimal CLI demo
go test -race ./...      # run all tests with the race detector
```
