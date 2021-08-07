package parser

import (
	"errors"
	"io"
)

const internal = "internal error"

type startingPosition struct {
	path string
}

func (sp startingPosition) Position() (string, int, int) { return sp.path, 1, 1 }

func Parse(l *Lexer) (Block, error) {
	p := &parser{l}
	b, err := p.block(startingPosition{l.path}, false)
	if err != nil {
		return Block{}, err
	}
	return b.(Block), nil
}

type parser struct {
	l *Lexer
}

func (p *parser) block(pos Positioned, explicitCurly bool) (Element, error) {
	path, line, col := pos.Position()
	b := Block{Path: path, Line: line, Column: col}

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF {
				if explicitCurly {
					return nil, errors.New("missing '}'")
				}
				return b, nil
			}
			return nil, err
		}

		switch t.(type) {
		case EndOfLine:
		case ClosedCurly:
			if !explicitCurly {
				return nil, errorf(t, "unexpected '}'")
			}
			return b, nil
		case ClosedBracket:
			return nil, errorf(t, "unexpected ')'")
		case ClosedSquare:
			return nil, errorf(t, "unexpected ']'")
		default:
			p.l.Unread()
			e, err := p.canonicalizeGroup(t, false, false)
			if err != nil {
				return nil, err
			}
			b.V = append(b.V, e)
		}
	}
}

func (p *parser) canonicalizeGroup(pos Positioned, explicitBracket bool, errorOnSymbol bool) (Element, error) {
	e, err := p.group(pos, explicitBracket, errorOnSymbol)
	if err != nil {
		return nil, err
	}
	return canonicalizeGroup(pos, e), nil
}

func canonicalizeGroup(p Positioned, e []Element) Element {
	path, line, column := p.Position()
	switch len(e) {
	case 0:
		return Unit{path, line, column}
	case 1:
		return e[0]
	default:
		return Call{
			Path:   path,
			Line:   line,
			Column: column,
			First:  e[0],
			Args:   e[1:],
		}
	}
}

func (p *parser) group(pos Positioned, explicitBracket bool, errorOnSymbol bool) ([]Element, error) {
	var g []Element

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF {
				if explicitBracket {
					return nil, errors.New("missing ')'")
				}
				return g, nil
			}
			return nil, err
		}

		switch v := t.(type) {
		case Symbol:
			// TODO: check if operator has empty left/right side?

			if errorOnSymbol {
				return nil, errorf(t, "more than 1 symbol (%s) in group, use extra brackets", v.V)
			}

			lhs := canonicalizeGroup(pos, g)
			// TODO: better position for rhs
			rhs, err := p.canonicalizeGroup(t, explicitBracket, true)
			if err != nil {
				return nil, err
			}
			path, line, col := pos.Position()
			call := Call{
				Path:   path,
				Line:   line,
				Column: col,
				First:  Ref(v),
				Args:   []Element{lhs, rhs},
			}
			return []Element{call}, nil
		case ClosedBracket:
			if !explicitBracket {
				return nil, errorf(t, "unexpected ')'")
			}
			return g, nil
		case OpenBracket:
			e, err := p.canonicalizeGroup(t, true, false)
			if err != nil {
				return nil, err
			}
			g = append(g, e)
		case EndOfLine:
			if !explicitBracket {
				return g, nil
			}
		case Atom, String, Number, Ref:
			g = append(g, v.(Element))
		case OpenCurly:
			b, err := p.block(t, true)
			if err != nil {
				return nil, err
			}
			g = append(g, b)
		case OpenSquare:
			l, err := p.list(t)
			if err != nil {
				return nil, err
			}
			g = append(g, l)
		case ClosedCurly:
			if explicitBracket {
				return nil, errorf(t, "unexpected '}'")
			}
			p.l.Unread()
			return g, nil
		case ClosedSquare:
			return nil, errorf(t, "unexpected ']'")
		default:
			panic(internal)
		}
	}
}

func (p *parser) list(pos Positioned) (Element, error) {
	path, line, col := pos.Position()
	l := List{Path: path, Line: line, Column: col}

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF {
				return nil, errors.New("missing ']'")
			}
			return nil, err
		}

		switch v := t.(type) {
		case Symbol:
			return nil, errorf(t, "bare operator not allowed in list, enclose in brackets")
		case ClosedBracket:
			return nil, errorf(t, "unexpected ']'")
		case OpenBracket:
			e, err := p.canonicalizeGroup(t, true, false)
			if err != nil {
				return nil, err
			}
			l.V = append(l.V, e)
		case EndOfLine:
		case Atom, Ref, String, Number:
			l.V = append(l.V, v.(Element))
		case OpenCurly:
			b, err := p.block(t, true)
			if err != nil {
				return nil, err
			}
			l.V = append(l.V, b)
		case OpenSquare:
			ll, err := p.list(t)
			if err != nil {
				return nil, err
			}
			l.V = append(l.V, ll)
		case ClosedCurly:
			return l, errorf(t, "unexpected '}'")
		case ClosedSquare:
			return l, nil
		default:
			panic(internal)
		}
	}
}
