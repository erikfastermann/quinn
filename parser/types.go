package parser

import (
	"fmt"

	"github.com/erikfastermann/quinn/number"
)

type Positioned interface {
	Position() (path string, line, column int)
}

type PositionedError struct {
	Path         string
	Line, Column int
	err          error
}

func errorf(p Positioned, format string, v ...interface{}) error {
	path, line, col := p.Position()
	return PositionedError{path, line, col, fmt.Errorf(format, v...)}
}

func (e PositionedError) Error() string {
	return fmt.Sprintf("%s|%d col %d| %v", e.Path, e.Line, e.Column, e.err)
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
	Path         string
	Line, Column int
	V            string
}

func (Ref) element() {}

func (Ref) token() {}

func (r Ref) Position() (string, int, int) { return r.Path, r.Line, r.Column }

type Atom struct {
	Path         string
	Line, Column int
	V            string
}

func (Atom) element() {}

func (Atom) token() {}

func (a Atom) Position() (string, int, int) { return a.Path, a.Line, a.Column }

type Number struct {
	Path         string
	Line, Column int
	V            number.Number
}

func (Number) element() {}

func (Number) token() {}

func (n Number) Position() (string, int, int) { return n.Path, n.Line, n.Column }

type String struct {
	Path         string
	Line, Column int
	V            string
}

func (String) element() {}

func (String) token() {}

func (s String) Position() (string, int, int) { return s.Path, s.Line, s.Column }

type Symbol struct {
	Path         string
	Line, Column int
	V            string
}

func (Symbol) token() {}

func (s Symbol) Position() (string, int, int) { return s.Path, s.Line, s.Column }

type OpenBracket struct {
	Path         string
	Line, Column int
}

func (OpenBracket) token() {}

func (ob OpenBracket) Position() (string, int, int) { return ob.Path, ob.Line, ob.Column }

type ClosedBracket struct {
	Path         string
	Line, Column int
}

func (ClosedBracket) token() {}

func (cb ClosedBracket) Position() (string, int, int) { return cb.Path, cb.Line, cb.Column }

type OpenCurly struct {
	Path         string
	Line, Column int
}

func (OpenCurly) token() {}

func (oc OpenCurly) Position() (string, int, int) { return oc.Path, oc.Line, oc.Column }

type ClosedCurly struct {
	Path         string
	Line, Column int
}

func (ClosedCurly) token() {}

func (cc ClosedCurly) Position() (string, int, int) { return cc.Path, cc.Line, cc.Column }

type OpenSquare struct {
	Path         string
	Line, Column int
}

func (OpenSquare) token() {}

func (os OpenSquare) Position() (string, int, int) { return os.Path, os.Line, os.Column }

type ClosedSquare struct {
	Path         string
	Line, Column int
}

func (ClosedSquare) token() {}

func (cs ClosedSquare) Position() (string, int, int) { return cs.Path, cs.Line, cs.Column }

type EndOfLine struct {
	Path         string
	Line, Column int
}

func (EndOfLine) token() {}

func (eol EndOfLine) Position() (string, int, int) { return eol.Path, eol.Line, eol.Column }

type Unit struct {
	Path         string
	Line, Column int
}

func (Unit) element() {}

func (u Unit) Position() (string, int, int) { return u.Path, u.Line, u.Column }

type Call struct {
	Path         string
	Line, Column int
	First        Element
	Args         []Element
}

func (Call) element() {}

func (c Call) Position() (string, int, int) { return c.Path, c.Line, c.Column }

type List struct {
	Path         string
	Line, Column int
	V            []Element
}

func (List) element() {}

func (l List) Position() (string, int, int) { return l.Path, l.Line, l.Column }

type Block struct {
	Path         string
	Line, Column int
	V            []Element
}

func (Block) element() {}

func (b Block) Position() (string, int, int) { return b.Path, b.Line, b.Column }
