package runtime

import (
	"errors"
	"fmt"
	"math/big"
)

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
	{"mut", func(v Value) (Value, error) {
		return &Mut{v}, nil
	}},
	{"load", func(target *Mut) (Value, error) {
		return target.v, nil
	}},
	{"<-", func(target *Mut, v Value) (Value, error) {
		target.v = v
		return unit, nil
	}},
	{"=", func(env *environment, assignee Atom, v Value) (*environment, Value, error) {
		next, ok := env.insert(assignee, v)
		if !ok {
			return nil, nil, fmt.Errorf("couldn't assign to name, %s already exists", assignee)
		}
		return next, unit, nil
	}},
	{"==", func(x, y Value) (Value, error) {
		return Bool(x.Eq(y)), nil
	}},
	{"!=", func(x, y Value) (Value, error) {
		return Bool(!x.Eq(y)), nil
	}},
	{">=", func(x, y *Number) (Value, error) {
		return Bool(x.Cmp(&y.Rat) >= 0), nil
	}},
	{"not", func(b Bool) (Value, error) {
		return !b, nil
	}},
	{"+", func(x, y *Number) (Value, error) {
		var z big.Rat
		z.Add(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"-", func(x, y *Number) (Value, error) {
		var z big.Rat
		z.Sub(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"neg", func(x *Number) (Value, error) {
		var z big.Rat
		z.Neg(&x.Rat)
		return &Number{z}, nil
	}},
	{"*", func(x, y *Number) (Value, error) {
		var z big.Rat
		z.Mul(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"/", func(x, y *Number) (Value, error) {
		var zero big.Rat
		if y.Cmp(&zero) == 0 {
			return nil, errors.New("denominator is zero")
		}

		var z big.Rat
		z.Quo(&x.Rat, &y.Rat)
		return &Number{z}, nil
	}},
	{"%%", func(x, y *Number) (Value, error) {
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
	{"->", func(def List, block Block) (Value, error) {
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
	{"defop", func(env *environment, symbol String, lhs, rhs Atom, block Block) (*environment, Value, error) {
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
	{"if", func(cond Value, tBlock Block, blocks ...Block) (Value, error) {
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
	{"loop", func(block Block) (Value, error) {
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
	{"@", func(l List, num *Number) (Value, error) {
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
	{"len", func(l List) (Value, error) {
		var r big.Rat
		r.SetInt64(int64(len(l.data)))
		return &Number{r}, nil
	}},
	{"append", func(l List, v Value) (Value, error) {
		next := make([]Value, len(l.data)+1)
		copy(next, l.data)
		next[len(next)-1] = v
		return List{next}, nil
	}},
	{"append_list", func(l, l2 List) (Value, error) {
		// TODO: if a list is empty, don't copy
		next := make([]Value, len(l.data)+len(l2.data))
		n := copy(next, l.data)
		copy(next[n:], l2.data)
		return List{next}, nil

	}},
	{"slice", func(l List, fromN, toN *Number) (Value, error) {
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
	{"call", func(b Block, args List) (Value, error) {
		return b.runWithoutEnv(args.data...)
	}},
	{"println", func(args ...Value) (Value, error) {
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
