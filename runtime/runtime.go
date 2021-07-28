package runtime

import (
	"fmt"

	"github.com/erikfastermann/quinn/number"
	"github.com/erikfastermann/quinn/parser"
	"github.com/erikfastermann/quinn/value"
)

// TODO: better naming matcher

const internal = "internal error"

var (
	tagReturner = value.NewTag()
	tagEq       = value.NewTag()
	tagStringer = value.NewTag()
	tagMatcher  = value.NewTag()
)

func newTagMatcher(tagFuncPairs ...interface{}) func(value.Value, value.Tag) (value.Value, bool) {
	if len(tagFuncPairs)%2 != 0 {
		panic(internal)
	}

	m := make(map[value.Tag]value.Value)
	for i := 0; i < len(tagFuncPairs); i += 2 {
		tag, ok := tagFuncPairs[i].(value.Tag)
		if !ok {
			panic(internal)
		}
		fn := tagFuncPairs[i+1]
		if _, ok := m[tag]; ok {
			panic(internal)
		}
		m[tag] = newBlockMust(fn)
	}

	return func(_ value.Value, tag value.Tag) (value.Value, bool) {
		v, ok := m[tag]
		return v, ok
	}
}

func matcherEq(matcher, v value.Value) (value.Value, error) {
	bV, err := eq(matcher, v)
	if err != nil {
		return nil, err
	}
	return List{[]value.Value{bV, List{}}}, nil
}

var tagValues map[value.Tag]func(value.Value, value.Tag) (v value.Value, ok bool)

func init() {
	// needed to avoid init loop
	tagValues = map[value.Tag]func(value.Value, value.Tag) (value.Value, bool){
		tagUnit: newTagMatcher(
			tagEq, eqUnit,
			tagStringer, stringerUnit,
			tagMatcher, matcherEq,
		),
		tagBool: newTagMatcher(
			tagEq, eqBool,
			tagStringer, stringerBool,
			tagMatcher, matcherEq,
		),
		number.Tag(): newTagMatcher(
			tagEq, eqNumber,
			tagStringer, stringerNumber,
			tagMatcher, matcherEq,
		),
		tagString: newTagMatcher(
			tagEq, eqString,
			tagStringer, stringerString,
			tagMatcher, matcherEq,
		),
		tagAtom: newTagMatcher(
			tagEq, eqAtom,
			tagStringer, stringerAtom,
			tagMatcher, matcherAtom,
		),
		tagList: newTagMatcher(
			tagEq, eqList,
			tagStringer, stringerList,
			tagMatcher, matcherList,
		),
		tagMut: newTagMatcher(
			tagEq, eqMut,
			tagStringer, stringerMut,
			tagMatcher, matcherEq,
		),
		tagBlock: newTagMatcher(tagStringer, stringerBlock),
		tagTag: newTagMatcher(
			tagEq, eqTag,
			tagStringer, stringerTag,
			tagMatcher, matcherEq,
		),
		tagOpaque: opaqueMatcher,
	}
}

func valueString(v value.Value) string {
	// should we also recover panics?
	// should we try to string until cycle?

	if v == nil {
		return "<unknown (value is nil)>"
	}
	attrs, ok := tagValues[v.Tag()]
	if !ok {
		return fmt.Sprintf("<%T (error: tag not found)>", v)
	}
	stringer, ok := attrs(v, tagStringer)
	if !ok {
		return fmt.Sprintf("<%T (error: no stringer attribute)", v)
	}
	b, ok := stringer.(Block)
	if !ok {
		return fmt.Sprintf(
			"<%T (error: stringer attribute is not a block, but a %T)",
			v,
			stringer,
		)
	}
	sV, err := b.runWithoutEnv(v)
	if err != nil {
		return fmt.Sprintf("<%T (stringer error: %v)", v, err)
	}
	s, ok := sV.(String)
	if !ok {
		return fmt.Sprintf(
			"<%T (error: stringer returned non string value (%T))",
			v,
			sV,
		)
	}
	return string(s)
}

func getAttribute(v value.Value, tag value.Tag) (value.Value, error) {
	attrs, ok := tagValues[v.Tag()]
	if !ok {
		return nil, fmt.Errorf("%s: value tag not found", valueString(v))
	}
	attr, ok := attrs(v, tag)
	if !ok {
		return nil, fmt.Errorf("%s: attribute tag not found", valueString(v))
	}
	return attr, nil
}

func getAttributeBlock(v value.Value, tag value.Tag) (Block, error) {
	attr, err := getAttribute(v, tag)
	if err != nil {
		return nil, err
	}
	b, ok := attr.(Block)
	if !ok {
		return nil, fmt.Errorf("%s: attribute is not a Block", valueString(v))
	}
	return b, nil
}

// TODO: should eq return Bool?
func eq(x, y value.Value) (value.Value, error) {
	b, err := getAttributeBlock(x, tagEq)
	if err != nil {
		return nil, err
	}
	return b.runWithoutEnv(x, y)
}

func Run(env *Environment, block parser.Block) (*Environment, error) {
	if env == nil {
		env = builtinEnv
	}
	env, _, err := runCode(env, block)
	return env, err
}
