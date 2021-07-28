package runtime

import "github.com/erikfastermann/quinn/value"

var tagBool = value.NewTag()

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

var (
	stringFalse value.Value = String("false")
	stringTrue  value.Value = String("true")
)

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
