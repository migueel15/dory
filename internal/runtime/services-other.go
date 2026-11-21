//go:build !linux

package runtime

import "errors"

var ErrSharedProcessGroup = errors.New("el servicio comparte grupo con Dory, el agente o una shell")

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
