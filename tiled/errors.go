package tiled

import (
	"errors"
	"fmt"
)

// unsupported is the error for a Tiled feature the engine does not support.
func unsupported(feature, detail string) error {
	if detail == "" {
		return fmt.Errorf("unsupported Tiled feature: %s", feature)
	}
	return fmt.Errorf("unsupported Tiled feature: %s: %s", feature, detail)
}

// withContext prefixes every error inside err (a joined error is flattened)
// with prefix, so that each line of a report says where the problem is:
// `layer "main": object 3: ...`. It returns nil for a nil err.
func withContext(prefix string, err error) error {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		inner := j.Unwrap()
		out := make([]error, len(inner))
		for i, e := range inner {
			out[i] = withContext(prefix, e)
		}
		return errors.Join(out...)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
