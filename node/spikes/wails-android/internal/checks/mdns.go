package checks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

const MDNSService = "_dory._tcp"

// CheckMDNS announces a _dory._tcp instance and browses for it. The Android
// multicast lock must already be held (the page calls DoryNative.multicast(true)).
// On success it reports every instance found, including ones from other devices.
func CheckMDNS(env *Env, port int) (string, error) {
	ifaces := env.NetIfaces()
	if len(ifaces) == 0 {
		return "", fmt.Errorf("no multicast interfaces known (Android: DoryNative.interfaces() not pushed?)")
	}
	instance := fmt.Sprintf("dory-spike-%d", time.Now().Unix()%100000)
	// RegisterProxy with explicit IPs: zeroconf.Register calls net.InterfaceAddrs(),
	// which fails on Android 11+ ("Could not determine host IP addresses").
	var ips []string
	for _, ip := range env.IPs() {
		ips = append(ips, ip.String())
	}
	srv, err := zeroconf.RegisterProxy(instance, MDNSService, "local.", port,
		"dory-spike", ips, []string{"spike=1"}, ifaces)
	if err != nil {
		return "", fmt.Errorf("register: %w", err)
	}
	defer srv.Shutdown()

	res, err := zeroconf.NewResolver(zeroconf.SelectIfaces(ifaces))
	if err != nil {
		return "", fmt.Errorf("resolver: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	entries := make(chan *zeroconf.ServiceEntry, 16)
	if err = res.Browse(ctx, MDNSService, "local.", entries); err != nil {
		return "", fmt.Errorf("browse: %w", err)
	}
	var found []string
	self := false
	for {
		select {
		case e, ok := <-entries:
			if !ok {
				goto done
			}
			found = append(found, fmt.Sprintf("%s@%v:%d", e.Instance, e.AddrIPv4, e.Port))
			if e.Instance == instance {
				self = true
				cancel()
			}
		case <-ctx.Done():
			// drain remaining buffered entries
			for {
				select {
				case e, ok := <-entries:
					if !ok {
						goto done
					}
					found = append(found, fmt.Sprintf("%s@%v:%d", e.Instance, e.AddrIPv4, e.Port))
					if e.Instance == instance {
						self = true
					}
				default:
					goto done
				}
			}
		}
	}
done:
	if !self {
		return "", fmt.Errorf("announced %q but did not discover it (found: %s); multicast lock/Wi-Fi AP isolation?",
			instance, strings.Join(found, ", "))
	}
	return fmt.Sprintf("announced %s.%s:%d and discovered it; all found: %s",
		instance, MDNSService, port, strings.Join(found, ", ")), nil
}
