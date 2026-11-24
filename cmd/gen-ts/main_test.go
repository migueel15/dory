package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedTypesUpToDate(t *testing.T) {
	got, err := os.ReadFile(filepath.Join("..", "..", output))
	if err != nil || !bytes.Equal(got, generate()) {
		t.Fatalf("tipos TypeScript desactualizados, ejecuta go generate ./cmd/gen-ts (lectura: %v)", err)
	}
}
