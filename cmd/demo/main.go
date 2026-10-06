// Command demo is a minimal walkthrough of the lease manager API.
package main

import (
	"fmt"
	"log"
	"time"

	"leasemanager/lease"
)

func main() {
	m, err := lease.NewManager(30 * time.Second)
	if err != nil {
		log.Fatal(err)
	}

	first, err := m.Acquire("res-1", "alice")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Acquire:    key=%s owner=%s token=%s\n", first.Key, first.Owner, first.Token)

	if l, ok := m.Get("res-1"); ok {
		fmt.Printf("Get:        owner=%s expires_at=%s\n", l.Owner, l.ExpiresAt.Format(time.RFC3339))
	}

	renewed, err := m.Renew("res-1", "alice", first.Token)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Renew:      expires_at=%s\n", renewed.ExpiresAt.Format(time.RFC3339))

	if err := m.Release("res-1", "alice", renewed.Token); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Release:    ok (active leases: %d)\n", m.Len())

	second, err := m.Acquire("res-1", "bob")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Re-acquire: owner=%s token=%s (new token: %t)\n", second.Owner, second.Token, second.Token != first.Token)
}
