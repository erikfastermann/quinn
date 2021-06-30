package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	l := NewLexer(bytes.NewReader(b))
	for {
		t, err := l.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(os.Stderr, err)
		}
		if _, ok := t.(EndOfLine); ok {
			fmt.Println()
		} else {
			fmt.Printf("%s ", t)
		}
	}
	fmt.Println()

	block, err := Parse(NewLexer(bytes.NewReader(b)))
	fmt.Println(block.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}
