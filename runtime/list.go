package runtime

import (
	"fmt"
	"strings"

	"github.com/erikfastermann/quinn/value"
)

var tagList = value.NewTag()

type List struct {
	// TODO: use persistent array
	data []value.Value
}

func (List) Tag() value.Tag {
	return tagList
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

var stringEmptyList value.Value = String("[]")

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
