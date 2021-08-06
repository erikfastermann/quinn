package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/erikfastermann/quinn/number"
)

type Lexer struct {
	r              io.RuneScanner // assumed to always return EOF after first EOF
	line, column   int
	lastWasNewline bool

	lastToken    Token
	useLastToken bool
}

func NewLexer(r io.RuneScanner) *Lexer {
	return &Lexer{r: r, line: 1, column: 1}
}

func (l *Lexer) readRune() (ch rune, line, column int, err error) {
	ch, _, err = l.r.ReadRune()
	if err != nil {
		return 0, 0, 0, err
	}

	if ch == '\n' {
		if l.lastWasNewline {
			line = l.line
			l.line++
			l.column = 1
			return ch, line, l.column, nil
		} else {
			line = l.line
			l.line++
			l.lastWasNewline = true
			return ch, line, l.column, nil
		}
	} else {
		if l.lastWasNewline {
			l.column = 2
			l.lastWasNewline = false
			return ch, l.line, 1, nil
		} else {
			column = l.column
			l.column++
			return ch, l.line, column, nil
		}
	}
}

func (l *Lexer) unreadRune() {
	if err := l.r.UnreadRune(); err != nil {
		panic(internal + ": " + err.Error())
	}
	if l.lastWasNewline {
		l.line--
		l.lastWasNewline = false
	} else {
		l.column--
	}
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
	ch, line, column, err := l.readRune()
	if err != nil {
		return nil, err
	}

	if unicode.IsSpace(ch) {
		if ch == '\n' {
			return EndOfLine{line, column}, nil
		}
		for {
			ch, line, column, err = l.readRune()
			if err != nil {
				return nil, err
			}
			if !unicode.IsSpace(ch) {
				break
			}
			if ch == '\n' {
				return EndOfLine{line, column}, nil
			}
		}
	}

	switch {
	case isReservedSymbol(ch):
		switch ch {
		case '"':
			var str strings.Builder
			for {
				ch, _, _, err := l.readRune()
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
			return String{line, column, str.String()}, nil
		case '\'':
			ch, _, _, err := l.readRune()
			if err != nil {
				if err == io.EOF {
					return nil, errBareTick
				}
				return nil, err
			}
			if !isCharStart(ch) {
				return nil, errBareTick
			}
			l.unreadRune()

			atom, err := l.takeStringWhile(isChar)
			if err != nil {
				return nil, err
			}
			return Atom{line, column, atom}, nil
		case '(':
			return OpenBracket{line, column}, nil
		case ')':
			return ClosedBracket{line, column}, nil
		case '[':
			return OpenSquare{line, column}, nil
		case ']':
			return ClosedSquare{line, column}, nil
		case '{':
			return OpenCurly{line, column}, nil
		case '}':
			return ClosedCurly{line, column}, nil
		case '#':
			for {
				ch, line, column, err := l.readRune()
				if err != nil {
					if err == io.EOF {
						return EndOfLine{line, column}, nil
					}
					return nil, err
				}
				if ch == '\n' {
					return EndOfLine{line, column}, nil
				}
			}
		default:
			panic(internal)
		}
	case isCharStart(ch):
		l.unreadRune()
		ref, err := l.takeStringWhile(isChar)
		if err != nil {
			return nil, err
		}
		return Ref{line, column, ref}, nil
	case isNumberStart(ch):
		// TODO: support hex, binary, octal
		// TODO: support .

		l.unreadRune()
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
		return Number{line, column, n}, nil
	case isSymbol(ch):
		l.unreadRune()
		symbol, err := l.takeStringWhile(isSymbol)
		if err != nil {
			return nil, err
		}
		return Symbol{line, column, symbol}, nil
	default:
		return nil, fmt.Errorf("unknown character %q", ch)
	}
}

func (l *Lexer) takeStringWhile(predicate func(rune) bool) (string, error) {
	var b strings.Builder
	for {
		ch, _, _, err := l.readRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if !predicate(ch) {
			l.unreadRune()
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
