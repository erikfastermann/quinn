package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type Element interface {
	element()
}

type Token interface {
	token()
}

type Atom string

func (_ Atom) element() {}
func (_ Atom) token()   {}

type Number struct {
	big.Int
}

func (_ *Number) element() {}
func (_ *Number) token()   {}

type String string

func (_ String) element() {}
func (_ String) token()   {}

type Symbol string

func (_ Symbol) token()   {}
func (_ Symbol) element() {}

type OpenBracket struct{}

func (_ OpenBracket) token() {}
func (b OpenBracket) String() string {
	return "("
}

type ClosedBracket struct{}

func (_ ClosedBracket) token() {}
func (b ClosedBracket) String() string {
	return ")"
}

type OpenCurly struct{}

func (_ OpenCurly) token() {}
func (c OpenCurly) String() string {
	return "{"
}

type ClosedCurly struct{}

func (_ ClosedCurly) token() {}
func (c ClosedCurly) String() string {
	return "}"
}

type OpenSquare struct{}

func (_ OpenSquare) token() {}
func (s OpenSquare) String() string {
	return "["
}

type ClosedSquare struct{}

func (_ ClosedSquare) token() {}
func (s ClosedSquare) String() string {
	return "]"
}

type EndOfLine struct{}

func (_ EndOfLine) token() {}

type Group []Element

func (_ Group) element() {}

type List []Element

func (_ List) element() {}

type Block []Group

func (_ Block) element() {}

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
	case Atom:
		return string(v)
	case String:
		return strconv.Quote(string(v))
	case Symbol:
		return string(v)
	case *Number:
		return v.String()
	case Group:
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

func must(err error) {
	if err != nil {
		panic("impl: " + err.Error())
	}
}

type scanner struct {
	r              io.RuneScanner
	line           int
	lastWasNewline bool
}

func (s *scanner) ReadRune() (rune, int, error) {
	ch, size, err := s.r.ReadRune()
	if ch == '\n' {
		s.line++
		s.lastWasNewline = true
	} else {
		s.lastWasNewline = false
	}
	return ch, size, err
}

func (s *scanner) UnreadRune() error {
	if err := s.r.UnreadRune(); err != nil {
		return err
	}
	if s.lastWasNewline {
		s.line--
		s.lastWasNewline = false
	}
	return nil
}

type Lexer struct {
	r            *scanner // assumed to always return io.EOF after first io.EOF
	lastToken    Token
	useLastToken bool
}

func NewLexer(r io.RuneScanner) *Lexer {
	return &Lexer{r: &scanner{r: r, line: 1}}
}

func (l *Lexer) Unread() {
	if l.lastToken == nil || l.useLastToken {
		panic("called Lexer.Unread without successfully calling Lexer.Next first")
	}
	l.useLastToken = true
}

func (l *Lexer) Next() (Token, error) {
	if l.useLastToken {
		t := l.lastToken
		l.useLastToken = false
		return t, nil
	}
	t, err := l.next()
	if err != nil {
		return nil, err
	}
	l.lastToken = t
	return t, nil
}

func (l *Lexer) next() (Token, error) {
	ch, _, err := l.r.ReadRune()
	if err != nil {
		return nil, err
	}

	if unicode.IsSpace(ch) {
		if ch == '\n' {
			return EndOfLine{}, nil
		}
		for {
			chch, _, err := l.r.ReadRune()
			if err != nil {
				return nil, err
			}
			if !unicode.IsSpace(chch) {
				ch = chch
				break
			}
			if chch == '\n' {
				return EndOfLine{}, nil
			}
		}
	}

	switch ch {
	case '"':
		var str strings.Builder
		for {
			ch, _, err := l.r.ReadRune()
			if err != nil {
				if err == io.EOF {
					return nil, errors.New("string not closed with \"")
				}
				return nil, err
			}
			if ch == '"' {
				break
			} else if ch == '\r' {
			} else {
				str.WriteRune(ch)
			}
		}
		return String(str.String()), nil
	case '(':
		return OpenBracket{}, nil
	case ')':
		return ClosedBracket{}, nil
	case '[':
		return OpenSquare{}, nil
	case ']':
		return ClosedSquare{}, nil
	case '{':
		return OpenCurly{}, nil
	case '}':
		return ClosedCurly{}, nil
	case '#':
		for {
			ch, _, err := l.r.ReadRune()
			if err != nil {
				return nil, err
			}
			if ch == '\n' {
				return EndOfLine{}, nil
			}
		}
	default:
		if ch >= '0' && ch <= '9' {
			// TODO: support hex, binary, octal
			// TODO: support .

			var num strings.Builder
			num.WriteRune(ch)
			for {
				ch, _, err := l.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						break
					}
					return nil, err
				}
				if (ch < '0' || ch > '9') && ch != '_' {
					must(l.r.UnreadRune())
					break
				}
				num.WriteRune(ch)
			}

			numStr := num.String()
			if len(numStr) > 1 && numStr[0] == '0' {
				return nil, fmt.Errorf("zero padded number %q", numStr)
			}
			var i big.Int
			if _, ok := i.SetString(numStr, 0); !ok {
				panic("impl")
			}
			return &Number{i}, nil
		} else if isAtomChar(ch) {
			var atom strings.Builder
			atom.WriteRune(ch)
			for {
				ch, _, err := l.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						break
					}
					return nil, err
				}
				if !isAtomChar(ch) {
					must(l.r.UnreadRune())
					break
				}
				atom.WriteRune(ch)
			}
			return Atom(atom.String()), nil
		} else if isSymbol(ch) {
			var symbol strings.Builder
			symbol.WriteRune(ch)
			for {
				ch, _, err := l.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						break
					}
					return nil, err
				}
				if !isSymbol(ch) {
					must(l.r.UnreadRune())
					break
				}
				symbol.WriteRune(ch)
			}
			return Symbol(symbol.String()), nil
		} else {
			return nil, fmt.Errorf("unknown character %q", ch)
		}
	}
}

func isAtomChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsNumber(ch) || ch == '_'
}

func isSymbol(ch rune) bool {
	switch ch {
	case '"', '(', ')', '{', '}', '[', ']', '_':
		return false
	default:
		return unicode.IsSymbol(ch) || unicode.IsPunct(ch)
	}
}

func Parse(lexer *Lexer) (Block, error) {
	p := &parser{lexer}
	b, err := p.block(false)
	if err == io.EOF {
		return b, nil
	}
	return b, fmt.Errorf("an error occured after reading to line %d: %w", p.l.r.line, err)
}

type parser struct {
	l *Lexer
}

func (p *parser) group(explicitBracket bool, errorOnSymbol bool) (Group, error) {
	var g Group

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF && explicitBracket {
				return g, errors.New("missing closed bracket")
			}
			return g, err
		}

		switch v := t.(type) {
		case Symbol:
			if errorOnSymbol {
				return g, fmt.Errorf("more than 1 symbol (%s) in group, use extra brackets", v)
			}

			rhs, err := p.group(explicitBracket, true)
			g = Group{g, v, rhs}
			if err != nil {
				return g, err
			}
			return g, nil
		case ClosedBracket:
			if !explicitBracket {
				return g, errors.New("unexpected closed bracket")
			}
			return g, nil
		case OpenBracket:
			gg, err := p.group(true, false)
			g = append(g, gg)
			if err != nil {
				return g, err
			}
		case EndOfLine:
			if !explicitBracket {
				return g, nil
			}
		case Atom, String, *Number:
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
				return g, errors.New("unexpected closed curly")
			}
			p.l.Unread()
			return g, nil
		case ClosedSquare:
			return g, errors.New("unexpected closed square")
		default:
			panic("impl")
		}
	}
}

func (p *parser) list() (List, error) {
	var l List

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF {
				return l, errors.New("missing closed square")
			}
			return l, err
		}

		switch v := t.(type) {
		case Symbol:
			return l, errors.New("bare operator not allowed in list, enclose in brackets")
		case ClosedBracket:
			return l, errors.New("unexpected closed bracket")
		case OpenBracket:
			g, err := p.group(true, false)
			l = append(l, g)
			if err != nil {
				return l, err
			}
		case EndOfLine:
		case Atom, String, *Number:
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
			return l, errors.New("unexpected closed curly")
		case ClosedSquare:
			return l, nil
		default:
			panic("impl")
		}
	}
}

func (p *parser) block(explicitCurly bool) (Block, error) {
	var b Block

	for {
		t, err := p.l.Next()
		if err != nil {
			if err == io.EOF && explicitCurly {
				return b, errors.New("missing closed curly")
			}
			return b, err
		}

		switch t.(type) {
		case EndOfLine:
		case ClosedCurly:
			if !explicitCurly {
				return b, errors.New("unexpected closed curly")
			}
			return b, nil
		case ClosedBracket:
			return b, errors.New("unexpected closed bracket")
		case ClosedSquare:
			return b, errors.New("unexpected closed square")
		default:
			p.l.Unread()
			g, err := p.group(false, false)
			b = append(b, g)
			if err != nil {
				return b, err
			}
		}
	}
}

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
