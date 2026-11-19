package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/LiteyukiStudio/devops/internal/api"
)

func main() {
	operations := api.DefaultAIToolCatalog()
	if err := json.NewEncoder(os.Stdout).Encode(operations); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
