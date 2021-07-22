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

type Block interface {
	Value
	runWithoutEnv(args ...Value) (Value, error)
	runWithEnv(env *environment, args ...Value) (*environment, Value, error)
}

const blockString = "<block>"

type basicBlock struct {
	env  *environment
	code parser.Block
}

func (basicBlock) value() {}

func (basicBlock) Eq(_ Value) bool {
	return false
}

func (basicBlock) String() string {
	return blockString
}

func (b basicBlock) runWithoutEnv(args ...Value) (Value, error) {
	return runCode(b.env, b.code, args...)
}

func (b basicBlock) runWithEnv(env *environment, args ...Value) (*environment, Value, error) {
	v, err := runCode(b.env, b.code, args...)
	return env, v, err
}

func runCode(env *environment, code parser.Block, args ...Value) (Value, error) {
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

	for i, group := range code {
		var (
			v   Value
			err error
		)
		env, v, err = evalGroup(env, group)
		if err != nil {
			return nil, err
		}

		if i == len(code)-1 {
			return v, nil
		} else {
			if _, ok := v.(Unit); !ok {
				return nil, fmt.Errorf("non unit value %s in other than last group of block", v)
			}
		}
	}
	return unit, nil
}

func (b basicBlock) withArgs(argNames ...Atom) (Block, error) {
	for _, a := range argNames {
		if _, ok := b.env.get(a); ok {
			return nil, fmt.Errorf(
				"can't use %s as an argument, "+
					"already exists in the environment",
				a,
			)
		}
	}
	return argBlock{argNames, b.env, b.code}, nil
}

type argBlock struct {
	argNames []Atom
	env      *environment
	code     parser.Block
}

func (argBlock) value() {}

func (argBlock) Eq(_ Value) bool {
	return false
}

func (argBlock) String() string {
	return blockString
}

func (b argBlock) runWithoutEnv(args ...Value) (Value, error) {
	if len(args) != len(b.argNames) {
		return nil, fmt.Errorf(
			"expected %d arguments, got %d",
			len(b.argNames),
			len(args),
		)
	}

	env, ok := b.env, false
	for i, a := range b.argNames {
		env, ok = env.insert(a, args[i])
		if !ok {
			panic(internal)
		}
	}
	return runCode(env, b.code)
}

func (b argBlock) runWithEnv(env *environment, args ...Value) (*environment, Value, error) {
	v, err := b.runWithoutEnv(args...)
	return env, v, err
}

var (
	typeValue          = reflect.TypeOf((*Value)(nil)).Elem()
	typeError          = reflect.TypeOf((*error)(nil)).Elem()
	typePtrEnvironment = reflect.TypeOf((*environment)(nil))
)

func newBlockFromFn(fn interface{}) (Block, error) {
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected func, got %T", fn)
	}

	needEnv := false
	switch numIn := t.NumOut(); numIn {
	case 2:
		if numIn < 1 {
			return nil, fmt.Errorf("func %T needs at least 1 argument", fn)
		}
	case 3:
		if numIn < 2 {
			return nil, fmt.Errorf(
				"func %T needs at least 2 arguments with 3 outputs",
				fn,
			)
		}
		if t.In(0) != typePtrEnvironment || t.Out(0) != typePtrEnvironment {
			return nil, fmt.Errorf(
				"func %T needs an input and output environment with 3 outputs",
				fn,
			)
		}
		needEnv = true
	default:
		return nil, fmt.Errorf("func %T needs 2 or 3 outputs", fn)
	}
	if t.Out(t.NumOut()-2) != typeValue || t.Out(t.NumOut()-1) != typeError {
		return nil, fmt.Errorf("func %T needs Value and error as last 2 outputs", fn)
	}

	i := 0
	inLength := t.NumIn()
	upperBound := t.NumIn()
	if needEnv {
		i++
		inLength--
	}
	if t.IsVariadic() {
		inLength--
		upperBound--
	}

	in := make([]reflect.Type, inLength)
	for inIdx := 0; i < upperBound; i, inIdx = i+1, inIdx+1 {
		inType := t.In(i)
		if !inType.ConvertibleTo(typeValue) {
			return nil, fmt.Errorf(
				"argument of func %T is not convertible to Value",
				fn,
			)
		}
		in[inIdx] = inType
	}

	var slice reflect.Type
	if t.IsVariadic() {
		slice = t.In(t.NumIn() - 1)
		if elem := slice.Elem(); !elem.ConvertibleTo(typeValue) {
			return nil, fmt.Errorf(
				"variadic argument of func %T is not convertible to Value",
				fn,
			)
		}
	}

	if needEnv {
		return fnBlockWithEnv{in, slice, fn}, nil
	}
	return fnBlockWithoutEnv{in, slice, fn}, nil
}

