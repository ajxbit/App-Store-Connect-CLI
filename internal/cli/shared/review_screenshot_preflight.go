package shared

import (
	"fmt"
	"io"
	"os"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

// PreflightReviewScreenshot checks an in-app purchase or subscription App
// Review screenshot locally before any App Store Connect request, so a file
// App Store Connect would reject at delivery fails with the actual and
// accepted values instead of leaving a failed screenshot on the product.
// Warnings for documented but unconfirmed constraints go to stderr.
func PreflightReviewScreenshot(path string, file io.ReaderAt, size int64) error {
	warnings, err := asc.CheckReviewScreenshotImage(path, io.NewSectionReader(file, 0, size))
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	return nil
}
