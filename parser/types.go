package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/erikfastermann/quinn/number"
)

type Element interface {
	element()
}

type Token interface {
	token()
}

type Ref string

func (Ref) element() {}
func (Ref) token()   {}

type Atom string

func (Atom) element() {}
func (Atom) token()   {}

type Number number.Number

func (Number) element() {}
func (Number) token()   {}

type String string

func (String) element() {}
func (String) token()   {}

type Symbol string

func (Symbol) token() {}

type OpenBracket struct{}

func (OpenBracket) token() {}
func (b OpenBracket) String() string {
	return "("
}

type ClosedBracket struct{}

func (ClosedBracket) token() {}
func (b ClosedBracket) String() string {
	return ")"
}

type OpenCurly struct{}

func (OpenCurly) token() {}
func (c OpenCurly) String() string {
	return "{"
}

type ClosedCurly struct{}

func (ClosedCurly) token() {}
func (c ClosedCurly) String() string {
	return "}"
}

type OpenSquare struct{}

func (OpenSquare) token() {}
func (s OpenSquare) String() string {
	return "["
}

type ClosedSquare struct{}

func (ClosedSquare) token() {}
func (s ClosedSquare) String() string {
	return "]"
}

type EndOfLine struct{}

func (EndOfLine) token() {}

type group []Element

func (group) element() {}

type Unit struct{}

func (Unit) element() {}

type Call struct {
	First Element
	Args  []Element
}

func (Call) element() {}

type Operator struct {
	Lhs    Element
	Symbol Symbol
	Rhs    Element
}

func (Operator) element() {}

type List []Element

func (List) element() {}

type Block []Element

func (Block) element() {}

func (b Block) String() string {
	return b.recString("")
}

func (b Block) recString(prefix string) string {
	newPrefix := prefix + "\t"
	var out strings.Builder
	out.WriteString("{")

	if len(b) > 1 {
		out.WriteString("\n")
	}
	for _, g := range b {
		if len(b) > 1 {
			out.WriteString(newPrefix)
		}
		out.WriteString(elementString(g, newPrefix))
		if len(b) > 1 {
			out.WriteString("\n")
		}
	}
	if len(b) > 1 {
		out.WriteString(prefix)
	}

	out.WriteString("}")
	return out.String()
}

func elementString(e Element, prefix string) string {
	switch v := e.(type) {
	case Ref:
		return string(v)
	case Atom:
		return string(v)
	case String:
		return strconv.Quote(string(v))
	case Number:
		return (number.Number(v)).String()
	case group:
		var b strings.Builder
		b.WriteString("(")
		for i, e := range v {
			b.WriteString(elementString(e, prefix))
			if i < len(v)-1 {
				b.WriteString(" ")
			}
		}
		b.WriteString(")")
		return b.String()
	case Operator:
		return fmt.Sprintf(
			"%s %s %s",
			elementString(v.Lhs, prefix),
			v.Symbol,
			elementString(v.Rhs, prefix),
		)
	case List:
		var b strings.Builder
		b.WriteString("[")
		for i, e := range v {
			b.WriteString(elementString(e, prefix))
			if i < len(v)-1 {
				b.WriteString(" ")
			}
		}
		b.WriteString("]")
		return b.String()
	case Block:
		return v.recString(prefix)
	default:
		return "<unknown>"
	}
}
