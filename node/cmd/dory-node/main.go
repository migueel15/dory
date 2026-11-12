// Command dory-node es el nodo local-first de Dory.
package main

import (
	"fmt"
	"os"

	"github.com/migueel15/dory/internal/service"
)

var _ service.Service

const version = "0.0.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "import" {
		if err := runImport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "dory-node import:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println("dory-node", version)
}
