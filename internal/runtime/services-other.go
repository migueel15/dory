//go:build !linux

package runtime

import (
	"context"
	"errors"
	"github.com/migueel15/dory/internal/apperr"
)

var ErrSharedProcessGroup = apperr.New(apperr.SharedProcessGroup, "the service shares a process group with Dory, the agent or a shell")

type Service struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Command string `json:"command"`
	Kind    string `json:"kind"`
}

func ThreadServices(string) []Service    { return []Service{} }
func StopService(string, int) error      { return ErrNotFound }
func StopThreadServices(string)          {}
func (m *Manager) StopAllServices()      {}
func (m *Manager) StopArchivedServices() {}

func (m *Manager) StartService(context.Context, string, string) (int, string, error) {
	return 0, "", errors.New("los servicios solo están disponibles en Linux")
}
func userScope() string { return "" }
