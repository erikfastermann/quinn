package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/erikfastermann/quinn/number"
)

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
		panic(internal + ": called Lexer.Unread without successfully calling Lexer.Next first")
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

var errBareTick = errors.New("bare '")

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
			ch, _, err = l.r.ReadRune()
			if err != nil {
				return nil, err
			}
			if !unicode.IsSpace(ch) {
				break
			}
			if ch == '\n' {
				return EndOfLine{}, nil
			}
		}
	}

	switch {
	case isReservedSymbol(ch):
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
		case '\'':
			ch, _, err := l.r.ReadRune()
			if err != nil {
				if err == io.EOF {
					return nil, errBareTick
				}
				return nil, err
			}
			if !isCharStart(ch) {
				return nil, errBareTick
			}
			must(l.r.UnreadRune())

			atom, err := l.takeStringWhile(isChar)
			if err != nil {
				return nil, err
			}
			return Atom(atom), nil
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
					if err == io.EOF {
						return EndOfLine{}, nil
					}
					return nil, err
				}
				if ch == '\n' {
					return EndOfLine{}, nil
				}
			}
		default:
			panic(internal)
		}
	case isCharStart(ch):
		must(l.r.UnreadRune())
		ref, err := l.takeStringWhile(isChar)
		if err != nil {
			return nil, err
		}
		return Ref(ref), nil
	case isNumberStart(ch):
		// TODO: support hex, binary, octal
		// TODO: support .

		must(l.r.UnreadRune())
		num, err := l.takeStringWhile(isNumber)
		if err != nil {
			return nil, err
		}
		if len(num) > 1 && num[0] == '0' {
			return nil, fmt.Errorf("zero padded number %q", num)
		}

		n, err := number.FromString(num)
		if err != nil {
			panic(internal + ": " + err.Error())
		}
		return Number(n), nil
	case isSymbol(ch):
		must(l.r.UnreadRune())
		symbol, err := l.takeStringWhile(isSymbol)
		if err != nil {
			return nil, err
		}
		return Symbol(symbol), nil
	default:
		return nil, fmt.Errorf("unknown character %q", ch)
	}
}

func (l *Lexer) takeStringWhile(predicate func(rune) bool) (string, error) {
	var b strings.Builder
	for {
		ch, _, err := l.r.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if !predicate(ch) {
			must(l.r.UnreadRune())
			break
		}
		b.WriteRune(ch)
	}
	return b.String(), nil
}

func isCharStart(ch rune) bool {
	return !isNumberStart(ch) && (unicode.IsLetter(ch) || ch == '_')
}

func isChar(ch rune) bool {
	return unicode.IsLetter(ch) || isNumberStart(ch) || ch == '_'
}

func isReservedSymbol(ch rune) bool {
	switch ch {
	case '"', '\'', '(', ')', '{', '}', '[', ']', '#':
		return true
	default:
		return false
	}
}

func isSymbol(ch rune) bool {
	return !isReservedSymbol(ch) && ch != '_' && (unicode.IsSymbol(ch) || unicode.IsPunct(ch))
}

func isNumberStart(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isNumber(ch rune) bool {
	return (ch >= '0' && ch <= '9') || ch == '_'
}
