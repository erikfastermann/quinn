package runtime

import (
	"fmt"

	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/value"
)

var builtinOther = []struct {
	name  Atom
	value value.Value
}{
	{"false", Bool(false)},
	{"true", Bool(true)},
	{"stop", IterationStop{}},
}

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
			return nil, nil, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		return next, unit, nil
	}},
	{"==", func(x, y value.Value) (value.Value, error) {
		return Bool(x.Eq(y)), nil
	}},
	{"!=", func(x, y value.Value) (value.Value, error) {
		return Bool(!x.Eq(y)), nil
	}},
	{">=", func(x, y number.Number) (value.Value, error) {
		return Bool(x.Cmp(y) >= 0), nil
	}},
	{"not", func(b Bool) (value.Value, error) {
		return !b, nil
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
				return v, fmt.Errorf("argument has to be atom, got %s", v)
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
			return nil, fmt.Errorf("can't create argumented block from other than basic block")
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
			return nil, nil, fmt.Errorf("can't create argumented block from other than basic block")
		}

		next, ok := env.insert(Atom(symbol), blockV)
		if !ok {
			return nil, nil, fmt.Errorf(
				"couldn't assign to name, %s already exists",
				symbol.String(),
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
		if b, isBool := cond.(Bool); (isBool && !bool(b)) || isUnit {
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
				"index out of range (%s with length %d)",
				idx,
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
		b, err := newBlockFromFn(builtin.fn)
		if err != nil {
			panic(internal + ": " + err.Error())
		}
		envWithBuiltins, ok = envWithBuiltins.insert(builtin.name, b)
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
