package runtime

import (
	"errors"
	"fmt"

	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/value"
)

var builtinOther = []struct {
	name  Atom
	value value.Value
}{
	{"false", falseValue},
	{"true", trueValue},
	{"tagReturner", tagReturner},
	{"tagEq", tagEq},
	{"tagStringer", tagStringer},
	{"tagMatcher", tagMatcher},
}

var (
	errNonBasicArgBlock  = errors.New("can't create argumented block from non basic block")
	errInvalidAttributes = errors.New("attributes must be lists of unique tag and value pairs")
)

var builtinBlocks = []struct {
	name Atom
	fn   interface{}
}{
	{"default", func(b Block, default_ Block) (value.Value, error) {
		v, err := b.runWithoutEnv(unit)
		if err != nil {
			return default_.runWithoutEnv(unit)
		}
		return v, nil
	}},
	{"atom", func(s String) (value.Value, error) {
		return Atom(s), nil
	}},
	{"newTag", func(_ Unit) (value.Value, error) {
		return value.NewTag(), nil
	}},
	{"tag", func(v value.Tag) (value.Value, error) {
		return v.Tag(), nil
	}},
	{"attr", func(v value.Value, attr value.Tag) (value.Value, error) {
		return getAttribute(v, attr)
	}},
	{"opaque", func(v value.Value, tag value.Tag, attrs ...List) (value.Value, error) {
		// TODO: use Map as attrs when implemented

		m := make(map[value.Tag]value.Value)
		for _, pair := range attrs {
			if len(pair.data) != 2 {
				return nil, errInvalidAttributes
			}
			tagV, attr := pair.data[0], pair.data[1]
			tag, ok := tagV.(value.Tag)
			if !ok {
				return nil, errInvalidAttributes
			}

			if _, ok := m[tag]; ok {
				return nil, errInvalidAttributes
			}
			m[tag] = attr
		}

		o := Opaque{
			tag:   tag,
			v:     v,
			attrs: m,
		}
		return o, nil
	}},
	{"unopaque", func(o Opaque, tag value.Tag) (value.Value, error) {
		if o.tag != tag {
			return nil, errors.New("can't unopaque: tag doesn't match")
		}
		return o.v, nil
	}},
	{"opaqueTagEq", func(o Opaque, tag value.Tag) (value.Value, error) {
		return NewBool(o.tag == tag), nil
	}},
	{"mut", func(v value.Value) (value.Value, error) {
		return &Mut{v}, nil
	}},
	{"load", func(target *Mut) (value.Value, error) {
		return target.v, nil
	}},
	{"<-", func(target *Mut, v value.Value) (value.Value, error) {
		target.v = v
		return unit, nil
	}},
	{"=", func(env *Environment, assignee Atom, v value.Value) (*Environment, value.Value, error) {
		next, ok := env.insert(assignee, v)
		if !ok {
			return nil, nil, fmt.Errorf(
				"couldn't assign to name, %s already exists",
				valueString(assignee),
			)
		}
		return next, unit, nil
	}},
	{"==", func(x, y value.Value) (value.Value, error) {
		return eq(x, y)
	}},
	{"!=", func(x, y value.Value) (value.Value, error) {
		bV, err := eq(x, y)
		if err != nil {
			return nil, err
		}
		b, ok := bV.(Bool)
		if !ok {
			return nil, fmt.Errorf(
				"can't negate non bool value %s",
				valueString(bV),
			)
		}
		return NewBool(!b.AsBool()), nil
	}},
	{">", func(x, y number.Number) (value.Value, error) {
		return NewBool(x.Cmp(y) > 0), nil
	}},
	{">=", func(x, y number.Number) (value.Value, error) {
		return NewBool(x.Cmp(y) >= 0), nil
	}},
	{"<", func(x, y number.Number) (value.Value, error) {
		return NewBool(x.Cmp(y) < 0), nil
	}},
	{"<=", func(x, y number.Number) (value.Value, error) {
		return NewBool(x.Cmp(y) <= 0), nil
	}},
	{"not", func(b Bool) (value.Value, error) {
		return NewBool(!b.AsBool()), nil
	}},
	{"+", func(x, y number.Number) (value.Value, error) {
		return x.Add(y), nil
	}},
	{"-", func(x, y number.Number) (value.Value, error) {
		return x.Sub(y), nil
	}},
	{"neg", func(x number.Number) (value.Value, error) {
		return x.Neg(), nil
	}},
	{"*", func(x, y number.Number) (value.Value, error) {
		return x.Mul(y), nil
	}},
	{"/", func(x, y number.Number) (value.Value, error) {
		return x.Div(y)
	}},
	{"%%", func(x, y number.Number) (value.Value, error) {
		return x.Mod(y)
	}},
	{"argumentify", func(beforeB, bB, afterB Block) (value.Value, error) {
		before, ok := beforeB.(basicBlock)
		if !ok {
			return nil, errNonBasicArgBlock
		}
		b, ok := bB.(basicBlock)
		if !ok {
			return nil, errNonBasicArgBlock
		}
		after, ok := afterB.(basicBlock)
		if !ok {
			return nil, errNonBasicArgBlock
		}
		return argBlock{before: before, b: b, after: after}, nil
	}},
	{"if", func(cond value.Value, tBlock Block, blocks ...Block) (value.Value, error) {
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
		if b, isBool := cond.(Bool); (isBool && !b.AsBool()) || isUnit {
			if hasFBlock {
				return fBlock.runWithoutEnv()
			} else {
				return unit, nil
			}
		}
		return tBlock.runWithoutEnv()
	}},
	{"loop", func(block Block) (value.Value, error) {
		for {
			v, err := block.runWithoutEnv()
			if err != nil {
				return nil, err
			}
			// TODO: use getAttribute for better error reporting
			returner, err := getAttributeBlock(v, tagReturner)
			if err != nil {
				continue
			}
			return returner.runWithoutEnv(v)
		}
	}},
	{"@", func(l List, idx number.Number) (value.Value, error) {
		i, err := idx.Unsigned()
		if err != nil {
			return nil, err
		}
		if i >= len(l.data) {
			return nil, fmt.Errorf(
				"index out of range (%d with length %d)",
				i,
				len(l.data),
			)
		}
		return l.data[i], nil
	}},
	{"len", func(l List) (value.Value, error) {
		return number.FromInt(len(l.data)), nil
	}},
	{"append", func(l List, v value.Value) (value.Value, error) {
		next := make([]value.Value, len(l.data)+1)
		copy(next, l.data)
		next[len(next)-1] = v
		return List{next}, nil
	}},
	{"append_list", func(l, l2 List) (value.Value, error) {
		// TODO: if a list is empty, don't copy
		next := make([]value.Value, len(l.data)+len(l2.data))
		n := copy(next, l.data)
		copy(next[n:], l2.data)
		return List{next}, nil

	}},
	{"slice", func(l List, fromN, toN number.Number) (value.Value, error) {
		from, err := fromN.Unsigned()
		if err != nil {
			return nil, fmt.Errorf("from is not valid, %w", err)
		}
		to, err := toN.Unsigned()
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
	{"call", func(b Block, args List) (value.Value, error) {
		return b.runWithoutEnv(args.data...)
	}},
	{"println", func(args ...value.Value) (value.Value, error) {
		if len(args) == 0 {
			return unit, nil
		}
		for _, v := range args[:len(args)-1] {
			if _, err := fmt.Print(valueString(v), " "); err != nil {
				return nil, err
			}
		}
		if _, err := fmt.Println(valueString(args[len(args)-1])); err != nil {
			return nil, err
		}
		return unit, nil
	}},
}

var builtinEnv *Environment = nil

func init() {
	var ok bool
	for _, builtin := range builtinBlocks {
		builtinEnv, ok = builtinEnv.insert(
			builtin.name,
			newBlockMust(builtin.fn),
		)
		if !ok {
			panic(internal)
		}
	}
	for _, builtin := range builtinOther {
		builtinEnv, ok = builtinEnv.insert(builtin.name, builtin.value)
		if !ok {
			panic(internal)
		}
	}
}
