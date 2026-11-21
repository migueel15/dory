//go:build !linux

package runtime

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
