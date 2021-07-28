package runtime

import "github.com/erikfastermann/quinn/value"

var tagAtom = value.NewTag()

type Atom string

func (Atom) Tag() value.Tag {
	return tagAtom
}

func eqAtom(a Atom, v value.Value) (value.Value, error) {
	a2, ok := v.(Atom)
	return NewBool(ok && a == a2), nil
}

func stringerAtom(a Atom) (value.Value, error) {
	return String(string(a)), nil
}

func matcherAtom(a Atom, v value.Value) (value.Value, error) {
	return List{[]value.Value{
		trueValue,
		List{[]value.Value{
			List{[]value.Value{
				a,
				v,
			}},
		}},
	}}, nil
}
