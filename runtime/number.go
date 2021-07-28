package runtime

import (
	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/value"
)

func eqNumber(n number.Number, v value.Value) (value.Value, error) {
	n2, ok := v.(number.Number)
	return NewBool(ok && n.Eq(n2)), nil
}

func stringerNumber(n number.Number) (value.Value, error) {
	return String(n.String()), nil
}
