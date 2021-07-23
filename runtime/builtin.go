package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/value"
)

var (
	tagStringer = value.NewTag()
	tagEq       = value.NewTag()
)

var (
	stringUnit          value.Value = String("()")
	stringFalse         value.Value = String("false")
	stringTrue          value.Value = String("true")
	stringEmptyList     value.Value = String("[]")
	stringIterationStop value.Value = String("<iteration stop>")
	stringBlock         value.Value = String("<block>")
)

func eqUnit(_ Unit, v value.Value) (value.Value, error) {
	_, ok := v.(Unit)
	return NewBool(ok), nil
}

func stringerUnit(_ Unit) (value.Value, error) {
	return stringUnit, nil
}

func eqBool(b Bool, v value.Value) (value.Value, error) {
	b2, ok := v.(Bool)
	return NewBool(ok && b.AsBool() == b2.AsBool()), nil
}

func stringerBool(b Bool) (value.Value, error) {
	if b.AsBool() {
		return stringTrue, nil
	}
	return stringFalse, nil
}

func eqNumber(n number.Number, v value.Value) (value.Value, error) {
	n2, ok := v.(number.Number)
	return NewBool(ok && n.Eq(n2)), nil
}

func stringerNumber(n number.Number) (value.Value, error) {
	return String(n.String()), nil
}

func eqString(s String, v value.Value) (value.Value, error) {
	s2, ok := v.(String)
	return NewBool(ok && s == s2), nil
}

func stringerString(s String) (value.Value, error) {
	return s, nil
}

func eqAtom(a Atom, v value.Value) (value.Value, error) {
	a2, ok := v.(Atom)
	return NewBool(ok && a == a2), nil
}

func stringerAtom(a Atom) (value.Value, error) {
	return String(string(a)), nil
}

func eqList(l List, v value.Value) (value.Value, error) {
	l2, ok := v.(List)
	if !ok || len(l.data) != len(l2.data) {
		return falseValue, nil
	}
	for i := range l.data {
		// TODO: check cycle?
		bV, err := eq(l.data[i], l2.data[i])
		if err != nil {
			return nil, err
		}
		b, ok := bV.(Bool)
		if !ok {
			return nil, fmt.Errorf("list equal: expected bool, got %s", valueString(bV))
		}
		if !b.AsBool() {
			return falseValue, nil
		}
	}
	return trueValue, nil
}

func stringerList(l List) (value.Value, error) {
	// TODO: check cycle?

	if len(l.data) == 0 {
		return stringEmptyList, nil
	}

	var b strings.Builder
	b.WriteString("[")
	for _, v := range l.data[:len(l.data)-1] {
		b.WriteString(valueString(v))
		b.WriteString(" ")
	}
	b.WriteString(valueString(l.data[len(l.data)-1]))
	b.WriteString("]")
	return String(b.String()), nil
}

func eqIterationStop(_ IterationStop, v value.Value) (value.Value, error) {
	_, ok := v.(IterationStop)
	return NewBool(ok), nil
}

func stringerIterationStop(_ IterationStop) (value.Value, error) {
	return stringIterationStop, nil
}

func eqMut(m *Mut, v value.Value) (value.Value, error) {
	m2, ok := v.(*Mut)
	if !ok {
		return falseValue, nil
	}
	// TODO: check cycle?
	return eq(m.v, m2.v)
}

func stringerMut(m *Mut) (value.Value, error) {
	return String(fmt.Sprintf("(mut %s)", valueString(m.v))), nil
}

func eqBlock(_ Block, _ value.Value) (value.Value, error) {
	return falseValue, nil
}

func stringerBlock(_ Block) (value.Value, error) {
	return stringBlock, nil
}

var tagValues map[value.Tag]map[value.Tag]value.Value = nil

