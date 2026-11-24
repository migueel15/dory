package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/migueel15/dory/internal/nodestate"
)

func importNodeState(ctx context.Context, store *nodestate.Store, dir string) error {
	for _, legacy := range []struct{ key, name string }{
		{nodestate.KeySettings, "settings.json"},
		{nodestate.KeyUsage, "usage.json"},
		{nodestate.KeyDisabledProviders, "providers.json"},
		{nodestate.KeyAccounts, "accounts.json"},
		{nodestate.KeyModelCatalog, "model-catalog.json"},
		{nodestate.KeyTitleModel, "title-model.json"},
		{nodestate.KeyNodeIcon, "node.json"},
		{nodestate.KeySkillsManifest, "skills-manifest.json"},
	} {
		if err := store.ImportFile(ctx, legacy.key, filepath.Join(dir, legacy.name)); err != nil {
			return fmt.Errorf("import node state %s: %w", legacy.name, err)
		}
	}
	return nil
}
