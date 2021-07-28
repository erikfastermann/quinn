package runtime

import (
	"strconv"

	"github.com/erikfastermann/quinn/value"
)

var tagString = value.NewTag()

type String string

func (String) Tag() value.Tag {
	return tagString
}

func eqString(s String, v value.Value) (value.Value, error) {
	s2, ok := v.(String)
	return NewBool(ok && s == s2), nil
}

func stringerString(s String) (value.Value, error) {
	return String(strconv.Quote(string(s))), nil
}
