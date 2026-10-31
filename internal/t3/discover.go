package t3

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/service"
)

const (
	discoverInterval = time.Minute
	discoverTimeout  = 2 * time.Second
	discoverWorkers  = 32
	discoverPort     = 3773
	discoverActor    = "t3-discovery"
	maxDiscoverHosts = 1024
)

var tokenEnvUnsafe = regexp.MustCompile(`[^A-Z0-9]+`)

// runDiscovery scans DORY_T3_DISCOVERY_CIDR; without it nothing is scanned.
func (m *Monitor) runDiscovery(ctx context.Context) {
	cidr := os.Getenv("DORY_T3_DISCOVERY_CIDR")
	if cidr == "" {
		return
	}
	hosts, err := scanHosts(cidr)
	if err != nil {
		log.Printf("t3: discovery: %v", err)
		return
	}
	tick := time.NewTicker(discoverInterval)
	defer tick.Stop()
	for {
		if err := m.syncFound(ctx, m.scan(ctx, hosts)); err != nil {
			log.Printf("t3: discovery: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// scanHosts lists the addresses of an IPv4 range, without network and broadcast.
func scanHosts(cidr string) ([]netip.Addr, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}
	bits := prefix.Bits()
	if !prefix.Addr().Is4() || bits < 22 {
		return nil, fmt.Errorf("t3: %s must be IPv4 and hold at most %d addresses", cidr, maxDiscoverHosts)
	}
	prefix = prefix.Masked()
	var out []netip.Addr
	for a := prefix.Addr(); prefix.Contains(a); a = a.Next() {
		out = append(out, a)
	}
	if bits <= 30 {
		out = out[1 : len(out)-1]
	}
	return out, nil
}

// scan returns every compatible instance that answers, by base URL.
func (m *Monitor) scan(ctx context.Context, hosts []netip.Addr) map[string]Discovery {
	found := map[string]Discovery{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, discoverWorkers)
	for _, h := range hosts {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			base := fmt.Sprintf("http://%s:%d", h, discoverPort)
			if d, ok := m.identify(ctx, base); ok {
				mu.Lock()
				defer mu.Unlock()
				found[base] = d
			}
		}()
	}
	wg.Wait()
	return found
}

func (m *Monitor) identify(ctx context.Context, base string) (Discovery, bool) {
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	d, err := Client{BaseURL: base, HTTP: &http.Client{}}.Discover(ctx)
	return d, err == nil && d.EnvironmentID != "" && d.Label != ""
}

// syncFound registers new instances. T3's environment ID recognises an instance
// under any URL; the name recovers one whose address changed. Environments keep
// their name when archived, so archiving one stops it from coming back.
func (m *Monitor) syncFound(ctx context.Context, found map[string]Discovery) error {
	instances, err := m.svc.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		return err
	}
	envs, err := m.svc.Environments().List(ctx, service.Filter{Limit: 1000, IncludeArchived: true})
	if err != nil {
		return err
	}
	known := map[string]bool{}
	byEnv := map[string]service.T3Instance{}
	for _, in := range instances {
		byEnv[in.EnvironmentID] = in
		if d, ok := m.identify(ctx, in.BaseUrl); ok {
			known[d.EnvironmentID] = true
		}
	}
	byName := map[string]service.Environment{}
	for _, e := range envs {
		byName[e.Name] = e
	}
	for base, d := range found {
		if known[d.EnvironmentID] {
			continue
		}
		known[d.EnvironmentID] = true
		env, exists := byName[d.Label]
		if exists && env.ArchivedAt != nil {
			continue
		}
		if !exists {
			env, err = m.svc.Environments().Put(ctx, discoverActor, service.Environment{Name: d.Label, Os: d.Platform.OS, AgentsAvailable: []string{}})
			if err != nil {
				return err
			}
		}
		in, ok := byEnv[env.ID]
		if !ok {
			in = service.T3Instance{EnvironmentID: env.ID, TokenEnv: tokenEnvName(d.Label)}
		}
		in.BaseUrl = base
		if _, err := m.svc.T3Instances().Put(ctx, discoverActor, in); err != nil {
			return err
		}
		log.Printf("t3: discovered %s at %s", d.Label, base)
	}
	return nil
}

// tokenEnvName is where the deployment must set the new instance's token.
func tokenEnvName(label string) string {
	return "T3_TOKEN_" + strings.Trim(tokenEnvUnsafe.ReplaceAllString(strings.ToUpper(label), "_"), "_")
}
