package runtime

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

func (n *Number) asUnsigned() (int, error) {
	if !n.IsInt() {
		return 0, fmt.Errorf("%s is not an integer", n)
	}
	num := n.Num()
	if num.Sign() < 0 {
		return 0, fmt.Errorf("%s is smaller than 0", n)
	}
	i64 := num.Int64()
	if !num.IsInt64() || int64(int(i64)) != i64 {
		return 0, fmt.Errorf("%s is too large", n)
	}
	return int(i64), nil
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

// TODO: implement and use exceptions instead
type IterationStop struct{}

func (IterationStop) value() {}

func (IterationStop) Eq(v Value) bool {
	_, ok := v.(IterationStop)
	return ok
}

func (IterationStop) String() string {
	return "iteration stop"
}

// TODO:
// type Exception struct {
// 	Err Value
// }

// func (Exception) value() {}

// func (e Exceprion) Eq(v Value) bool {
// 	e2, ok := v.(Exception)
// 	return ok && e.Err.Eq(e2.Err)
// }

// func (e Excepion) String() string {
// 	return fmt.Sprintf("(exception %s)", e.Err)
// }

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
	fromGo func(**environment, []Value) (Value, error)
	code   parser.Block
}

func (Block) value() {}

func (Block) Eq(_ Value) bool {
	return false
}

func (Block) String() string {
	return "<block>"
}

