package number

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/erikfastermann/quinn/value"
)

type Number struct {
	r big.Rat
}

func FromInt(x int) Number {
	var r big.Rat
	r.Num().SetInt64(int64(x))
	return Number{r}
}

func FromString(s string) (Number, error) {
	var r big.Rat
	if _, ok := r.SetString(s); !ok {
		// TODO: better error message
		return Number{}, fmt.Errorf("%q is not a valid number")
	}
	return Number{r}, nil
}

func (x Number) Eq(v value.Value) bool {
	y, ok := v.(Number)
	return ok && x.r.Cmp(&y.r) == 0
}

func (x Number) Cmp(y Number) int {
	return x.r.Cmp(&y.r)
}

func (x Number) Add(y Number) Number {
	var z big.Rat
	z.Add(&x.r, &y.r)
	return Number{z}
}

func (x Number) Sub(y Number) Number {
	var z big.Rat
	z.Sub(&x.r, &y.r)
	return Number{z}
}

func (x Number) Neg() Number {
	var z big.Rat
	z.Neg(&x.r)
	return Number{z}
}

var errZeroDenominator = errors.New("denominator is zero")

func (x Number) Mul(y Number) Number {
	var z big.Rat
	z.Mul(&x.r, &y.r)
	return Number{z}
}

func (x Number) Div(y Number) (Number, error) {
	if y.r.Sign() == 0 {
		return Number{}, errZeroDenominator
	}
	var z big.Rat
	z.Quo(&x.r, &y.r)
	return Number{z}, nil
}

func (x Number) Mod(y Number) (Number, error) {
	if err := x.checkInt(); err != nil {
		return Number{}, err
	}
	if err := y.checkInt(); err != nil {
		return Number{}, err
	}
	if y.r.Sign() == 0 {
		return Number{}, errZeroDenominator
	}

	var z big.Int
	z.Rem(x.r.Num(), y.r.Num())
	var r big.Rat
	r.SetInt(&z)
	return Number{r}, nil
}

func (x Number) String() string {
	return x.r.RatString()
}

func (x Number) Signed() (int, error) {
	if err := x.checkInt(); err != nil {
		return 0, err
	}
	num := x.r.Num()
	i64 := num.Int64()
	if !num.IsInt64() || int64(int(i64)) != i64 {
		return 0, fmt.Errorf("%s is too large", x)
	}
	return int(i64), nil
}

func (x Number) Unsigned() (int, error) {
	if err := x.checkInt(); err != nil {
		return 0, err
	}
	num := x.r.Num()
	if num.Sign() < 0 {
		return 0, fmt.Errorf("%s is smaller than 0", x)
	}
	i64 := num.Int64()
	if !num.IsInt64() || int64(int(i64)) != i64 {
		return 0, fmt.Errorf("%s is too large", x)
	}
	return int(i64), nil
}

func (x Number) checkInt() error {
	if !x.r.IsInt() {
		return fmt.Errorf("%s is not an integer", x)
	}
	return nil
}
