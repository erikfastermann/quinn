package runtime

import "github.com/erikfastermann/quinn/value"

var tagOpaque = value.NewTag()

type Opaque struct {
	tag   value.Tag
	v     value.Value
	attrs map[value.Tag]value.Value
}

func (o Opaque) Tag() value.Tag {
	return tagOpaque
}

func opaqueMatcher(v value.Value, tag value.Tag) (value.Value, bool) {
	o, ok := v.(Opaque)
	if !ok {
		return nil, false
	}
	attr, ok := o.attrs[tag]
	return attr, ok
}
