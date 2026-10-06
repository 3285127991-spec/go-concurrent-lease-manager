# go-concurrent-lease-manager

Concurrency-safe in-memory lease manager for Go. Backend library only,
plus a minimal CLI demo and tests. No third-party dependencies.

## Features

- One active lease per key; expired leases are treated as absent.
- Manager-minted tokens that are never reused.
- `Renew` / `Release` require both the owner and token of the active lease.
- `Get` / `Snapshot` / `Len` only see active leases; snapshots are sorted
  by key and detached from internal state.
- Pluggable `Clock` (`ManualClock` for tests) so expiry is testable
  without sleeping.
- Safe for concurrent use; no background goroutines.

## Layout

- `lease/` — the library (`Manager`, `Lease`, `Clock`, `ManualClock`).
- `cmd/demo/` — minimal CLI demo of Acquire / Get / Renew / Release /
  re-Acquire.
- `lease/manager_test.go` — unit and concurrency tests.

## Usage

```go
m, err := lease.NewManager(30 * time.Second) // lease.WithClock(clk) to inject a clock
if err != nil {
	// ttl must be positive
}

l, err := m.Acquire("printer", "alice") // fails with ErrLeaseHeld while held
got, ok := m.Get("printer")
l, err = m.Renew("printer", "alice", l.Token)  // owner + token must match
err = m.Release("printer", "alice", l.Token)
all := m.Snapshot() // active leases, sorted by key
```

## Run

```sh
go run ./cmd/demo
go test -race ./...
```
