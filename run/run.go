package run

import (
	"fmt"
	"math/big"

	"github.com/erikfastermann/quinn/parser"
)

// TODO:
//	better printing for vals
//	define function with args

const internal = "internal error"

type Value interface {
	value()
}

type Unit struct{}

func (Unit) value() {}

type Bool bool

func (Bool) value() {}

type String string

func (String) value() {}

type Atom string

func (Atom) value() {}

// TODO: could use unsafe to cast *parser.Number to *Number

type Number struct {
	big.Rat
}

func (*Number) value() {}

type List struct {
	data []Value
}

func (List) value() {}

type Block struct {
	fromGo func(*environment, []Value) (Value, error)

	env  *environment
	code parser.Block
}

func (*Block) value() {}

func (b *Block) run(local *environment, args []Value) (Value, error) {
	if b.fromGo != nil {
		return b.fromGo(local, args)
	}

	env := b.env.merge()
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
	return Unit{}, nil
}

func evalGroup(env *environment, group parser.Group) (Value, error) {
	switch len(group) {
	case 0:
		return Unit{}, nil
	case 1:
		return evalElement(env, group[0])
	default:
		name, ok := group[0].(parser.Atom)
		if !ok {
			// TODO: parser: check atom is first
			panic(internal)
		}

		args := make([]Value, len(group)-1)
		for i, e := range group[1:] {
			v, err := evalElement(env, e)
			if err != nil {
				return v, err
			}
			args[i] = v
		}

		nameValue, ok := env.get(string(name))
		if !ok {
			return nil, fmt.Errorf("name %s not found", name)
		}
		block, ok := nameValue.(*Block)
		if !ok {
			return nameValue, fmt.Errorf(
				"name %s is not a block, but a %#v value instead",
				name,
				nameValue,
			)
		}
		return block.run(env, args)
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

var builtins = []struct {
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
		return Unit{}, nil
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
			_, err := fmt.Print(v)
			if err != nil {
				return nil, err
			}
			if i < len(args)-1 {
				fmt.Print(" ")
			}
		}
		fmt.Println()
		return Unit{}, nil
	}},
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
//		() (String) (Number) (List) (Block) (Operator)
//	stored as name (previous names are also stored) and underlying value (if any):
//		(Atom)
//	evaluated (arguments treated like this as well) and stored as value:
//		(Atom ...)
func Run(block parser.Block) error {
	b := Block{env: new(environment), code: block}
	for _, builtin := range builtins {
		v := &Block{fromGo: builtin.fn}
		if ok := b.env.put(builtin.name, v); !ok {
			panic(internal)
		}
	}
	_, err := b.run(nil, nil)
	return err
}
