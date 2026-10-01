package shared

import "errors"

// ErrMissingWebSession reports that an `asc web` command needs a signed-in
// Apple web session that is not cached and cannot be created without a
// terminal. Like missing App Store Connect API credentials it is an
// authentication failure, not a usage error: it maps to the authentication exit
// code and is rendered with its own hint instead of the command's usage page.
var ErrMissingWebSession = errors.New("missing Apple web session")

// MissingWebSessionError is the rendered form of ErrMissingWebSession. Hint
// carries the next step the root error renderer prints after the message.
type MissingWebSessionError struct {
	Message string
	Hint    string
}

func (e *MissingWebSessionError) Error() string { return e.Message }

// Is lets errors.Is match ErrMissingWebSession through any wrapping.
func (e *MissingWebSessionError) Is(target error) bool { return target == ErrMissingWebSession }
