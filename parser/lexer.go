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
				if err == io.EOF {
					// assumed that subsequent reads will return io.EOF
					return EndOfLine{}, nil
				}
				return nil, err
			}
			if ch == '\n' {
				return EndOfLine{}, nil
			}
		}
	case '\'':
		// TODO: check first char is not a number

		var atom strings.Builder
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

			n, err := number.FromString(numStr)
			if err != nil {
				panic(internal + ": " + err.Error())
			}
			return Number(n), nil
		} else if isAtomChar(ch) {
			var ref strings.Builder
			ref.WriteRune(ch)
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
				ref.WriteRune(ch)
			}
			return Ref(ref.String()), nil
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
	case '"', '\'', '(', ')', '{', '}', '[', ']', '_':
		return false
	default:
		return unicode.IsSymbol(ch) || unicode.IsPunct(ch)
	}
}
