package runtime

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/value"
)

// TODO:
//	loops / conditionals
//	recursion
//	early returns
//	Value.Type
//	return nil everywhere if error

var (
	tagUnit          = value.NewTag()
	tagBool          = value.NewTag()
	tagString        = value.NewTag()
	tagAtom          = value.NewTag()
	tagList          = value.NewTag()
	tagIterationStop = value.NewTag()
	tagMut           = value.NewTag()
	tagBlock         = value.NewTag()
)

const internal = "internal error"

type Unit struct{}

var unit value.Value = Unit{}

func (Unit) Tag() value.Tag {
	return tagUnit
}

type Bool struct {
	b bool
}

var (
	falseValue value.Value = Bool{false}
	trueValue  value.Value = Bool{true}
)

func NewBool(b bool) value.Value {
	if b {
		return trueValue
	}
	return falseValue
}

func (Bool) Tag() value.Tag {
	return tagBool
}

func (b Bool) AsBool() bool {
	return b.b
}

type String string

func (String) Tag() value.Tag {
	return tagString
}

type Atom string

func (Atom) Tag() value.Tag {
	return tagAtom
}

type List struct {
	// TODO: use persistent array
	data []value.Value
}

func (List) Tag() value.Tag {
	return tagList
}

// TODO: implement and use exceptions instead
type IterationStop struct{}

func (IterationStop) Tag() value.Tag {
	return tagIterationStop
}

type Mut struct {
	v value.Value
}

func (*Mut) Tag() value.Tag {
	return tagMut
}

type Block interface {
	value.Value
	runWithoutEnv(args ...value.Value) (value.Value, error)
	runWithEnv(env *environment, args ...value.Value) (*environment, value.Value, error)
}

type basicBlock struct {
	env  *environment
	code parser.Block
}

func (basicBlock) Tag() value.Tag {
	return tagBlock
}

func (b basicBlock) runWithoutEnv(args ...value.Value) (value.Value, error) {
	return runCode(b.env, b.code, args...)
}

func (b basicBlock) runWithEnv(env *environment, args ...value.Value) (*environment, value.Value, error) {
	v, err := runCode(b.env, b.code, args...)
	return env, v, err
}

func runCode(env *environment, code parser.Block, args ...value.Value) (value.Value, error) {
	switch len(args) {
	case 0:
	case 1:
		if _, isUnit := args[0].(Unit); !isUnit {
			return nil, fmt.Errorf(
				"first argument in call to basic block must be unit, not %s",
				valueString(args[0]),
			)
		}
	default:
		return nil, fmt.Errorf("too many arguments in call to basic block (%d)", len(args))
	}

	for i, group := range code {
		var (
			v   value.Value
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
				return nil, fmt.Errorf(
					"non unit value %s in other than last group of block",
					valueString(v),
				)
			}
		}
	}
	return unit, nil
}