type fnBlockWithoutEnv struct {
	in    []reflect.Type
	slice reflect.Type
	fn    interface{}
}

func (fnBlockWithoutEnv) value() {}

func (fnBlockWithoutEnv) Eq(_ Value) bool {
	return false
}

func (fnBlockWithoutEnv) String() string {
	return blockString
}

func (b fnBlockWithoutEnv) runWithoutEnv(args ...Value) (Value, error) {
	if fn, ok := b.fn.(func(...Value) (Value, error)); ok {
		return fn(args...)
	}

	isVariadic := b.slice != nil
	inLen := len(b.in)
	if isVariadic {
		inLen++
	}
	in := make([]reflect.Value, inLen)
	if err := prepareReflectCall(in, b.in, b.slice, args...); err != nil {
		return nil, err
	}

	var (
		out []reflect.Value
		v   Value
		err error
	)
	if isVariadic {
		out = reflect.ValueOf(b.fn).CallSlice(in)
	} else {
		out = reflect.ValueOf(b.fn).Call(in)
	}
	if iV := out[0].Interface(); iV != nil {
		v = iV.(Value)
	}
	if iErr := out[1].Interface(); iErr != nil {
		err = iErr.(error)
	}
	return v, err
}

func (b fnBlockWithoutEnv) runWithEnv(env *environment, args ...Value) (*environment, Value, error) {
	v, err := b.runWithoutEnv(args...)
	return env, v, err
}

type fnBlockWithEnv struct {
	in    []reflect.Type
	slice reflect.Type
	fn    interface{}
}

func (fnBlockWithEnv) value() {}

func (fnBlockWithEnv) Eq(_ Value) bool {
	return false
}

func (fnBlockWithEnv) String() string {
	return blockString
}

func (b fnBlockWithEnv) runWithEnv(env *environment, args ...Value) (*environment, Value, error) {
	if fn, ok := b.fn.(func(*environment, ...Value) (*environment, Value, error)); ok {
		return fn(env, args...)
	}

	isVariadic := b.slice != nil
	inLen := len(b.in) + 1
	if isVariadic {
		inLen++
	}
	in := make([]reflect.Value, inLen)
	in[0] = reflect.ValueOf(env)
	if err := prepareReflectCall(in[1:], b.in, b.slice, args...); err != nil {
		return nil, nil, err
	}

	var (
		out  []reflect.Value
		next *environment
		v    Value
		err  error
	)
	if isVariadic {
		out = reflect.ValueOf(b.fn).CallSlice(in)
	} else {
		out = reflect.ValueOf(b.fn).Call(in)
	}
	if nextV := out[0].Interface(); nextV != nil {
		next = nextV.(*environment)
	}
	if iV := out[1].Interface(); iV != nil {
		v = iV.(Value)
	}
	if iErr := out[2].Interface(); iErr != nil {
		err = iErr.(error)
	}
	return next, v, err
}

func (b fnBlockWithEnv) runWithoutEnv(args ...Value) (Value, error) {
	return nil, errors.New("can't run this block without an environment")
}

