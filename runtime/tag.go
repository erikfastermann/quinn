package runtime

import "github.com/erikfastermann/quinn/value"

var tagTag = value.Tag{}.Tag()

func eqTag(t value.Tag, v value.Value) (value.Value, error) {
	t2, ok := v.(value.Tag)
	return NewBool(ok && t == t2), nil
}

var stringTag value.Value = String("tag")

func stringerTag(_ value.Tag) (value.Value, error) {
	return stringTag, nil
}
