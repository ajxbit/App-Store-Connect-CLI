package shared

import "github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"

// SanitizeTerminal removes characters interpreted by terminals and log viewers.
func SanitizeTerminal(input string) string {
	return asc.SanitizeTerminalText(input)
}
