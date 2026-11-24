package runtime

import (
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/i18n"
)

// turnError is the error a thread keeps in its state when a turn ends badly.
func turnError(end harness.TurnEnded) string {
	switch end.Reason {
	case harness.Interrupted:
		return i18n.Text("turn.interrupted")
	case harness.NoResponse:
		return i18n.Text("turn.no_response")
	case harness.Failed:
		if end.Error != "" {
			return end.Error
		}
		return i18n.Text("turn.failed")
	default:
		return ""
	}
}

// turnFailureCode is the code of the chat message of a failed turn. The text
// the agent reported itself is not ours to translate: it travels as Text.
func turnFailureCode(end harness.TurnEnded) string {
	switch {
	case end.Reason == harness.NoResponse:
		return "turn.no_response"
	case end.Reason == harness.Failed && end.Error == "":
		return "turn.failed"
	}
	return ""
}

func turnFailureText(end harness.TurnEnded) string {
	if turnFailureCode(end) != "" {
		return ""
	}
	return turnError(end)
}