func (b Block) run(local **environment, args []Value) (Value, error) {
	env := local
	if b.env != nil {
		currentEnv := b.env
		env = &currentEnv
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

func (b Block) withArgs(argNames []Atom) (Value, error) {
	if b.fromGo != nil {
		return nil, errors.New("can't create argumented block from an in Go defined block")
	}

	block := b
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

	return Block{fromGo: func(_ **environment, args []Value) (Value, error) {
		if len(args) != len(argNames) {
			return nil, fmt.Errorf(
				"expected %d arguments, got %d",
				len(argNames),
				len(args),
			)
		}

		currentEnv, ok := env, false
		for i, a := range argNames {
			currentEnv, ok = currentEnv.insert(a, args[i])
			if !ok {
				panic(internal)
			}
		}
		return block.run(&currentEnv, nil)
	}}, nil
}

func evalGroup(env **environment, group parser.Group) (Value, error) {
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
			return (&Block{env: *env, code: first}).run(nil, args)
		case parser.Group:
			blockV, err := evalGroup(env, first)
			if err != nil {
				return blockV, err
			}
			block, ok := blockV.(Block)
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

		nameV, ok := (*env).get(name)
		if !ok {
			return nil, fmt.Errorf("name %s not found", name)
		}
		block, ok := nameV.(Block)
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

func evalElement(env **environment, element parser.Element) (Value, error) {
	switch v := element.(type) {
	case parser.Atom:
		val, ok := (*env).get(Atom(v))
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

		symbolV, ok := (*env).get(Atom(v.Symbol))
		if !ok {
			return nil, fmt.Errorf("unknown operator %s", v.Symbol)
		}
		block, ok := symbolV.(Block)
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
		return Block{env: *env, code: v}, nil
	default:
		panic(internal)
	}
}

type environment struct {
	// TODO: use persistent map
	// TODO: iterative

	key         Atom
	value       Value
	left, right *environment
}

func (env *environment) get(k Atom) (Value, bool) {
	if env == nil {
		return nil, false
	}

	if k < env.key {
		return env.left.get(k)
	} else if k > env.key {
		return env.right.get(k)
	} else {
		return env.value, true
	}
}

func (env *environment) insert(k Atom, v Value) (*environment, bool) {
	if env == nil {
		return &environment{k, v, nil, nil}, true
	}

	if k < env.key {
		next, ok := env.left.insert(k, v)
		if !ok {
			return nil, false
		}
		return &environment{env.key, env.value, next, env.right}, true
	} else if k > env.key {
		next, ok := env.right.insert(k, v)
		if !ok {
			return nil, false
		}
		return &environment{env.key, env.value, env.left, next}, true
	} else {
		return nil, false
	}
}

func (env *environment) String() string {
	s := env.stringRec()
	if len(s) == 0 {
		return "(map ())"
	}
	return fmt.Sprintf("(map %s)", s)
}

func (env *environment) stringRec() string {
	if env == nil {
		return ""
	}

	left := env.left.stringRec()
	str := left
	if len(left) > 0 {
		str += " "
	}

	str += fmt.Sprintf("[%s %v]", env.key, env.value)

	right := env.right.stringRec()
	if len(right) > 0 {
		str += " "
	}
	str += right

	return str
}

var builtinOther = []struct {
	name  Atom
	value Value
}{
	{"false", Bool(false)},
	{"true", Bool(true)},
	{"stop", IterationStop{}},
}

var errOperatorArgumentsLength = errors.New("operator can only be called with 2 arguments")

var builtinBlocks = []struct {
	name Atom
	fn   func(**environment, []Value) (Value, error)
}{
	{"mut", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("mut: expected one argument, got %d", len(args))
		}
		return &Mut{args[0]}, nil
	}},
	{"load", func(_ **environment, args []Value) (Value, error) {
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
	{"<-", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}
		targetV, v := args[0], args[1]
		target, ok := targetV.(*Mut)
		if !ok {
			return targetV, fmt.Errorf("can't store into non mut %s", targetV)
		}
		target.v = v
		return unit, nil
	}},
	{"=", func(env **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}
		assignee, ok := args[0].(Atom)
		if !ok {
			return args[0], fmt.Errorf("couldn't assign to name, %s is not an atom", args[0])
		}
		nextEnv, ok := (*env).insert(assignee, args[1])
		if !ok {
			return assignee, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		*env = nextEnv
		return unit, nil
	}},
	{"==", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}
		return Bool(args[0].Eq(args[1])), nil
	}},
	{"!=", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}
		return Bool(!args[0].Eq(args[1])), nil
	}},
	{">=", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("greater or equal: %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("greater or equal: %s is not a number", yV)
		}
		return Bool(x.Cmp(&y.Rat) >= 0), nil
	}},
	{"not", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("not: expected 1 argument, got %d", len(args))
		}
		bV := args[0]
		b, ok := bV.(Bool)
		if !ok {
			return bV, fmt.Errorf("not: first argument must be bool, not %s", bV)
		}
		return !b, nil
	}},
	{"+", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("add: %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("add: %s is not a number", yV)
		}
		var z big.Rat
		z.Add(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"-", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("sub: %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("sub: %s is not a number", yV)
		}
		var z big.Rat
		z.Sub(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"neg", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("neg: expected 1 argument, got %d", len(args))
		}
		xV := args[0]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("neg: %s is not a number", xV)
		}
		var z big.Rat
		z.Neg(&x.Rat)
		return &Number{z}, nil
	}},
	{"*", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("mul: %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("mul: %s is not a number", yV)
		}
		var z big.Rat
		z.Mul(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"/", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
		}

		xV, yV := args[0], args[1]
		x, ok := xV.(*Number)
		if !ok {
			return xV, fmt.Errorf("div: %s is not a number", xV)
		}
		y, ok := yV.(*Number)
		if !ok {
			return yV, fmt.Errorf("div: %s is not a number", yV)
		}
		var zero big.Rat
		if y.Cmp(&zero) == 0 {
			return yV, errors.New("div: denominator is zero")
		}

		var z big.Rat
		z.Quo(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"%%", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
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
	{"->", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
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

		block, ok := blockV.(Block)
		if !ok {
			return blockV, fmt.Errorf(
				"expected block, got %s",
				blockV,
			)
		}
		return block.withArgs(atoms)
	}},
	{"defop", func(env **environment, args []Value) (Value, error) {
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
		block, ok := blockV.(Block)
		if !ok {
			return blockV, fmt.Errorf("fourth argument must be block, got %s", blockV)
		}

		blockV, err := block.withArgs([]Atom{lhs, rhs})
		if err != nil {
			// TODO: blockV already overwritten
			return blockV, err
		}
		nextEnv, ok := (*env).insert(Atom(symbol), blockV)
		if !ok {
			return blockV, fmt.Errorf("couldn't assign to name, %s already exists", symbolV)
		}
		*env = nextEnv
		return unit, nil
	}},
	{"if", func(_ **environment, args []Value) (Value, error) {
		if len(args) < 2 || len(args) > 3 {
			return nil, fmt.Errorf("if: expected 2 or 3 arguments, got %d", len(args))
		}
		tBlock, ok := args[1].(Block)
		if !ok {
			return args[1], fmt.Errorf("if: second argument must be a block, got %s", args[1])
		}

		_, isUnit := args[0].(Unit)
		if b, isBool := args[0].(Bool); (isBool && !bool(b)) || isUnit {
			if len(args) == 3 {
				fBlock, ok := args[2].(Block)
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
	{"loop", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("loop: expected 1 argument, got %d", len(args))
		}
		blockV := args[0]
		block, ok := blockV.(Block)
		if !ok {
			return blockV, fmt.Errorf("loop: expected block, got %s", blockV)
		}
		for {
			// TODO: check if block needs env
			v, err := block.run(nil, nil)
			if err != nil {
				return v, err
			}
			if _, isUnit := v.(Unit); !isUnit {
				return v, nil
			}
		}
	}},
	{"@", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, errOperatorArgumentsLength
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
	{"len", func(_ **environment, args []Value) (Value, error) {
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
	{"append", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("append: expected 2 arguments, got %d", len(args))
		}
		lV, v := args[0], args[1]
		l, ok := lV.(List)
		if !ok {
			return lV, fmt.Errorf("append: expected list, got %s", lV)
		}
		next := make([]Value, len(l.data)+1)
		copy(next, l.data)
		next[len(next)-1] = v
		return List{next}, nil

	}},
	{"append_list", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("append_list: expected 2 arguments, got %d", len(args))
		}
		lV, l2V := args[0], args[1]
		l, ok := lV.(List)
		if !ok {
			return lV, fmt.Errorf("append_list: expected list, got %s", lV)
		}
		l2, ok := l2V.(List)
		if !ok {
			return l2V, fmt.Errorf("append_list: expected list, got %s", l2V)
		}

		// TODO: if a list is empty, don't copy
		next := make([]Value, len(l.data)+len(l2.data))
		n := copy(next, l.data)
		copy(next[n:], l2.data)
		return List{next}, nil

	}},
	{"slice", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 3 {
			return nil, fmt.Errorf("slice: expected 3 arguments, got %d", len(args))
		}
		lV, fromV, toV := args[0], args[1], args[2]
		l, ok := lV.(List)
		if !ok {
			return lV, fmt.Errorf("slice: expected list, got %s", lV)
		}
		fromN, ok := fromV.(*Number)
		if !ok {
			return fromV, fmt.Errorf("slice: expected number, got %s", fromV)
		}
		toN, ok := toV.(*Number)
		if !ok {
			return toV, fmt.Errorf("slice: expected number, got %s", toV)
		}

		from, err := fromN.asUnsigned()
		if err != nil {
			return fromV, fmt.Errorf("slice: from is not valid, %w", err)
		}
		to, err := toN.asUnsigned()
		if err != nil {
			return toV, fmt.Errorf("slice: to is not valid, %w", err)
		}

		if from > len(l.data) {
			return fromV, fmt.Errorf("slice: from (%d) is too large", from)
		}
		if to > len(l.data) {
			return toV, fmt.Errorf("slice: to (%d) is too large", from)
		}
		if from > to {
			return fromV, fmt.Errorf(
				"slice: from (%d) is bigger than to (%d)",
				from,
				to,
			)
		}

		return List{l.data[from:to]}, nil
	}},
	{"call", func(_ **environment, args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("call: expected 2 arguments, got %d", len(args))
		}
		blockV, argListV := args[0], args[1]
		block, ok := blockV.(Block)
		if !ok {
			return blockV, fmt.Errorf("call: expected block, got %s", blockV)
		}
		argList, ok := argListV.(List)
		if !ok {
			return argListV, fmt.Errorf("call: expected list, got %s", argListV)
		}

		// TODO: check if block needs an env
		return block.run(nil, argList.data)

	}},
	{"println", func(_ **environment, args []Value) (Value, error) {
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

var envWithBuiltins *environment = nil

func init() {
	var ok bool
	for _, builtin := range builtinBlocks {
		v := Block{fromGo: builtin.fn}
		envWithBuiltins, ok = envWithBuiltins.insert(builtin.name, v)
		if !ok {
			panic(internal)
		}
	}
	for _, builtin := range builtinOther {
		envWithBuiltins, ok = envWithBuiltins.insert(builtin.name, builtin.value)
		if !ok {
			panic(internal)
		}
	}
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
//		() (String) (Number) (List) (Block) (Group) (Operator)
//	stored as underlying value or atom:
//		(Atom)
//	evaluated (arguments treated like this as well) and stored as value:
//		(Atom ...)  (String ...) (Block ...) (Group ...)
func Run(block parser.Block) error {
	b := Block{env: envWithBuiltins, code: block}
	v, err := b.run(nil, nil)
	if err != nil {
		return err
	}
	if _, isUnit := v.(Unit); !isUnit {
		return fmt.Errorf("last group in root block evaluates to %s, not unit", v)
	}
	return nil
}
