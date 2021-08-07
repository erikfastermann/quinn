package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/runtime"
)

func main() {
	if err := _main(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func _main() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("USAGE: %s FILE\n", os.Args[0])
	}
	env, err := run("prelude.qn", nil)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = run(os.Args[1], env)
	return err
}

func run(path string, env *runtime.Environment) (*runtime.Environment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lines := make([]string, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := parser.Parse(parser.NewLexer(f.Name(), bufio.NewReader(f)))
	if err != nil {
		return nil, err
	}
	if err := runtime.RegisterLineInfo(f.Name(), lines); err != nil {
		return nil, err
	}
	return runtime.Run(env, b)
}
