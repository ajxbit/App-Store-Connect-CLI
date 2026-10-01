package shared

import (
	"fmt"
	"io"
	"os"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

// PreflightReviewScreenshot checks the bytes of an in-app purchase or
// subscription App Review screenshot that are about to be uploaded. A format
// problem App Store Connect would reject at delivery is returned as an error
// before any request. Documented but unconfirmed constraints (size and alpha)
// and a payload that does not fully decode are printed to stderr as warnings.
func PreflightReviewScreenshot(path string, file io.ReaderAt, size int64) error {
	warnings, err := asc.CheckReviewScreenshotImage(path, io.NewSectionReader(file, 0, size))
	if err != nil {
		return err
	}
	if warning := asc.ReviewScreenshotDecodeWarning(path, file, size); warning != "" {
		warnings = append(warnings, warning)
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	return nil
}
