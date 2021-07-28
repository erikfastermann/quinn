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

var noMatch value.Value = List{[]value.Value{falseValue, List{}}}

func matcherList(matcher List, v value.Value) (value.Value, error) {
	const errMatcherReturn = "expected matcher to return a pair of bool " +
		"and list of unique atom and value pairs, got %s"

	candidate, ok := v.(List)
	if !ok {
		return noMatch, nil
	}
	if len(matcher.data) != len(candidate.data) {
		return noMatch, nil
	}

	out := make([]value.Value, 0)
	for i := range matcher.data {
		m, c := matcher.data[i], candidate.data[i]
		b, err := getAttributeBlock(m, tagMatcher)
		if err != nil {
			return nil, err
		}
		nextV, err := b.runWithoutEnv(m, c)
		if err != nil {
			return nil, err
		}
		next, ok := nextV.(List)
		if !ok {
			return nil, fmt.Errorf(errMatcherReturn, valueString(nextV))
		}
		if len(next.data) != 2 {
			return nil, fmt.Errorf(errMatcherReturn, valueString(nextV))
		}
		matchedV, pairsV := next.data[0], next.data[1]
		matched, ok := matchedV.(Bool)
		if !ok {
			return nil, fmt.Errorf(errMatcherReturn, valueString(nextV))
		}
		if !matched.AsBool() {
			return noMatch, nil
		}
		pairs, ok := pairsV.(List)
		if !ok {
			return nil, fmt.Errorf(errMatcherReturn, valueString(nextV))
		}
		out = append(out, pairs.data...)
	}

	return List{[]value.Value{trueValue, List{out}}}, nil
}
