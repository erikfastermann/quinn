package run

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/erikfastermann/quinn/parser"
)

const internal = "internal error"

type Value interface {
	value()
}

func valueFromGroup(parser.Group) (Value, ok) {
}

type Unit struct{}

func (Unit) value() {}

type Bool bool

func (Bool) value() {}

type String string

func (String) value() {}

type Number big.Int

func (*Number) value() {}

type List []Value

func (List) value() {}

type Block struct {
	fromGo  func(*stack, []namedValue) error
	context map[string]namedValue
	code    Block
}

func (*Block) value() {}

type namedValue struct {
	name  *name
	value value // nil == no underlying value
}

func (v namedValue) onlyName() (string, error) {
	if v.name == nil || v.value != nil {
		if v.name == nil {
			return "", errors.New("value is unnamed")
		} else {
			return "", fmt.Errorf("%s has underlying value %#v", v.name.n, v.value)
		}
	}
	return v.name.n, nil
}

type name struct {
	n    string
	next *name
}

// lookup var
// copy environment

type stack struct {
	m map[string]namedValue
	s [][]string
}

func newStack() *stack {
	return &stack{
		m: make(map[string]namedValue),
		s: nil,
	}
}

func (s *stack) push() {
	s.s = append(s.s, nil)
}

func (s *stack) pop() {
	for _, n := range s.s[len(s.s)-1] {
		delete(s.m, n)
	}
	s.s = s.s[:len(s.s)-1]
	if len(s.s) == 0 {
		panic(internal)
	}
}

func (s *stack) get(name string) (namedValue, bool) {
	v, ok := s.m[name]
	return v, ok
}

func (s *stack) put(name string, v namedValue) bool {
	if _, ok := s.m[name]; ok {
		return false
	}
	s.m[name] = v
	s.s[len(s.s)-1] = append(s.s[len(s.s)-1], name)
	return true
}

// when adding blocks, all unknown vars are marked and attached to the block definition.
// calling a block places arguments internally,
// use an operator defintion and builtins to replace.
// evaluating an undefined var panics.
//
// a block is evaluated group by group.
// the last generated value in a block is returned automatically.
// a builtin can be used to return early.
// every other expression that doesn't evaluate to () panics.
// an empty block evaluates to ().
//
// the following groups are possible:
//	stored as value:
//		() (String) (Number) (List) (Block)
//	stored as name (previous names are also stored) and underlying value (if any):
//		(Atom)
//	evaluated (arguments treated like this as well) and stored as value:
//		(Atom ...) (Group Symbol Group)
func Run(block parser.Block) error {
	i := &interpreter{
		s: newStack(),
	}

	builtins := []struct {
		name string
		fn   func(args []namedValue) error
	}{
		{"=", func(args []namedValue) error {
			if len(args) != 2 {
				panic(internal) // op can only be called with 2 args
			}
			assignee, err := args[0].onlyName()
			if err != nil {
				return fmt.Errorf("can't assign to name, %w", err)
			}
			if ok := i.s.put(assignee, args[1]); !ok {
				return fmt.Errorf("couldn't assign to name, %s already exists", assignee)
			}
			return nil
		}},
	}
	for _, builtin := range builtins {
		v := namedValue{nil, Block{fromGo: builtin.fn}}
		if ok := i.s.put(builtin.name, v); !ok {
			panic(internal)
		}
	}

	return r.run(block)
}

type interpreter struct {
	s *stack
}

func (i *interpreter) run(block parser.Block, args []namedValue) error {
	i.s.push()
	defer i.s.pop()

	for _, group := range block {
		switch len(group) {
		case 0:
			return Unit, nil
		case 1:
		case 3:
			_, ok0 := group[0].(parser.Group)
			_, ok1 := group[1].(parser.Symbol)
			_, ok2 := group[2].(parser.Group)
			if !ok0 || !ok1 || !ok2 {
				fallthrough
			}
		default:
			name, ok := group[0].(parser.Atom)
			if !ok {
				panic("TODO: parser: check atom is first")
			}
		}
	}
}
