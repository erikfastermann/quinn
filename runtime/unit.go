package runtime

import "github.com/erikfastermann/quinn/value"

var tagUnit = value.NewTag()

var stringUnit value.Value = String("()")

type Unit struct{}

var unit value.Value = Unit{}

func (Unit) Tag() value.Tag {
	return tagUnit
}

func eqUnit(_ Unit, v value.Value) (value.Value, error) {
	_, ok := v.(Unit)
	return NewBool(ok), nil
}

func stringerUnit(_ Unit) (value.Value, error) {
	return stringUnit, nil
}
