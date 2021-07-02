package run

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/erikfastermann/quinn/parser"
)

// TODO:
//	define function with args

const internal = "internal error"

type Value interface {
	value()
	String() string
}

type Unit struct{}

func (Unit) value() {}

func (Unit) String() string {
	return "()"
}

var unit Value = Unit{}

type Bool bool

func (Bool) value() {}

func (b Bool) String() string {
	if b {
		return "true"
	}
	return "false"
}

type String string

func (String) value() {}

func (s String) String() string {
	return strconv.Quote(string(s))
}

type Atom string

func (Atom) value() {}

func (a Atom) String() string {
	return string(a)
}

// TODO: could use unsafe to cast *parser.Number to *Number

type Number struct {
	big.Rat
}

func (*Number) value() {}

func (n *Number) String() string {
	return n.RatString()
}

type List struct {
	// TODO: use persistent array
	data []Value
}

func (List) value() {}

func (l List) String() string {
	var b strings.Builder
	b.WriteString("[")
	for i, v := range l.data {
		b.WriteString(v.String())
		if i < len(l.data)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("]")
	return b.String()
}

type Block struct {
	fromGo func(*environment, []Value) (Value, error)

	env  *environment
	code parser.Block
}

func (*Block) value() {}

func (*Block) String() string {
	return "<block>"
}

func (b *Block) run(local *environment, args []Value) (Value, error) {
	env := local
	if b.env != nil {
		env = b.env.merge()
	}

	if b.fromGo != nil {
		return b.fromGo(env, args)
	}

	for i, group := range b.code {
		v, err := evalGroup(env, group)
		if err != nil {
			return v, err
		}

		if i == len(b.code)-1 {
			return v, nil
		} else {
			if _, ok := v.(Unit); !ok {
				return v, fmt.Errorf("non unit value %#v in other than last group of block", v)
			}
		}
	}
	return unit, nil
}

func evalGroup(env *environment, group parser.Group) (Value, error) {
	switch len(group) {
	case 0:
		return unit, nil
	case 1:
		return evalElement(env, group[0])
	default:
		args := make([]Value, len(group)-1)
		for i, e := range group[1:] {
			v, err := evalElement(env, e)
			if err != nil {
				return v, err
			}
			args[i] = v
		}

		switch first := group[0].(type) {
		case parser.Block:
			return (&Block{env: env.merge(), code: first}).run(env, args)
		case parser.Group:
			blockValue, err := evalGroup(env, first)
			if err != nil {
				return blockValue, err
			}
			block, ok := blockValue.(*Block)
			if !ok {
				return blockValue, fmt.Errorf(
					"can't call value %#v, not a block",
					blockValue,
				)
			}
			return block.run(env, args)
		case parser.Atom:
			nameValue, ok := env.get(string(first))
			if !ok {
				return nil, fmt.Errorf("name %s not found", first)
			}
			block, ok := nameValue.(*Block)
			if !ok {
				return nameValue, fmt.Errorf(
					"name %s is not a block, but a %#v value instead",
					first,
					nameValue,
				)
			}
			return block.run(env, args)
		default:
			// TODO: parser: check atom is first
			panic(internal)
		}

	}
}

func evalElement(env *environment, element parser.Element) (Value, error) {
	switch v := element.(type) {
	case parser.Atom:
		val, ok := env.get(string(v))
		if !ok {
			return Atom(string(v)), nil
		}
		return val, nil
	case parser.String:
		return String(v), nil
	case *parser.Number:
		return &Number{v.Rat}, nil
	case parser.Operator:
		rhsValue, err := evalGroup(env, v.Rhs)
		if err != nil {
			return rhsValue, err
		}
		lhsValue, err := evalGroup(env, v.Lhs)
		if err != nil {
			return lhsValue, err
		}

		symbolValue, ok := env.get(string(v.Symbol))
		if !ok {
			return nil, fmt.Errorf("unknown operator %s", v.Symbol)
		}
		block, ok := symbolValue.(*Block)
		if !ok {
			return symbolValue, fmt.Errorf(
				"operator %s is not a block, but a %#v value instead",
				v.Symbol,
				symbolValue,
			)
		}
		return block.run(env, []Value{lhsValue, rhsValue})
	case parser.List:
		l := make([]Value, len(v))
		for i, e := range v {
			v, err := evalElement(env, e)
			if err != nil {
				return v, err
			}
			l[i] = v
		}
		return List{l}, nil
	case parser.Group:
		return evalGroup(env, v)
	case parser.Block:
		return &Block{env: env.merge(), code: v}, nil
	default:
		panic(internal)
	}
}

// TODO: use persistent maps
type environment struct {
	outer map[string]Value
	local map[string]Value
}

func (e *environment) get(name string) (Value, bool) {
	v, ok := e.local[name]
	if !ok {
		v, ok = e.outer[name]
	}
	return v, ok
}

func (e *environment) put(name string, v Value) bool {
	_, ok0 := e.local[name]
	_, ok1 := e.outer[name]
	if ok0 || ok1 {
		return false
	}
	if e.local == nil {
		e.local = make(map[string]Value)
	}
	e.local[name] = v
	return true
}

func (e *environment) merge() *environment {
	outer := e.outer
	if len(e.local) > 0 {
		outer = make(map[string]Value, len(e.outer)+len(e.local))
		for k, v := range e.outer {
			outer[k] = v
		}
		for k, v := range e.local {
			outer[k] = v
		}
	}
	return &environment{outer: outer}
}

var builtinOther = []struct {
	name  string
	value Value
}{
	{"false", Bool(false)},
	{"true", Bool(true)},
}

var builtinBlocks = []struct {
	name string
	fn   func(*environment, []Value) (Value, error)
}{
	{"=", func(env *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			panic(internal) // op can only be called with 2 args
		}
		assignee, ok := args[0].(Atom)
		if !ok {
			return nil, fmt.Errorf("can't assign to name, %#v is not an atom", args[0])
		}
		if ok := env.put(string(assignee), args[1]); !ok {
			return nil, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		return unit, nil
	}},
	{"+", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			panic(internal) // op can only be called with 2 args
		}

		xValue, yValue := args[0], args[1]
		x, ok := xValue.(*Number)
		if !ok {
			return xValue, fmt.Errorf("can't add, %#v is not a number", xValue)
		}
		y, ok := yValue.(*Number)
		if !ok {
			return yValue, fmt.Errorf("can't add, %#v is not a number", yValue)
		}
		var z big.Rat
		z.Add(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"println", func(_ *environment, args []Value) (Value, error) {
		for i, v := range args {
			_, err := fmt.Print(v.String())
			if err != nil {
				return nil, err
			}
			if i < len(args)-1 {
				fmt.Print(" ")
			}
		}
		fmt.Println()
		return unit, nil
	}},
}

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
//		() (String) (Number) (List) (Block) (Operator)
//	stored as name (previous names are also stored) and underlying value (if any):
//		(Atom)
//	evaluated (arguments treated like this as well) and stored as value:
//		(Atom ...)
func Run(block parser.Block) error {
	b := Block{env: new(environment), code: block}
	for _, builtin := range builtinBlocks {
		v := &Block{fromGo: builtin.fn}
		if ok := b.env.put(builtin.name, v); !ok {
			panic(internal)
		}
	}
	for _, builtin := range builtinOther {
		if ok := b.env.put(builtin.name, builtin.value); !ok {
			panic(internal)
		}
	}
	_, err := b.run(nil, nil)
	return err
}
