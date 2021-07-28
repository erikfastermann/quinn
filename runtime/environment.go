package runtime

import (
	"fmt"

	"github.com/erikfastermann/quinn/value"
)

type Environment struct {
	// TODO: use persistent map

	key         Atom
	value       value.Value
	left, right *Environment
}

func (env *Environment) get(k Atom) (value.Value, bool) {
	// TODO: iterative

	if env == nil {
		return nil, false
	}

	if k < env.key {
		return env.left.get(k)
	} else if k > env.key {
		return env.right.get(k)
	} else {
		return env.value, true
	}
}

func (env *Environment) insert(k Atom, v value.Value) (*Environment, bool) {
	if env == nil {
		return &Environment{k, v, nil, nil}, true
	}

	if k < env.key {
		next, ok := env.left.insert(k, v)
		if !ok {
			return nil, false
		}
		return &Environment{env.key, env.value, next, env.right}, true
	} else if k > env.key {
		next, ok := env.right.insert(k, v)
		if !ok {
			return nil, false
		}
		return &Environment{env.key, env.value, env.left, next}, true
	} else {
		return nil, false
	}
}

func (env *Environment) String() string {
	if env == nil {
		return ""
	}

	left := env.left.String()
	str := left
	if len(left) > 0 {
		str += " "
	}

	str += fmt.Sprintf("[%s %s]", env.key, valueString(env.value))

	right := env.right.String()
	if len(right) > 0 {
		str += " "
	}
	str += right

	return str
}
