package runtime

import (
	"fmt"

	"github.com/erikfastermann/quinn/value"
)

var tagMut = value.NewTag()

type Mut struct {
	v value.Value
}

func (*Mut) Tag() value.Tag {
	return tagMut
}

// TODO: should Mut implement eq?
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
