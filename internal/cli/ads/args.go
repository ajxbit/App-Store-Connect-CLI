package ads

import (
	"strings"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

func rejectUnexpectedArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return shared.UsageErrorf("unexpected argument(s): %s", strings.Join(args, " "))
}
