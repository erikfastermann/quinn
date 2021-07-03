package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/runtime"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer f.Close()

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

	block, err := parser.Parse(parser.NewLexer(bufio.NewReader(f)))
	// fmt.Println(block.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	if err := runtime.Run(block); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}
