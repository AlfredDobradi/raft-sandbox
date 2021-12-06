package main

import "github.com/alecthomas/kong"

var CLI struct {
	Debug bool `help:"Enable debug mode"`

	Start StartCmd `cmd:"" help:"Start service"`
}

type Context struct {
	Debug bool
}

func main() {
	ctx := kong.Parse(&CLI)
	err := ctx.Run(&Context{Debug: CLI.Debug})
	ctx.FatalIfErrorf(err)
}
