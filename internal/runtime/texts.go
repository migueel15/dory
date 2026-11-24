package runtime

import "github.com/migueel15/dory/internal/harness"

const interrupted = "interrumpido"
const noResponse = "el agente terminó sin respuesta"
const turnInterrupted = "Turno interrumpido"

func turnError(end harness.TurnEnded) string {
	switch end.Reason {
	case harness.Interrupted:
		return interrupted
	case harness.NoResponse:
		return noResponse
	case harness.Failed:
		if end.Error != "" {
			return end.Error
		}
		return "El turno falló"
	default:
		return ""
	}
}