func (b basicBlock) withArgs(argNames ...Atom) (Block, error) {
	for _, a := range argNames {
		if _, ok := b.env.get(a); ok {
			return nil, fmt.Errorf(
				"can't use %s as an argument, already exists in the environment",
				valueString(a),
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

func (argBlock) Tag() value.Tag {
	return tagBlock
}

func (b argBlock) runWithoutEnv(args ...value.Value) (value.Value, error) {
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

func (b argBlock) runWithEnv(env *environment, args ...value.Value) (*environment, value.Value, error) {
	v, err := b.runWithoutEnv(args...)
	return env, v, err
}

var (
	typeValue          = reflect.TypeOf((*value.Value)(nil)).Elem()
	typeError          = reflect.TypeOf((*error)(nil)).Elem()
	typePtrEnvironment = reflect.TypeOf((*environment)(nil))
)

func newBlockMust(fn interface{}) Block {
	b, err := NewBlock(fn)
	if err != nil {
		panic(internal + ": " + err.Error())
	}
	return b
}

func NewBlock(fn interface{}) (Block, error) {
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

func (fnBlockWithoutEnv) Tag() value.Tag {
	return tagBlock
}

func (b fnBlockWithoutEnv) runWithoutEnv(args ...value.Value) (value.Value, error) {
	if fn, ok := b.fn.(func(...value.Value) (value.Value, error)); ok {
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
		v   value.Value
		err error
	)
	if isVariadic {
		out = reflect.ValueOf(b.fn).CallSlice(in)
	} else {
		out = reflect.ValueOf(b.fn).Call(in)
	}
	if iV := out[0].Interface(); iV != nil {
		v = iV.(value.Value)
	}
	if iErr := out[1].Interface(); iErr != nil {
		err = iErr.(error)
	}
	return v, err
}

func (b fnBlockWithoutEnv) runWithEnv(env *environment, args ...value.Value) (*environment, value.Value, error) {
	v, err := b.runWithoutEnv(args...)
	return env, v, err
}

type fnBlockWithEnv struct {
	in    []reflect.Type
	slice reflect.Type
	fn    interface{}
}

func (fnBlockWithEnv) Tag() value.Tag {
	return tagBlock
}

func (b fnBlockWithEnv) runWithEnv(env *environment, args ...value.Value) (*environment, value.Value, error) {
	if fn, ok := b.fn.(func(*environment, ...value.Value) (*environment, value.Value, error)); ok {
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
		v    value.Value
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
		v = iV.(value.Value)
	}
	if iErr := out[2].Interface(); iErr != nil {
		err = iErr.(error)
	}
	return next, v, err
}

func (b fnBlockWithEnv) runWithoutEnv(args ...value.Value) (value.Value, error) {
	return nil, errors.New("can't run this block without an environment")
}

func prepareReflectCall(in []reflect.Value, inTypes []reflect.Type, slice reflect.Type, args ...value.Value) error {
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
				valueString(arg),
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
					valueString(vv),
				)
			}
			s.Index(i).Set(v.Convert(to))
		}

		in[len(in)-1] = s
	}

	return nil
}

func evalGroup(env *environment, group parser.Group) (*environment, value.Value, error) {
	switch len(group) {
	case 0:
		return env, unit, nil
	case 1:
		return evalElement(env, group[0])
	default:
		args := make([]value.Value, len(group)-1)
		for i, e := range group[1:] {
			var (
				v   value.Value
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
				blockV value.Value
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
					valueString(blockV),
				)
			}
			return block.runWithEnv(env, args...)
		default:
			// TODO: parser: check atom/string/block/group is first
			panic(internal)
		}

		nameV, ok := env.get(name)
		if !ok {
			return nil, nil, fmt.Errorf("name %s not found", valueString(name))
		}
		block, ok := nameV.(Block)
		if !ok {
			return nil, nil, fmt.Errorf(
				"name %s is not a block, but a %s value instead",
				valueString(name),
				valueString(nameV),
			)
		}
		return block.runWithEnv(env, args...)
	}
}

func evalElement(env *environment, element parser.Element) (*environment, value.Value, error) {
	switch v := element.(type) {
	case parser.Atom:
		val, ok := env.get(Atom(v))
		if !ok {
			return env, Atom(string(v)), nil
		}
		return env, val, nil
	case parser.String:
		return env, String(v), nil
	case parser.Number:
		return env, number.Number(v), nil
	case parser.Operator:
		var (
			lhsV, rhsV value.Value
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
				valueString(symbolV),
			)
		}
		return block.runWithEnv(env, lhsV, rhsV)
	case parser.List:
		l := make([]value.Value, len(v))
		for i, e := range v {
			var (
				v   value.Value
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
	value       value.Value
	left, right *environment
}

func (env *environment) get(k Atom) (value.Value, bool) {
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

func (env *environment) insert(k Atom, v value.Value) (*environment, bool) {
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

	str += fmt.Sprintf("[%s %s]", env.key, valueString(env.value))

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
		return fmt.Errorf(
			"last group in root block evaluates to %s, not unit",
			valueString(v),
		)
	}
	return nil
}