func init() {
	// needed to avoid init loop
	tagValues = map[value.Tag]map[value.Tag]value.Value{
		tagUnit: {
			tagEq:       newBlockMust(eqUnit),
			tagStringer: newBlockMust(stringerUnit),
		},
		tagBool: {
			tagEq:       newBlockMust(eqBool),
			tagStringer: newBlockMust(stringerBool),
		},
		number.Tag(): {
			tagEq:       newBlockMust(eqNumber),
			tagStringer: newBlockMust(stringerNumber),
		},
		tagString: {
			tagEq:       newBlockMust(eqString),
			tagStringer: newBlockMust(stringerString),
		},
		tagAtom: {
			tagEq:       newBlockMust(eqAtom),
			tagStringer: newBlockMust(stringerAtom),
		},
		tagList: {
			tagEq:       newBlockMust(eqList),
			tagStringer: newBlockMust(stringerList),
		},
		tagIterationStop: {
			tagEq:       newBlockMust(eqIterationStop),
			tagStringer: newBlockMust(stringerIterationStop),
		},
		tagMut: {
			tagEq:       newBlockMust(eqMut),
			tagStringer: newBlockMust(stringerMut),
		},
		tagBlock: {
			tagEq:       newBlockMust(eqBlock),
			tagStringer: newBlockMust(stringerBlock),
		},
	}
}

func valueString(v value.Value) string {
	// should we also recover panics?
	// should we try to string until cycle?

	if v == nil {
		return "<unknown (value is nil)>"
	}
	attr, ok := tagValues[v.Tag()]
	if !ok {
		return fmt.Sprintf("<%T (error: tag not found)>", v)
	}
	stringer, ok := attr[tagStringer]
	if !ok {
		return fmt.Sprintf("<%T (error: no stringer attribute)", v)
	}
	b, ok := stringer.(Block)
	if !ok {
		return fmt.Sprintf(
			"<%T (error: stringer attribute is not a block, but a %T)",
			v,
			stringer,
		)
	}
	sV, err := b.runWithoutEnv(v)
	if err != nil {
		return fmt.Sprintf("<%T (stringer error: %v)", v, err)
	}
	s, ok := sV.(String)
	if !ok {
		return fmt.Sprintf(
			"<%T (error: stringer returned non string value (%T))",
			v,
			sV,
		)
	}
	return string(s)
}

func eq(x, y value.Value) (value.Value, error) {
	attr, ok := tagValues[x.Tag()]
	if !ok {
		return nil, fmt.Errorf("can't compare %s: tag not found", valueString(x))
	}
	eq, ok := attr[tagEq]
	if !ok {
		return nil, fmt.Errorf("can't compare %s: no eq attribute", valueString(x))
	}
	b, ok := eq.(Block)
	if !ok {
		return nil, fmt.Errorf("can't compare %s: eq is not a Block", valueString(x))
	}
	return b.runWithoutEnv(x, y)
}

var builtinOther = []struct {
	name  Atom
	value value.Value
}{
	{"false", falseValue},
	{"true", trueValue},
	{"stop", IterationStop{}},
}

var errNonBasicArgBlock = errors.New("can't create argumented block from non basic block")

var builtinBlocks = []struct {
	name Atom
	fn   interface{}
}{
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
	{"=", func(env *environment, assignee Atom, v value.Value) (*environment, value.Value, error) {
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
	{">=", func(x, y number.Number) (value.Value, error) {
		return NewBool(x.Cmp(y) >= 0), nil
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
	{"->", func(def List, block Block) (value.Value, error) {
		atoms := make([]Atom, len(def.data))
		for i, v := range def.data {
			atom, ok := v.(Atom)
			if !ok {
				return v, fmt.Errorf("argument has to be atom, got %s", valueString(v))
			}
			atoms[i] = atom
		}
		switch b := block.(type) {
		case basicBlock:
			block, err := b.withArgs(atoms...)
			if err != nil {
				return nil, err
			}
			return block, nil
		default:
			return nil, errNonBasicArgBlock
		}
	}},
	{"defop", func(env *environment, symbol String, lhs, rhs Atom, block Block) (*environment, value.Value, error) {
		// TODO: check symbol is valid operator

		var blockV Block
		switch b := block.(type) {
		case basicBlock:
			var err error
			blockV, err = b.withArgs(lhs, rhs)
			if err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, errNonBasicArgBlock
		}

		next, ok := env.insert(Atom(symbol), blockV)
		if !ok {
			return nil, nil, fmt.Errorf(
				"couldn't assign to name, %s already exists",
				valueString(symbol),
			)
		}
		return next, unit, nil
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
				return v, err
			}
			if _, isUnit := v.(Unit); !isUnit {
				return v, nil
			}
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

var envWithBuiltins *environment = nil

func init() {
	var ok bool
	for _, builtin := range builtinBlocks {
		envWithBuiltins, ok = envWithBuiltins.insert(
			builtin.name,
			newBlockMust(builtin.fn),
		)
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
