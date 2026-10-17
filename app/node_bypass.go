package app

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/singbox"
)

const (
	nodeResolveTimeout  = 3 * time.Second
	nodeResolveCacheTTL = 10 * time.Minute
)

type nodeResolveEntry struct {
	addrs []netip.Addr
	at    time.Time
}

// nodeResolver caches node server lookups so switching nodes does not wait on
// DNS every time.
type nodeResolver struct {
	mu     sync.Mutex
	cache  map[string]nodeResolveEntry
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
}

func (r *nodeResolver) resolve(nodes []config.Node) []netip.Addr {
	domains := singbox.NodeDomains(nodes)
	now := time.Now()
	r.mu.Lock()
	if r.cache == nil {
		r.cache = make(map[string]nodeResolveEntry)
	}
	var stale []string
	for _, domain := range domains {
		if entry, ok := r.cache[domain]; !ok || now.Sub(entry.at) > nodeResolveCacheTTL {
			stale = append(stale, domain)
		}
	}
	lookup := r.lookup
	r.mu.Unlock()
	if lookup == nil {
		lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}

	if len(stale) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), nodeResolveTimeout)
		defer cancel()
		var wg sync.WaitGroup
		for _, domain := range stale {
			wg.Add(1)
			go func() {
				defer wg.Done()
				addrs, err := lookup(ctx, domain)
				if err != nil || len(addrs) == 0 {
					// Keep the previous answer, if any, when a lookup fails.
					return
				}
				r.mu.Lock()
				r.cache[domain] = nodeResolveEntry{addrs: addrs, at: time.Now()}
				r.mu.Unlock()
			}()
		}
		wg.Wait()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	var result []netip.Addr
	for _, domain := range domains {
		result = append(result, r.cache[domain].addrs...)
	}
	return result
}
