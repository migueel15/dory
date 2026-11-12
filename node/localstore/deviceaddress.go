package localstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/migueel15/dory/internal/service"
)

// DeviceAddress is where a device can be reached. Each device publishes its own
// record and nobody else edits it; the record replicates like any other, so
// every peer learns the addresses without a server.
type DeviceAddress struct {
	service.Record
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	// Local are host:port addresses on the LAN, Tailnet a host:port reachable
	// over the tailnet, Manual a host:port the user typed.
	Local   []string `json:"local"`
	Tailnet string   `json:"tailnet"`
	Manual  string   `json:"manual"`
}

// DeviceAddressID is the stable record id of a device's address record: a UUID
// derived from the device id, so concurrent first publishes converge.
func DeviceAddressID(deviceID string) string {
	sum := sha256.Sum256([]byte("dory-deviceaddress:" + deviceID))
	b := sum[:16]
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// DeviceAddresses is the replicated address book.
func (s *Store) DeviceAddresses() service.Resource[DeviceAddress] {
	return res[DeviceAddress](s.Service.(*backend), "deviceaddress")
}

// PublishAddress writes the record of the local device, replacing the previous
// one.
func (s *Store) PublishAddress(ctx context.Context, a DeviceAddress) error {
	a.ID = DeviceAddressID(a.DeviceID)
	if a.Local == nil {
		a.Local = []string{}
	}
	_, err := s.DeviceAddresses().Put(ctx, a.DeviceID, a)
	return err
}

// AddressOf returns the record published by deviceID, or false if none arrived.
func (s *Store) AddressOf(ctx context.Context, deviceID string) (DeviceAddress, bool, error) {
	a, err := s.DeviceAddresses().Get(ctx, DeviceAddressID(deviceID))
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return DeviceAddress{}, false, nil
		}
		return DeviceAddress{}, false, err
	}
	return a, true, nil
}
