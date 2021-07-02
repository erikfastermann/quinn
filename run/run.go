package run

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/erikfastermann/quinn/parser"
)

// TODO:
//	better printing for vals
//	define function with args
//	Number/parser.Number as pointer

const internal = "internal error"

type Value interface {
	value()
}

type namedValue struct {
	name  *name
	value Value // nil == no underlying value
}

func (v namedValue) get() (Value, error) {
	if v.value == nil {
		// TODO: better error message with possible name
		return nil, errors.New("no value")
	}
	return v.value, nil
}

func (v namedValue) onlyName() (string, error) {
	if v.name == nil || v.value != nil {
		if v.name == nil {
			return "", fmt.Errorf("value %#v is unnamed", v.value)
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

type Unit struct{}

func (Unit) value() {}

type Bool bool

func (Bool) value() {}

type String string

func (String) value() {}

// TODO: could use unsafe to cast *parser.Number to *Number

type Number struct {
	big.Rat
}

func (*Number) value() {}

type List struct {
	data []namedValue
}

func (List) value() {}

type Block struct {
	env    *environment
	fromGo func(*environment, []namedValue) (namedValue, error)
	code   parser.Block
}

func (*Block) value() {}

func (b *Block) run(local *environment, args []namedValue) (namedValue, error) {
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
			if _, ok := v.value.(Unit); !ok {
				// TODO: better error message with possible name
				return v, fmt.Errorf("non unit value %#v in not last group of block", v.value)
			}
		}
	}
	return namedValue{nil, Unit{}}, nil
}

func evalGroup(env *environment, group parser.Group) (namedValue, error) {
	switch len(group) {
	case 0:
		return namedValue{nil, Unit{}}, nil
	case 1:
		return evalElement(env, group[0])
	default:
		name, ok := group[0].(parser.Atom)
		if !ok {
			// TODO: parser: check atom is first
			panic(internal)
		}

		args := make([]namedValue, len(group)-1)
		for i, e := range group[1:] {
			v, err := evalElement(env, e)
			if err != nil {
				return v, err
			}
			args[i] = v
		}

		nameValue, ok := env.get(string(name))
		if !ok {
			return namedValue{}, fmt.Errorf("name %s not found", name)
		}
		block, ok := nameValue.value.(*Block)
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

func evalElement(env *environment, element parser.Element) (namedValue, error) {
	switch v := element.(type) {
	case parser.Atom:
		val, _ := env.get(string(v))
		return namedValue{&name{string(v), val.name}, val.value}, nil
	case parser.String:
		return namedValue{nil, String(v)}, nil
	case *parser.Number:
		return namedValue{nil, &Number{v.Rat}}, nil
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
			return namedValue{}, fmt.Errorf("unknown operator %s", v.Symbol)
		}
		block, ok := symbolValue.value.(*Block)
		if !ok {
			return symbolValue, fmt.Errorf(
				"operator %s is not a block, but a %#v value instead",
				v.Symbol,
				symbolValue,
			)
		}
		return block.run(env, []namedValue{lhsValue, rhsValue})
	case parser.List:
		l := make([]namedValue, len(v))
		for i, e := range v {
			v, err := evalElement(env, e)
			if err != nil {
				return v, err
			}
			l[i] = v
		}
		return namedValue{nil, List{l}}, nil
	case parser.Group:
		return evalGroup(env, v)
	case parser.Block:
		return namedValue{nil, &Block{env: env.merge(), code: v}}, nil
	default:
		panic(internal)
	}
}

// TODO: use persistent maps
type environment struct {
	outer map[string]namedValue
	local map[string]namedValue
}

func (e *environment) get(name string) (namedValue, bool) {
	v, ok := e.local[name]
	if !ok {
		v, ok = e.outer[name]
	}
	return v, ok
}

func (e *environment) put(name string, v namedValue) bool {
	_, ok0 := e.local[name]
	_, ok1 := e.outer[name]
	if ok0 || ok1 {
		return false
	}
	if e.local == nil {
		e.local = make(map[string]namedValue)
	}
	e.local[name] = v
	return true
}

func (e *environment) merge() *environment {
	outer := e.outer
	if len(e.local) > 0 {
		outer = make(map[string]namedValue, len(e.outer)+len(e.local))
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
	fn   func(*environment, []namedValue) (namedValue, error)
}{
	{"=", func(env *environment, args []namedValue) (namedValue, error) {
		if len(args) != 2 {
			panic(internal) // op can only be called with 2 args
		}
		assignee, err := args[0].onlyName()
		if err != nil {
			return namedValue{}, fmt.Errorf("can't assign to name, %w", err)
		}
		if ok := env.put(assignee, args[1]); !ok {
			return namedValue{}, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		return namedValue{nil, Unit{}}, nil
	}},
	{"+", func(_ *environment, args []namedValue) (namedValue, error) {
		if len(args) != 2 {
			panic(internal) // op can only be called with 2 args
		}
		xValue, err := args[0].get()
		if err != nil {
			return namedValue{}, fmt.Errorf("can't add, %w", err)
		}
		yValue, err := args[1].get()
		if err != nil {
			return namedValue{}, fmt.Errorf("can't add, %w", err)
		}
		x, ok := xValue.(*Number)
		if !ok {
			return args[0], fmt.Errorf("can't add, %#v is not a number", args[0])
		}
		y, ok := yValue.(*Number)
		if !ok {
			return args[1], fmt.Errorf("can't add, %#v is not a number", args[1])
		}
		var z big.Rat
		z.Add(&x.Rat, &y.Rat)
		return namedValue{nil, &Number{z}}, nil
	}},
	{"println", func(_ *environment, args []namedValue) (namedValue, error) {
		for i, v := range args {
			_, err := fmt.Print(v.value)
			if err != nil {
				return namedValue{}, err
			}
			if i < len(args)-1 {
				fmt.Print(" ")
			}
		}
		fmt.Println()
		return namedValue{nil, Unit{}}, nil
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
		v := namedValue{nil, &Block{fromGo: builtin.fn}}
		if ok := b.env.put(builtin.name, v); !ok {
			panic(internal)
		}
	}
	_, err := b.run(nil, nil)
	return err
}
