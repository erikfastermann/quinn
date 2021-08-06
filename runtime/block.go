package runtime

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/value"
)

var tagBlock = value.NewTag()

type Block interface {
	value.Value
	runWithoutEnv(args ...value.Value) (value.Value, error)
	runWithEnv(env *Environment, args ...value.Value) (*Environment, value.Value, error)
}

type basicBlock struct {
	env  *Environment
	code parser.Block
}

func (basicBlock) Tag() value.Tag {
	return tagBlock
}

func (b basicBlock) runWithoutEnv(args ...value.Value) (value.Value, error) {
	_, v, err := runCode(b.env, b.code, args...)
	return v, err
}

func (b basicBlock) runWithEnv(env *Environment, args ...value.Value) (*Environment, value.Value, error) {
	_, v, err := runCode(b.env, b.code, args...)
	return env, v, err
}

func runCode(env *Environment, code parser.Block, args ...value.Value) (*Environment, value.Value, error) {
	switch len(args) {
	case 0:
	case 1:
		if _, isUnit := args[0].(Unit); !isUnit {
			return nil, nil, fmt.Errorf(
				"first argument in call to basic block must be unit, not %s",
				valueString(args[0]),
			)
		}
	default:
		return nil, nil, fmt.Errorf(
			"too many arguments in call to basic block (%d)",
			len(args),
		)
	}

	if len(code.V) == 0 {
		return env, unit, nil
	}

	for _, elem := range code.V[:len(code.V)-1] {
		var (
			v   value.Value
			err error
		)
		env, v, err = evalElement(env, elem)
		if err != nil {
			return nil, nil, err
		}

		if _, err := getAttribute(v, tagReturner); err != nil {
			continue
		}
		return env, v, nil
	}
	return evalElement(env, code.V[len(code.V)-1])
}

func evalElement(env *Environment, element parser.Element) (_ *Environment, v value.Value, err error) {
	env, v, err = evalElementNoPosition(env, element)
	if err != nil {
		if _, ok := err.(PositionedError); ok {
			return nil, nil, err
		}
		line, col := element.Position()
		return nil, nil, PositionedError{line, col, err}
	}
	return env, v, err
}

func evalElementNoPosition(env *Environment, element parser.Element) (*Environment, value.Value, error) {
	switch v := element.(type) {
	case parser.Ref:
		val, ok := env.get(Atom(v.V))
		if !ok {
			return nil, nil, fmt.Errorf("unknown variable %s", v.V)
		}
		return env, val, nil
	case parser.Atom:
		return env, Atom(v.V), nil
	case parser.String:
		return env, String(v.V), nil
	case parser.Number:
		return env, number.Number(v.V), nil
	case parser.Unit:
		return env, unit, nil
	case parser.Call:
		args := make([]value.Value, len(v.Args))
		for i, e := range v.Args {
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

		var (
			val value.Value
			err error
		)
		env, val, err = evalElement(env, v.First)
		if err != nil {
			return nil, nil, err
		}
		b, ok := val.(Block)
		if !ok {
			return nil, nil, fmt.Errorf(
				"first in call must evaluate to block, got %s instead",
				valueString(val),
			)
		}
		return b.runWithEnv(env, args...)
	case parser.List:
		l := make([]value.Value, len(v.V))
		for i, e := range v.V {
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
	case parser.Block:
		return env, basicBlock{env, v}, nil
	default:
		panic(internal)
	}
}

type argBlock struct {
	before, b, after basicBlock
}

func (argBlock) Tag() value.Tag {
	return tagBlock
}

func (b argBlock) runWithoutEnv(args ...value.Value) (value.Value, error) {
	const errBefore = "expected before to return a list " +
		"of unique atom and value pairs, got %s instead"

	const input = "__args"
	beforeEnv, ok := b.before.env.insert(Atom(input), List{args})
	if !ok {
		return nil, fmt.Errorf(
			"before already has %s defined in the environment",
			input,
		)
	}
	_, kvV, err := runCode(beforeEnv, b.before.code)
	if err != nil {
		return nil, err
	}
	kv, ok := kvV.(List)
	if !ok {
		return nil, fmt.Errorf(errBefore, valueString(kvV))
	}

	env, ok := b.b.env, false
	for _, pairV := range kv.data {
		pair, ok := pairV.(List)
		if !ok {
			return nil, fmt.Errorf(errBefore, valueString(kv))
		}
		if len(pair.data) != 2 {
			return nil, fmt.Errorf(errBefore, valueString(kv))
		}
		atomV, v := pair.data[0], pair.data[1]
		atom, ok := atomV.(Atom)
		if !ok {
			return nil, fmt.Errorf(errBefore, valueString(kv))
		}
		env, ok = env.insert(atom, v)
		if !ok {
			return nil, fmt.Errorf(
				"can't use %s as an argument, already exists in the environment",
				valueString(atom),
			)
		}
	}

	_, v, err := runCode(env, b.b.code)
	if err != nil {
		return nil, err
	}

	const ret = "__return"
	afterEnv, ok := b.after.env.insert(Atom(ret), v)
	if !ok {
		return nil, fmt.Errorf(
			"after already has %s defined in the environment",
			ret,
		)
	}
	_, v, err = runCode(afterEnv, b.after.code)
	return v, err
}

func (b argBlock) runWithEnv(env *Environment, args ...value.Value) (*Environment, value.Value, error) {
	v, err := b.runWithoutEnv(args...)
	return env, v, err
}

var (
	typeValue          = reflect.TypeOf((*value.Value)(nil)).Elem()
	typeError          = reflect.TypeOf((*error)(nil)).Elem()
	typePtrEnvironment = reflect.TypeOf((*Environment)(nil))
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

func (b fnBlockWithoutEnv) runWithEnv(env *Environment, args ...value.Value) (*Environment, value.Value, error) {
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

func (b fnBlockWithEnv) runWithEnv(env *Environment, args ...value.Value) (*Environment, value.Value, error) {
	if fn, ok := b.fn.(func(*Environment, ...value.Value) (*Environment, value.Value, error)); ok {
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
		next *Environment
		v    value.Value
		err  error
	)
	if isVariadic {
		out = reflect.ValueOf(b.fn).CallSlice(in)
	} else {
		out = reflect.ValueOf(b.fn).Call(in)
	}
	if nextV := out[0].Interface(); nextV != nil {
		next = nextV.(*Environment)
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

var stringBlock value.Value = String("<block>")

func stringerBlock(_ Block) (value.Value, error) {
	return stringBlock, nil
}
