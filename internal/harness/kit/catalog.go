package kit

import (
	"bufio"
	"context"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

func ReadModelCatalog(ctx context.Context, bin string, env []string, args []string, requests string, read func([]byte) []harness.Model) []harness.Model {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := cmdexec.CommandContext(ctx, bin, args...)
	if env != nil {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}
	defer cmd.Wait()
	defer cancel()
	if _, err := stdin.Write([]byte(requests)); err != nil {
		return nil
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		if models := read(scanner.Bytes()); len(models) > 0 {
			return models
		}
	}
	return nil
}
