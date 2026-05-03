package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/htxryan/butverify/internal/cliref"
)

func newCLIFlagSet(name string) (*flag.FlagSet, cliref.FlagValues) {
	fs, values, ok := cliref.NewFlagSet(name, io.Discard)
	if !ok {
		panic("unknown CLI command metadata: " + name)
	}
	return fs, values
}

func handleFlagParseError(g globalContext, name string, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		printCommandHelp(name)
		return 0
	}
	g.w.Error(toErrorEnvelope(err))
	return 2
}

func printCommandHelp(name string) {
	fmt.Print(cliref.CommandHelp(name))
}

func usageError(name string) error {
	return errors.New(cliref.UsageError(name))
}
