package value

import "sync/atomic"

type Value interface {
	Tag() Tag
}

type Tag struct{ id int64 }

var tagIndex = int64(0)

func NewTag() Tag {
	id := atomic.AddInt64(&tagIndex, 1)
	if id < 0 {
		panic("overflow")
	}
	return Tag{id}
}

func (t Tag) Valid() bool {
	return t.id != 0
}

var tagTag = NewTag()

func (t Tag) Tag() Tag {
	return tagTag
}