func prepareReflectCall(in []reflect.Value, inTypes []reflect.Type, slice reflect.Type, args ...Value) error {
	isVariadic := slice != nil

	expected, got := len(inTypes), len(args)
	if !isVariadic && expected != got {
		return fmt.Errorf("expected %d arguments, got %d", expected, got)
	}
	if got < expected {
		return fmt.Errorf("expected at least %d arguments, got %d", expected, got)
	}

	for i := range inTypes {
		t, arg := inTypes[i], args[i]
		v := reflect.ValueOf(arg)
		if !v.Type().ConvertibleTo(t) {
			// TODO: better error message for inType
			return fmt.Errorf(
				"argument error: expected %s, got %s",
				t.String(),
				arg.String(),
			)
		}
		in[i] = v.Convert(t)
	}

	if isVariadic {
		remainder := args[len(inTypes):]
		s := reflect.MakeSlice(
			slice,
			len(remainder),
			len(remainder),
		)

		to := slice.Elem()
		for i, vv := range remainder {
			v := reflect.ValueOf(vv)
			if !v.Type().ConvertibleTo(to) {
				// TODO: better error message for v
				return fmt.Errorf(
					"argument error: expected %s, got %s",
					to.String(),
					vv.String(),
				)
			}
			s.Index(i).Set(v.Convert(to))
		}

		in[len(in)-1] = s
	}

	return nil
}

func evalGroup(env *environment, group parser.Group) (*environment, Value, error) {
	switch len(group) {
	case 0:
		return env, unit, nil
	case 1:
		return evalElement(env, group[0])
	default:
		args := make([]Value, len(group)-1)
		for i, e := range group[1:] {
			var (
				v   Value
				err error
			)
			env, v, err = evalElement(env, e)
			if err != nil {
				return nil, nil, err
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
			v, err := runCode(env, first, args...)
			if err != nil {
				return nil, nil, err
			}
			return env, v, nil
		case parser.Group:
			var (
				blockV Value
				err    error
			)
			env, blockV, err = evalGroup(env, first)
			if err != nil {
				return nil, nil, err
			}
			block, ok := blockV.(Block)
			if !ok {
				return nil, nil, fmt.Errorf(
					"can't call value %s, not a block",
					blockV,
				)
			}
			return block.runWithEnv(env, args...)
		default:
			// TODO: parser: check atom/string/block/group is first
			panic(internal)
		}

		nameV, ok := env.get(name)
		if !ok {
			return nil, nil, fmt.Errorf("name %s not found", name)
		}
		block, ok := nameV.(Block)
		if !ok {
			return nil, nil, fmt.Errorf(
				"name %s is not a block, but a %s value instead",
				name,
				nameV.String(),
			)
		}
		return block.runWithEnv(env, args...)
	}
}

func evalElement(env *environment, element parser.Element) (*environment, Value, error) {
	switch v := element.(type) {
	case parser.Atom:
		val, ok := env.get(Atom(v))
		if !ok {
			return env, Atom(string(v)), nil
		}
		return env, val, nil
	case parser.String:
		return env, String(v), nil
	case *parser.Number:
		return env, &Number{v.Rat}, nil
	case parser.Operator:
		var (
			lhsV, rhsV Value
			err        error
		)
		env, lhsV, err = evalGroup(env, v.Lhs)
		if err != nil {
			return nil, nil, err
		}
		env, rhsV, err = evalGroup(env, v.Rhs)
		if err != nil {
			return nil, nil, err
		}

		symbolV, ok := env.get(Atom(v.Symbol))
		if !ok {
			return nil, nil, fmt.Errorf("unknown operator %s", v.Symbol)
		}
		block, ok := symbolV.(Block)
		if !ok {
			return nil, nil, fmt.Errorf(
				"operator %s is not a block, but a %s value instead",
				v.Symbol,
				symbolV,
			)
		}
		return block.runWithEnv(env, lhsV, rhsV)
	case parser.List:
		l := make([]Value, len(v))
		for i, e := range v {
			var (
				v   Value
				err error
			)
			env, v, err = evalElement(env, e)
			if err != nil {
				return nil, nil, err
			}
			l[i] = v
		}
		return env, List{l}, nil
	case parser.Group:
		return evalGroup(env, v)
	case parser.Block:
		return env, basicBlock{env, v}, nil
	default:
		panic(internal)
	}
}

type environment struct {
	// TODO: use persistent map

	key         Atom
	value       Value
	left, right *environment
}

func (env *environment) get(k Atom) (Value, bool) {
	// TODO: iterative

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

func Run(block parser.Block) error {
	v, err := runCode(envWithBuiltins, block)
	if err != nil {
		return err
	}
	if _, isUnit := v.(Unit); !isUnit {
		return fmt.Errorf("last group in root block evaluates to %s, not unit", v)
	}
	return nil
}
