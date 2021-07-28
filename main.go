package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("USAGE: %s FILE\n", os.Args[0])
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		return err
	}
	defer f.Close()

	var env *runtime.Environment
	prelude, err := os.Open("prelude.qn")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		block, err := parser.Parse(parser.NewLexer(bufio.NewReader(prelude)))
		if err != nil {
			return err
		}
		env, err = runtime.Run(nil, block)
		if err != nil {
			return err
		}
	}

	block, err := parser.Parse(parser.NewLexer(bufio.NewReader(f)))
	if err != nil {
		return err
	}
	_, err = runtime.Run(env, block)
	return err
}
