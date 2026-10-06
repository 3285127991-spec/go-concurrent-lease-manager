// Command demo shows the lease manager API with a few built-in calls.
package main

import (
	"fmt"
	"log"
	"time"

	"leasemanager/lease"
)

func main() {
	m, err := lease.NewManager(30 * time.Second)
	must(err)

	l, err := m.Acquire("printer", "alice")
	must(err)
	fmt.Printf("Acquire:  key=%s owner=%s token=%s expiresAt=%s\n",
		l.Key, l.Owner, l.Token, l.ExpiresAt.Format(time.RFC3339))

	if got, ok := m.Get("printer"); ok {
		fmt.Printf("Get:      key=%s owner=%s (len=%d)\n", got.Key, got.Owner, m.Len())
	}

	l, err = m.Renew("printer", "alice", l.Token)
	must(err)
	fmt.Printf("Renew:    new expiresAt=%s\n", l.ExpiresAt.Format(time.RFC3339))

	must(m.Release("printer", "alice", l.Token))
	fmt.Printf("Release:  ok (len=%d)\n", m.Len())

	l2, err := m.Acquire("printer", "bob")
	must(err)
	fmt.Printf("Acquire:  owner=%s token=%s (token reused: %v)\n",
		l2.Owner, l2.Token, l2.Token == l.Token)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
