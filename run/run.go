package run

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/erikfastermann/quinn/parser"
)

// TODO:
//	loops / conditionals
//	recursion
//	early returns
//	Value.Type
//	Block not with ptr recv?

const internal = "internal error"

type Value interface {
	value()

	Eq(Value) bool
	String() string
}

type Unit struct{}

func (Unit) value() {}

func (Unit) Eq(v Value) bool {
	_, isUnit := v.(Unit)
	return isUnit
}

func (Unit) String() string {
	return "()"
}

var unit Value = Unit{}

type Bool bool

func (Bool) value() {}

func (b Bool) Eq(v Value) bool {
	b2, ok := v.(Bool)
	return ok && b == b2
}

func (b Bool) String() string {
	if b {
		return "true"
	}
	return "false"
}

type String string

func (String) value() {}

func (s String) Eq(v Value) bool {
	s2, ok := v.(String)
	return ok && s == s2
}

func (s String) String() string {
	return strconv.Quote(string(s))
}

type Atom string

func (Atom) value() {}

func (a Atom) Eq(v Value) bool {
	a2, ok := v.(Atom)
	return ok && a == a2
}

func (a Atom) String() string {
	return string(a)
}

// TODO: could use unsafe to cast *parser.Number to *Number

type Number struct {
	big.Rat
}

func (*Number) value() {}

func (n *Number) Eq(v Value) bool {
	n2, ok := v.(*Number)
	return ok && n.Cmp(&n2.Rat) == 0
}

func (n *Number) String() string {
	return n.RatString()
}

type List struct {
	// TODO: use persistent array
	data []Value
}

func (List) value() {}

func (l List) Eq(v Value) bool {
	l2, ok := v.(List)
	if !ok || len(l.data) != len(l2.data) {
		return false
	}
	for i := range l.data {
		if !l.data[i].Eq(l2.data[i]) {
			return false
		}
	}
	return true
}

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

type Mut struct {
	v Value
}

func (*Mut) value() {}

func (*Mut) Eq(_ Value) bool {
	return false
}

func (m *Mut) String() string {
	return fmt.Sprintf("(mut %s)", m.v)
}

type Block struct {
	env    *environment
	fromGo func(*environment, []Value) (Value, error)
	code   parser.Block
}

func (*Block) value() {}

func (*Block) Eq(_ Value) bool {
	return false
}

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

	switch len(args) {
	case 0:
	case 1:
		if _, isUnit := args[0].(Unit); !isUnit {
			return nil, fmt.Errorf(
				"first argument in call to basic block must be unit, not %s",
				args[0],
			)
		}
	default:
		return nil, fmt.Errorf("too many arguments in call to basic block (%d)", len(args))
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
				return v, fmt.Errorf("non unit value %s in other than last group of block", v)
			}
		}
	}
	return unit, nil
}

var errArgumentedGoBlock = errors.New("can't create argumented block from an in Go defined block")

func (b *Block) withArgs(argNames []Atom) (Value, error) {
	if b.fromGo != nil {
		return nil, errArgumentedGoBlock
	}

	block := &(*b)
	env := block.env
	block.env = nil
	for _, a := range argNames {
		if _, ok := env.get(a); ok {
			return a, fmt.Errorf(
				"can't use %s as an argument, "+
					"already exists in the environment",
				a,
			)
		}
	}

	return &Block{fromGo: func(_ *environment, args []Value) (Value, error) {
		if len(args) != len(argNames) {
			return nil, fmt.Errorf(
				"expected %d arguments, got %d",
				len(argNames),
				len(args),
			)
		}

		env := env.merge()
		for i, a := range argNames {
			if !env.put(a, args[i]) {
				panic(internal)
			}
		}
		return block.run(env, nil)
	}}, nil
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

		var name Atom
		switch first := group[0].(type) {
		case parser.Atom:
			name = Atom(first)
		case parser.String:
			name = Atom(first)
		case parser.Block:
			return (&Block{env: env.merge(), code: first}).run(env, args)
		case parser.Group:
			blockV, err := evalGroup(env, first)
			if err != nil {
				return blockV, err
			}
			block, ok := blockV.(*Block)
			if !ok {
				return blockV, fmt.Errorf(
					"can't call value %s, not a block",
					blockV,
				)
			}
			return block.run(env, args)
		default:
			// TODO: parser: check atom/string/block/group is first
			panic(internal)
		}

		nameV, ok := env.get(name)
		if !ok {
			return nil, fmt.Errorf("name %s not found", name)
		}
		block, ok := nameV.(*Block)
		if !ok {
			return nameV, fmt.Errorf(
				"name %s is not a block, but a %s value instead",
				name,
				nameV,
			)
		}
		return block.run(env, args)
	}
}

