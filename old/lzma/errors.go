package lzma

// lerror represents an LZMA-specific error. It currently adds the prefix
// "lzma -" to all errors created in the package.
type lerror struct {
	Msg string
}

// Error returns the error message with the prefix "lzma: ".
func (e lerror) Error() string {
	return "lzma: " + e.Msg
}

// newError creates a new lzma error with the given message.
func newError(msg string) error {
	return lerror{msg}
}