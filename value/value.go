package value

type Value interface {
	Eq(Value) bool
	String() string
}
