package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/run"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	// l := NewLexer(bytes.NewReader(b))
	// for {
	// 	t, err := l.Next()
	// 	if err != nil {
	// 		if err == io.EOF {
	// 			break
	// 		}
	// 		fmt.Fprintln(os.Stderr, err)
	// 	}
	// 	if _, ok := t.(EndOfLine); ok {
	// 		fmt.Println()
	// 	} else {
	// 		fmt.Printf("%s ", t)
	// 	}
	// }
	// fmt.Println()

	block, err := parser.Parse(parser.NewLexer(bytes.NewReader(b)))
	fmt.Println(block.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	if err := run.Run(block); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}
