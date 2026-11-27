package dictation

import (
	"slices"
	"testing"
)

func TestWorkerArgsUploadLimit(t *testing.T) {
	batch := workerArgs("/models/parakeet.gguf", "8080", "tok", false)
	if !slices.Contains(batch, "--max-upload-mb") {
		t.Fatalf("batch worker should keep --max-upload-mb: %v", batch)
	}
	streaming := workerArgs("/models/nemotron.gguf", "8080", "tok", true)
	if slices.Contains(streaming, "--max-upload-mb") {
		t.Fatalf("streaming worker must not cap --max-upload-mb: %v", streaming)
	}
}
