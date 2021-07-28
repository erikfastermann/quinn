package parser

import (
	"errors"
	"fmt"
	"io"
)

const internal = "internal error"

func must(err error) {
	if err != nil {
		panic(internal + ": " + err.Error())
	}
}

func Parse(lexer *Lexer) (Block, error) {
	p := &parser{lexer}
	b, err := p.block(false)
	if err == io.EOF {
		return b, nil
	}
	return b, fmt.Errorf("read until line %d: %w", p.l.r.line, err)
}

type parser struct {
	l *Lexer
}

func (p *parser) groupOrSimplify(explicitBracket bool, errorOnSymbol bool) (Element, error) {
	g, err := p.group(explicitBracket, errorOnSymbol)
	if err != nil {
		return g, err
	}
	return simplifyGroup(g), nil
}

func simplifyGroup(g group) Element {
	switch len(g) {
	case 0:
		return Unit{}
	case 1:
		return g[0]
	default:
		return Call{First: g[0], Args: g[1:]}
	}
}

func (p *parser) group(explicitBracket bool, errorOnSymbol bool) (group, error) {
	var g group

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF && explicitBracket {
				return g, errors.New("missing ')'")
			}
			return g, err
		}

		switch v := t.(type) {
		case Symbol:
			if errorOnSymbol {
				return g, fmt.Errorf("more than 1 symbol (%s) in group, use extra brackets", v)
			}

			rhs, err := p.groupOrSimplify(explicitBracket, true)
			op := Operator{simplifyGroup(g), v, rhs}
			g = group{op}
			if err != nil {
				return g, err
			}
			// TODO: check if operator has empty left/right hand side?
			return g, nil
		case ClosedBracket:
			if !explicitBracket {
				return g, errors.New("unexpected ')'")
			}
			return g, nil
		case OpenBracket:
			e, err := p.groupOrSimplify(true, false)
			g = append(g, e)
			if err != nil {
				return g, err
			}
		case EndOfLine:
			if !explicitBracket {
				return g, nil
			}
		case Atom, String, Number, Ref:
			g = append(g, v.(Element))
		case OpenCurly:
			b, err := p.block(true)
			g = append(g, b)
			if err != nil {
				return g, err
			}
		case OpenSquare:
			l, err := p.list()
			g = append(g, l)
			if err != nil {
				return g, err
			}
		case ClosedCurly:
			if explicitBracket {
				return g, errors.New("unexpected '}'")
			}
			p.l.Unread()
			return g, nil
		case ClosedSquare:
			return g, errors.New("unexpected ']'")
		default:
			panic(internal)
		}
	}
}

func (p *parser) list() (List, error) {
	var l List

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF {
				return l, errors.New("missing ']'")
			}
			return l, err
		}

		switch v := t.(type) {
		case Symbol:
			return l, errors.New("bare operator not allowed in list, enclose in brackets")
		case ClosedBracket:
			return l, errors.New("unexpected ']'")
		case OpenBracket:
			e, err := p.groupOrSimplify(true, false)
			l = append(l, e)
			if err != nil {
				return l, err
			}
		case EndOfLine:
		case Atom, Ref, String, Number:
			l = append(l, v.(Element))
		case OpenCurly:
			b, err := p.block(true)
			l = append(l, b)
			if err != nil {
				return l, err
			}
		case OpenSquare:
			ll, err := p.list()
			l = append(l, ll)
			if err != nil {
				return l, err
			}
		case ClosedCurly:
			return l, errors.New("unexpected '}'")
		case ClosedSquare:
			return l, nil
		default:
			panic(internal)
		}
	}
}

func (p *parser) block(explicitCurly bool) (Block, error) {
	var b Block

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF && explicitCurly {
				return b, errors.New("missing '}'")
			}
			return b, err
		}

		switch t.(type) {
		case EndOfLine:
		case ClosedCurly:
			if !explicitCurly {
				return b, errors.New("unexpected '}'")
			}
			return b, nil
		case ClosedBracket:
			return b, errors.New("unexpected ')'")
		case ClosedSquare:
			return b, errors.New("unexpected ']'")
		default:
			p.l.Unread()
			e, err := p.groupOrSimplify(false, false)
			b = append(b, e)
			if err != nil {
				return b, err
			}
		}
	}
}
