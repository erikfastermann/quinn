package parser

import (
	"fmt"

	"github.com/erikfastermann/quinn/number"
)

type Positioned interface {
	Position() (line, column int)
}

type PositionedError struct {
	Line, Column int
	err          error
}

func errorf(p Positioned, format string, v ...interface{}) error {
	l, c := p.Position()
	return PositionedError{l, c, fmt.Errorf(format, v...)}
}

func (e PositionedError) Error() string {
	return fmt.Sprintf("line %d column %d: %v", e.Line, e.Column, e.err)
}

func (e PositionedError) Unwrap() error {
	return e.err
}

type Element interface {
	element()
	Positioned
}

type Token interface {
	token()
	Positioned
}

type Ref struct {
	Line, Column int
	V            string
}

func (Ref) element() {}

func (Ref) token() {}

func (r Ref) Position() (int, int) { return r.Line, r.Column }

type Atom struct {
	Line, Column int
	V            string
}

func (Atom) element() {}

func (Atom) token() {}

func (a Atom) Position() (int, int) { return a.Line, a.Column }

type Number struct {
	Line, Column int
	V            number.Number
}

func (Number) element() {}

func (Number) token() {}

func (n Number) Position() (int, int) { return n.Line, n.Column }

type String struct {
	Line, Column int
	V            string
}

func (String) element() {}

func (String) token() {}

func (s String) Position() (int, int) { return s.Line, s.Column }

type Symbol struct {
	Line, Column int
	V            string
}

func (Symbol) token() {}

func (s Symbol) Position() (int, int) { return s.Line, s.Column }

type OpenBracket struct {
	Line, Column int
}

func (OpenBracket) token() {}

func (ob OpenBracket) Position() (int, int) { return ob.Line, ob.Column }

type ClosedBracket struct {
	Line, Column int
}

func (ClosedBracket) token() {}

func (cb ClosedBracket) Position() (int, int) { return cb.Line, cb.Column }

type OpenCurly struct {
	Line, Column int
}

func (OpenCurly) token() {}

func (oc OpenCurly) Position() (int, int) { return oc.Line, oc.Column }

type ClosedCurly struct {
	Line, Column int
}

func (ClosedCurly) token() {}

func (cc ClosedCurly) Position() (int, int) { return cc.Line, cc.Column }

type OpenSquare struct {
	Line, Column int
}

func (OpenSquare) token() {}

func (os OpenSquare) Position() (int, int) { return os.Line, os.Column }

type ClosedSquare struct {
	Line, Column int
}

func (ClosedSquare) token() {}

func (cs ClosedSquare) Position() (int, int) { return cs.Line, cs.Column }

type EndOfLine struct {
	Line, Column int
}

func (EndOfLine) token() {}

func (eol EndOfLine) Position() (int, int) { return eol.Line, eol.Column }

type Unit struct {
	Line, Column int
}

func (Unit) element() {}

func (u Unit) Position() (int, int) { return u.Line, u.Column }

type Call struct {
	Line, Column int
	First        Element
	Args         []Element
}

func (Call) element() {}

func (c Call) Position() (int, int) { return c.Line, c.Column }

type List struct {
	Line, Column int
	V            []Element
}

func (List) element() {}

func (l List) Position() (int, int) { return l.Line, l.Column }

type Block struct {
	Line, Column int
	V            []Element
}

func (Block) element() {}

func (b Block) Position() (int, int) { return b.Line, b.Column }
