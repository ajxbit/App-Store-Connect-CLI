package storekit

import (
	"strconv"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

func printOutput(data any, format string, pretty bool, headers []string, rows [][]string) error {
	return shared.PrintOutputWithRenderers(
		data,
		format,
		pretty,
		func() error { asc.RenderTable(headers, rows); return nil },
		func() error { asc.RenderMarkdown(headers, rows); return nil },
	)
}

func boolString(value bool) string { return strconv.FormatBool(value) }
