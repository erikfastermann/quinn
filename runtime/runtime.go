package runtime

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/erikfastermann/quinn/parser"
)

// TODO:
//	loops / conditionals
//	recursion
//	early returns
//	Value.Type
//	return nil everywhere if error

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
	fromGo interface{}
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
		return b.runInterface(env, args)
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

var (
	typeValue             = reflect.TypeOf((*Value)(nil)).Elem()
	typeError             = reflect.TypeOf((*error)(nil)).Elem()
	typePtrPtrEnvironment = reflect.TypeOf((**environment)(nil))
)

func (b Block) runInterface(env **environment, args []Value) (Value, error) {
	if fn, ok := b.fromGo.(func(**environment, ...Value) (Value, error)); ok {
		return fn(env, args...)
	}

	fn := reflect.ValueOf(b.fromGo)
	t := fn.Type()
	if t.Kind() != reflect.Func ||
		t.NumIn() < 1 ||
		t.In(0) != typePtrPtrEnvironment ||
		t.NumOut() != 2 ||
		t.Out(0) != typeValue ||
		t.Out(1) != typeError {
		panic(internal)
	}

	expected, got := t.NumIn()-1, len(args)
	if !t.IsVariadic() {
		if expected != got {
			return nil, fmt.Errorf("expected %d arguments, got %d", expected, got)
		}
	} else {
		expected--
		if got < expected {
			return nil, fmt.Errorf("expected at least %d arguments, got %d", expected, got)
		}
	}

	in := make([]reflect.Value, t.NumIn())
	in[0] = reflect.ValueOf(env)

	upperBound := t.NumIn()
	if t.IsVariadic() {
		upperBound--
	}
	for inIdx := 1; inIdx < upperBound; inIdx++ {
		argIdx := inIdx - 1
		arg := args[argIdx]
		v := reflect.ValueOf(arg)
		inType := t.In(inIdx)
		if !v.Type().ConvertibleTo(inType) {
			// TODO: better error message for inType
			return arg, fmt.Errorf(
				"%d. argument: expected %s, got %s",
				argIdx+1,
				inType.String(),
				arg.String(),
			)
		}
		in[inIdx] = v.Convert(inType)
	}

	var out []reflect.Value
	if t.IsVariadic() {
		start := len(in) - 2
		remainder := args[start:]
		sliceType := t.In(t.NumIn() - 1)
		s := reflect.MakeSlice(
			sliceType,
			len(remainder),
			len(remainder),
		)

		toType := sliceType.Elem()
		for i := range remainder {
			vv := remainder[i]
			v, to := reflect.ValueOf(vv), s.Index(i)
			if !v.Type().ConvertibleTo(toType) {
				// TODO: better error message for v
				return nil, fmt.Errorf(
					"%d. argument: expected %s, got %s",
					start+i+1,
					toType.String(),
					vv.String(),
				)
			}
			to.Set(v.Convert(toType))
		}
		in[len(in)-1] = s

		out = fn.CallSlice(in)
	} else {
		out = fn.Call(in)
	}

	var (
		v   Value
		err error
	)
	if iV := out[0].Interface(); iV != nil {
		v = iV.(Value)
	}
	if iErr := out[1].Interface(); iErr != nil {
		err = iErr.(error)
	}
	return v, err
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

	return Block{fromGo: func(_ **environment, args ...Value) (Value, error) {
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

var builtinBlocks = []struct {
	name Atom
	fn   interface{}
}{
	{"mut", func(_ **environment, v Value) (Value, error) {
		return &Mut{v}, nil
	}},
	{"load", func(_ **environment, target *Mut) (Value, error) {
		return target.v, nil
	}},
	{"<-", func(_ **environment, target *Mut, v Value) (Value, error) {
		target.v = v
		return unit, nil
	}},
	{"=", func(env **environment, assignee Atom, v Value) (Value, error) {
		nextEnv, ok := (*env).insert(assignee, v)
		if !ok {
			return nil, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		*env = nextEnv
		return unit, nil
	}},
	{"==", func(_ **environment, x, y Value) (Value, error) {
		return Bool(x.Eq(y)), nil
	}},
	{"!=", func(_ **environment, x, y Value) (Value, error) {
		return Bool(!x.Eq(y)), nil
	}},
	{">=", func(_ **environment, x, y *Number) (Value, error) {
		return Bool(x.Cmp(&y.Rat) >= 0), nil
	}},
	{"not", func(_ **environment, b Bool) (Value, error) {
		return !b, nil
	}},
	{"+", func(_ **environment, x, y *Number) (Value, error) {
		var z big.Rat
		z.Add(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"-", func(_ **environment, x, y *Number) (Value, error) {
		var z big.Rat
		z.Sub(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"neg", func(_ **environment, x *Number) (Value, error) {
		var z big.Rat
		z.Neg(&x.Rat)
		return &Number{z}, nil
	}},
	{"*", func(_ **environment, x, y *Number) (Value, error) {
		var z big.Rat
		z.Mul(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"/", func(_ **environment, x, y *Number) (Value, error) {
		var zero big.Rat
		if y.Cmp(&zero) == 0 {
			return nil, errors.New("denominator is zero")
		}

		var z big.Rat
		z.Quo(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"%%", func(_ **environment, x, y *Number) (Value, error) {
		if !x.IsInt() {
			return nil, fmt.Errorf(
				"%s is not an integer",
				x.RatString(),
			)
		}
		if !y.IsInt() {
			return nil, fmt.Errorf(
				"%s is not an integer",
				y.RatString(),
			)
		}
		if y.Num().IsInt64() && y.Num().Int64() == 0 {
			return nil, errors.New("denominator is zero")
		}

		var z big.Int
		z.Rem(x.Num(), y.Num())
		var r big.Rat
		r.SetInt(&z)
		return &Number{r}, nil
	}},
	{"->", func(_ **environment, def List, block Block) (Value, error) {
		atoms := make([]Atom, len(def.data))
		for i, v := range def.data {
			atom, ok := v.(Atom)
			if !ok {
				return v, fmt.Errorf("argument has to be atom, got %s", v)
			}
			atoms[i] = atom
		}
		return block.withArgs(atoms)
	}},
	{"defop", func(env **environment, symbol String, lhs, rhs Atom, block Block) (Value, error) {
		// TODO: check symbol is valid operator

		blockV, err := block.withArgs([]Atom{lhs, rhs})
		if err != nil {
			return nil, err
		}
		nextEnv, ok := (*env).insert(Atom(symbol), blockV)
		if !ok {
			return nil, fmt.Errorf(
				"couldn't assign to name, %s already exists",
				symbol.String(),
			)
		}
		*env = nextEnv
		return unit, nil
	}},
	{"if", func(_ **environment, cond Value, tBlock Block, blocks ...Block) (Value, error) {
		var fBlock Block
		hasFBlock := false
		switch len(blocks) {
		case 0:
		case 1:
			fBlock = blocks[0]
			hasFBlock = true
		default:
			return nil, fmt.Errorf("expected 2 or 3 arguments, got %d", 2+len(blocks))
		}

		_, isUnit := cond.(Unit)
		if b, isBool := cond.(Bool); (isBool && !bool(b)) || isUnit {
			if hasFBlock {
				return fBlock.run(nil, nil)
			} else {
				return unit, nil
			}
		}
		return tBlock.run(nil, nil)
	}},
	{"loop", func(_ **environment, block Block) (Value, error) {
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
	{"@", func(_ **environment, l List, num *Number) (Value, error) {
		var zero big.Int
		if !num.IsInt() || num.Num().Cmp(&zero) < 0 {
			return nil, fmt.Errorf("%s is not an unsigned integer", num.String())
		}
		idx64 := num.Num().Int64()
		idx := int(idx64)
		if !num.Num().IsInt64() || int64(idx) != idx64 || idx >= len(l.data) {
			return nil, fmt.Errorf(
				"index out of range (%s with length %d)",
				num,
				len(l.data),
			)
		}
		return l.data[idx], nil
	}},
	{"len", func(_ **environment, l List) (Value, error) {
		var r big.Rat
		r.SetInt64(int64(len(l.data)))
		return &Number{r}, nil
	}},
	{"append", func(_ **environment, l List, v Value) (Value, error) {
		next := make([]Value, len(l.data)+1)
		copy(next, l.data)
		next[len(next)-1] = v
		return List{next}, nil

	}},
	{"append_list", func(_ **environment, l, l2 List) (Value, error) {
		// TODO: if a list is empty, don't copy
		next := make([]Value, len(l.data)+len(l2.data))
		n := copy(next, l.data)
		copy(next[n:], l2.data)
		return List{next}, nil

	}},
	{"slice", func(_ **environment, l List, fromN, toN *Number) (Value, error) {
		from, err := fromN.asUnsigned()
		if err != nil {
			return nil, fmt.Errorf("from is not valid, %w", err)
		}
		to, err := toN.asUnsigned()
		if err != nil {
			return nil, fmt.Errorf("to is not valid, %w", err)
		}

		if from > len(l.data) {
			return nil, fmt.Errorf("from (%d) is too large", from)
		}
		if to > len(l.data) {
			return nil, fmt.Errorf("to (%d) is too large", from)
		}
		if from > to {
			return nil, fmt.Errorf(
				"from (%d) is bigger than to (%d)",
				from,
				to,
			)
		}
		return List{l.data[from:to]}, nil
	}},
	{"call", func(_ **environment, b Block, args List) (Value, error) {
		// TODO: check if block needs an env
		return b.run(nil, args.data)

	}},
	{"println", func(_ **environment, args ...Value) (Value, error) {
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
