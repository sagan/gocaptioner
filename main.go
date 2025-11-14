package main

import (
	"github.com/sagan/gocaptioner/cmd"
	_ "github.com/sagan/gocaptioner/cmd/all"
)

func main() {
	cmd.Execute()
}