func evalElement(env *environment, element parser.Element) (Value, error) {
	switch v := element.(type) {
	case parser.Atom:
		val, ok := env.get(Atom(v))
		if !ok {
			return Atom(string(v)), nil
		}
		return val, nil
	case parser.String:
		return String(v), nil
	case *parser.Number:
		return &Number{v.Rat}, nil
	case parser.Operator:
		rhsV, err := evalGroup(env, v.Rhs)
		if err != nil {
			return rhsV, err
		}
		lhsV, err := evalGroup(env, v.Lhs)
		if err != nil {
			return lhsV, err
		}

		symbolV, ok := env.get(Atom(v.Symbol))
		if !ok {
			return nil, fmt.Errorf("unknown operator %s", v.Symbol)
		}
		block, ok := symbolV.(*Block)
		if !ok {
			return symbolV, fmt.Errorf(
				"operator %s is not a block, but a %s value instead",
				v.Symbol,
				symbolV,
			)
		}
		return block.run(env, []Value{lhsV, rhsV})
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
	outer map[Atom]Value
	local map[Atom]Value
}

func (e *environment) get(name Atom) (Value, bool) {
	v, ok := e.local[name]
	if !ok {
		v, ok = e.outer[name]
	}
	return v, ok
}

func (e *environment) put(name Atom, v Value) bool {
	_, ok0 := e.local[name]
	_, ok1 := e.outer[name]
	if ok0 || ok1 {
		return false
	}
	if e.local == nil {
		e.local = make(map[Atom]Value)
	}
	e.local[name] = v
	return true
}

func (e *environment) merge() *environment {
	outer := e.outer
	if len(e.local) > 0 {
		outer = make(map[Atom]Value, len(e.outer)+len(e.local))
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
	name  Atom
	value Value
}{
	{"false", Bool(false)},
	{"true", Bool(true)},
}

var builtinBlocks = []struct {
	name Atom
	fn   func(*environment, []Value) (Value, error)
}{
	{"mut", func(env *environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("mut: expected one argument, got %d", len(args))
		}
		return &Mut{args[0]}, nil
	}},
	{"load", func(env *environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("load: expected one argument, got %d", len(args))
		}
		targetV := args[0]
		target, ok := targetV.(*Mut)
		if !ok {
			return targetV, fmt.Errorf("can't load from non mut %s", targetV)
		}
		return target.v, nil
	}},
	{"<-", func(env *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}
		targetV, v := args[0], args[1]
		target, ok := targetV.(*Mut)
		if !ok {
			return targetV, fmt.Errorf("can't store into non mut %s", targetV)
		}
		target.v = v
		return unit, nil
	}},
	{"=", func(env *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}
		assignee, ok := args[0].(Atom)
		if !ok {
			return args[0], fmt.Errorf("couldn't assign to name, %s is not an atom", args[0])
		}
		if ok := env.put(assignee, args[1]); !ok {
			return assignee, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		return unit, nil
	}},
	{"==", func(env *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}
		return Bool(args[0].Eq(args[1])), nil
	}},
	{"!=", func(env *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}
		return Bool(!args[0].Eq(args[1])), nil
	}},
	{"+", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("can't add, %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("can't add, %s is not a number", yV)
		}
		var z big.Rat
		z.Add(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"%%", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("modulo: %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("modulo: %s is not a number", yV)
		}
		if !x.IsInt() {
			return xV, fmt.Errorf(
				"modulo: %s is not an integer",
				x.RatString(),
			)
		}
		if !y.IsInt() {
			return yV, fmt.Errorf(
				"modulo: %s is not an integer",
				y.RatString(),
			)
		}
		if y.Num().IsInt64() && y.Num().Int64() == 0 {
			return yV, errors.New("modulo: denominator is zero")
		}

		var z big.Int
		z.Rem(x.Num(), y.Num())
		var r big.Rat
		r.SetInt(&z)
		return &Number{r}, nil
	}},
	{"->", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}
		defV, blockV := args[0], args[1]

		def, ok := defV.(List)
		if !ok {
			return defV, fmt.Errorf(
				"block defintion has to be a list, got %s",
				defV,
			)
		}
		atoms := make([]Atom, len(def.data))
		for i, v := range def.data {
			atom, ok := v.(Atom)
			if !ok {
				return v, fmt.Errorf("argument has to be atom, got %s", v)
			}
			atoms[i] = atom
		}

		block, ok := blockV.(*Block)
		if !ok {
			return blockV, fmt.Errorf(
				"expected block, got %s",
				blockV,
			)
		}
		return block.withArgs(atoms)
	}},
	{"defop", func(env *environment, args []Value) (Value, error) {
		// TODO: check symbol is valid operator

		if len(args) != 4 {
			return nil, fmt.Errorf("expected 4 arguments, got %d", len(args))
		}
		symbolV, lhsV, rhsV, blockV := args[0], args[1], args[2], args[3]
		symbol, ok := symbolV.(String)
		if !ok {
			return symbolV, fmt.Errorf("first argument must be string, got %s", symbolV)
		}
		lhs, ok := lhsV.(Atom)
		if !ok {
			return lhsV, fmt.Errorf("second argument must be atom, got %s", lhsV)
		}
		rhs, ok := rhsV.(Atom)
		if !ok {
			return rhsV, fmt.Errorf("third argument must be atom, got %s", rhsV)
		}
		block, ok := blockV.(*Block)
		if !ok {
			return blockV, fmt.Errorf("fourth argument must be block, got %s", blockV)
		}

		blockV, err := block.withArgs([]Atom{lhs, rhs})
		if err != nil {
			return blockV, err
		}
		if ok := env.put(Atom(symbol), blockV); !ok {
			return blockV, fmt.Errorf("couldn't assign to name, %s already exists", symbolV)
		}
		return unit, nil
	}},
	{"if", func(_ *environment, args []Value) (Value, error) {
		if len(args) < 2 || len(args) > 3 {
			return nil, fmt.Errorf("if: expected 2 or 3 arguments, got %d", len(args))
		}
		tBlock, ok := args[1].(*Block)
		if !ok {
			return args[1], fmt.Errorf("if: second argument must be a block, got %s", args[1])
		}

		_, isUnit := args[0].(Unit)
		if b, isBool := args[0].(Bool); (isBool && !bool(b)) || isUnit {
			if len(args) == 3 {
				fBlock, ok := args[2].(*Block)
				if !ok {
					return args[2], fmt.Errorf("if: third argument must be a block, got %s", args[2])
				}
				return fBlock.run(nil, nil)
			} else {
				return unit, nil
			}
		}
		return tBlock.run(nil, nil)
	}},
	{"each", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 3 {
			return nil, fmt.Errorf("each: expected 3 arguments, got %d", len(args))
		}
		nameV, listV, blockV := args[0], args[1], args[2]

		name, ok := nameV.(Atom)
		if !ok {
			return nameV, fmt.Errorf("each: first argument must be an atom, got %s", nameV)
		}
		list, ok := listV.(List)
		if !ok {
			return listV, fmt.Errorf("each: second argument must be a list, got %s", listV)
		}
		blockOrig, ok := blockV.(*Block)
		if !ok {
			return blockV, fmt.Errorf("each: third argument must be a block, got %s", blockV)
		}

		if blockOrig.fromGo != nil {
			return blockOrig, errArgumentedGoBlock
		}
		block := &(*blockOrig)
		env := block.env
		block.env = nil
		if _, ok := env.get(name); ok {
			return name, fmt.Errorf(
				"each: can't use %s as an argument, "+
					"already exists in the environment",
				name,
			)
		}

		for _, v := range list.data {
			env := env.merge()
			if !env.put(name, v) {
				panic(internal)
			}
			v, err := block.run(env, nil)
			if err != nil {
				return v, err
			}

			if _, isUnit := v.(Unit); !isUnit {
				return v, nil
			}
		}

		return unit, nil
	}},
	{"@", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("operator can only be called with 2 arguments")
		}
		listV, numV := args[0], args[1]
		list, ok := listV.(List)
		if !ok {
			return listV, fmt.Errorf("at: expected list, got %s", listV)
		}
		num, ok := numV.(*Number)
		var zero big.Int
		if !ok || !num.IsInt() || num.Num().Cmp(&zero) < 0 {
			return numV, fmt.Errorf("at: %s is not an unsigned integer", numV)
		}
		idx64 := num.Num().Int64()
		idx := int(idx64)
		if !num.Num().IsInt64() || int64(idx) != idx64 || idx >= len(list.data) {
			return numV, fmt.Errorf(
				"at: index out of range (%s with length %d)",
				num,
				len(list.data),
			)
		}
		return list.data[idx], nil
	}},
	{"len", func(_ *environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len: expected 1 argument, got %d", len(args))
		}
		lV := args[0]
		l, ok := lV.(List)
		if !ok {
			return lV, fmt.Errorf("len: expected list, got %s", lV)
		}
		var r big.Rat
		r.SetInt64(int64(len(l.data)))
		return &Number{r}, nil
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
//		(Atom ...) (Block ...) (Group ...) (String ...)
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
