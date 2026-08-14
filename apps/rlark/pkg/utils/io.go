package utils

import "io"

// CloseIO closes the given closer, ignoring errors.
func CloseIO(c io.Closer) {
	if c != nil {
		_ = c.Close()
	}
}
