package cmdtest

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	reviewScreenshotPNGOnce  sync.Once
	reviewScreenshotPNGBytes []byte
)

// reviewScreenshotPNG returns an opaque PNG at an accepted App Review
// screenshot size (iPhone 3.5-inch without status bar), encoded once.
func reviewScreenshotPNG(t *testing.T) []byte {
	t.Helper()
	reviewScreenshotPNGOnce.Do(func() {
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 640, 920))); err != nil {
			panic(fmt.Sprintf("encode review screenshot fixture: %v", err))
		}
		reviewScreenshotPNGBytes = buf.Bytes()
	})
	return append([]byte(nil), reviewScreenshotPNGBytes...)
}

// writeReviewScreenshotPNG writes an opaque PNG that passes the local review
// screenshot preflight.
func writeReviewScreenshotPNG(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, reviewScreenshotPNG(t), 0o600); err != nil {
		t.Fatalf("write review screenshot fixture: %v", err)
	}
}

func writeOpaquePNG(t *testing.T, path string, width, height int) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write png: %v", err)
	}
}

// failOnAnyRequest installs a transport that records any App Store Connect
// request, proving the preflight rejected the file before contacting it.
func failOnAnyRequest(t *testing.T) *int {
	t.Helper()
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	requests := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
	})
	return &requests
}

func runReviewScreenshotCommand(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	root := RootCommand("1.2.3")
	root.FlagSet.SetOutput(io.Discard)

	var runErr error
	stdout, stderr := captureOutput(t, func() {
		if err := root.Parse(args); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		runErr = root.Run(context.Background())
	})
	return stdout, stderr, runErr
}

func TestReviewScreenshotUploadsRejectUnsupportedDimensionsBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name   string
		args   func(path string) []string
		prefix string
	}{
		{
			name: "subscriptions review screenshots create",
			args: func(path string) []string {
				return []string{"subscriptions", "review", "screenshots", "create", "--subscription-id", "8000000001", "--file", path}
			},
			prefix: "subscriptions review screenshots create: ",
		},
		{
			name: "iap review-screenshots create",
			args: func(path string) []string {
				return []string{"iap", "review-screenshots", "create", "--iap-id", "9000000001", "--file", path}
			},
			prefix: "iap review-screenshots create: ",
		},
		{
			name: "iap review-screenshots update",
			args: func(path string) []string {
				return []string{"iap", "review-screenshots", "update", "--screenshot-id", "shot-1", "--file", path}
			},
			prefix: "iap review-screenshots update: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupAuth(t)
			t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
			requests := failOnAnyRequest(t)

			path := filepath.Join(t.TempDir(), "paywall.png")
			writeOpaquePNG(t, path, 1179, 2560)

			stdout, _, err := runReviewScreenshotCommand(t, tt.args(path))
			if err == nil {
				t.Fatal("expected unsupported dimensions error")
			}
			if *requests != 0 {
				t.Fatalf("expected no App Store Connect requests, got %d", *requests)
			}
			if stdout != "" {
				t.Fatalf("expected empty stdout, got %q", stdout)
			}
			for _, want := range []string{
				tt.prefix + fmt.Sprintf("review screenshot %q is 1179x2560 pixels", path),
				"nearest accepted size: 1179x2556",
				"1290x2796",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

func TestReviewScreenshotUploadsRejectMislabeledFormatBeforeAnyRequest(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
	requests := failOnAnyRequest(t)

	path := filepath.Join(t.TempDir(), "review.png")
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 1290, 2796)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write jpeg: %v", err)
	}

	_, _, err := runReviewScreenshotCommand(t, []string{"iap", "review-screenshots", "create", "--iap-id", "9000000001", "--file", path})
	if err == nil {
		t.Fatal("expected mislabeled format error")
	}
	if *requests != 0 {
		t.Fatalf("expected no App Store Connect requests, got %d", *requests)
	}
	want := fmt.Sprintf("review screenshot %q is JPEG data but has a .png extension; rename it to review.jpg", path)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestSubscriptionsSetupRejectsUnsupportedReviewScreenshotBeforeAnyRequest(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
	requests := failOnAnyRequest(t)

	path := filepath.Join(t.TempDir(), "paywall.png")
	writeOpaquePNG(t, path, 1024, 1024)

	stdout, stderr, err := runReviewScreenshotCommand(t, []string{
		"subscriptions", "setup",
		"--group-id", "GROUP_ID",
		"--reference-name", "Pro Monthly",
		"--product-id", "com.example.pro.monthly",
		"--review-screenshot", path,
	})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if *requests != 0 {
		t.Fatalf("expected no App Store Connect requests, got %d", *requests)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	want := fmt.Sprintf("invalid --review-screenshot: review screenshot %q is 1024x1024 pixels", path)
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr %q does not contain %q", stderr, want)
	}
}

func TestIAPImportRejectsUnsupportedReviewScreenshotBeforeAnyRequest(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
	requests := failOnAnyRequest(t)

	dir := t.TempDir()
	filePath := writeIAPImportFile(t, dir, `{"products":[{"type":"CONSUMABLE","referenceName":"Coins","productId":"com.example.coins","reviewScreenshot":"shots/coins.png"}]}`)
	if err := os.MkdirAll(filepath.Join(dir, "shots"), 0o755); err != nil {
		t.Fatalf("create screenshot dir: %v", err)
	}
	writeOpaquePNG(t, filepath.Join(dir, "shots", "coins.png"), 40, 40)

	_, stderr, err := runIAPImport(t, []string{"iap", "import", "--app", "123456789", "--file", filePath, "--confirm", "--output", "json"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if *requests != 0 {
		t.Fatalf("expected no App Store Connect requests, got %d", *requests)
	}
	want := `review screenshot "shots/coins.png" is 40x40 pixels, which matches no App Store screenshot size`
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr %q does not contain %q", stderr, want)
	}
}

func TestIAPReviewScreenshotsCreateWarnsAboutAlphaChannelAndUploads(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	path := filepath.Join(t.TempDir(), "review.png")
	img := image.NewNRGBA(image.Rect(0, 0, 640, 920))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write png: %v", err)
	}
	size := len(buf.Bytes())

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	uploaded := false
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v1/inAppPurchaseAppStoreReviewScreenshots":
			return jsonResponse(http.StatusCreated, fmt.Sprintf(`{"data":{"type":"inAppPurchaseAppStoreReviewScreenshots","id":"shot-1","attributes":{"fileName":"review.png","fileSize":%d,"uploadOperations":[{"method":"PUT","url":"https://upload.example.com/upload/shot-1","length":%d,"offset":0}]}}}`, size, size))
		case req.Method == http.MethodPut && req.URL.Host == "upload.example.com":
			uploaded = true
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		case req.Method == http.MethodPatch && req.URL.Path == "/v1/inAppPurchaseAppStoreReviewScreenshots/shot-1":
			return jsonResponse(http.StatusOK, `{"data":{"type":"inAppPurchaseAppStoreReviewScreenshots","id":"shot-1","attributes":{"fileName":"review.png"}}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/v1/inAppPurchaseAppStoreReviewScreenshots/shot-1":
			return jsonResponse(http.StatusOK, `{"data":{"type":"inAppPurchaseAppStoreReviewScreenshots","id":"shot-1","attributes":{"fileName":"review.png","assetDeliveryState":{"state":"COMPLETE"}}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	_, stderr, err := runReviewScreenshotCommand(t, []string{"iap", "review-screenshots", "create", "--iap-id", "9000000001", "--file", path, "--output", "json"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !uploaded {
		t.Fatal("expected the screenshot to upload despite the warning")
	}
	want := fmt.Sprintf("Warning: review screenshot %q has an alpha channel", path)
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr %q does not contain %q", stderr, want)
	}
}

func TestIAPImportRechecksReviewScreenshotReplacedAfterPlanning(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	dir := t.TempDir()
	filePath := writeIAPImportFile(t, dir, `{"products":[{"type":"CONSUMABLE","referenceName":"Coins","productId":"com.example.coins","reviewScreenshot":"shots/coins.png"}]}`)
	screenshotPath := writeIAPImportScreenshot(t, dir)

	createdProduct := false
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/apps/123456789/inAppPurchasesV2":
			writeOpaquePNG(t, screenshotPath, 40, 40)
			return jsonResponse(http.StatusOK, `{"data":[]}`)
		case req.Method == http.MethodPost && req.URL.Path == "/v2/inAppPurchases":
			createdProduct = true
			return jsonResponse(http.StatusCreated, `{"data":{"type":"inAppPurchases","id":"iap-1","attributes":{}}}`)
		default:
			t.Fatalf("unexpected request after the screenshot was replaced: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	_, stderr, err := runIAPImport(t, []string{"iap", "import", "--app", "123456789", "--file", filePath, "--confirm", "--output", "json"})
	if err == nil || !createdProduct {
		t.Fatalf("run error = %v, product created = %t, stderr = %q; want rejection after product creation", err, createdProduct, stderr)
	}
	if want := `review screenshot "shots/coins.png" is 40x40 pixels`; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestIAPImportWarnsAboutAlphaChannelOnceAtUpload(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	dir := t.TempDir()
	filePath := writeIAPImportFile(t, dir, `{"products":[{"type":"CONSUMABLE","referenceName":"Coins","productId":"com.example.coins","reviewScreenshot":"shots/coins.png"}]}`)
	screenshotPath := writeIAPImportScreenshot(t, dir)
	img := image.NewNRGBA(image.Rect(0, 0, 640, 920))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(screenshotPath, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write png: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	reserved := false
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/apps/123456789/inAppPurchasesV2":
			return jsonResponse(http.StatusOK, `{"data":[]}`)
		case req.Method == http.MethodPost && req.URL.Path == "/v2/inAppPurchases":
			return jsonResponse(http.StatusCreated, `{"data":{"type":"inAppPurchases","id":"iap-1","attributes":{}}}`)
		case req.Method == http.MethodPost && req.URL.Path == "/v1/inAppPurchaseAppStoreReviewScreenshots":
			reserved = true
			return jsonResponse(http.StatusCreated, `{"data":{"type":"inAppPurchaseAppStoreReviewScreenshots","id":"shot-1","attributes":{"fileName":"coins.png"}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	_, stderr, _ := runIAPImport(t, []string{"iap", "import", "--app", "123456789", "--file", filePath, "--confirm", "--output", "json"})
	if !reserved {
		t.Fatalf("expected the screenshot upload to proceed despite the warning; stderr = %q", stderr)
	}
	if got := strings.Count(stderr, `Warning: review screenshot "shots/coins.png" has an alpha channel`); got != 1 {
		t.Fatalf("alpha warnings = %d, want exactly 1; stderr = %q", got, stderr)
	}
}

func TestSubscriptionsSetupRechecksReviewScreenshotReplacedAfterValidation(t *testing.T) {
	setupAuth(t)
	t.Setenv("ASC_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))

	path := filepath.Join(t.TempDir(), "review.png")
	writeReviewScreenshotPNG(t, path)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	createdSubscription := false
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/subscriptionGroups/group-1/subscriptions":
			return jsonResponse(http.StatusOK, `{"data":[],"links":{"next":""}}`)
		case req.Method == http.MethodPost && req.URL.Path == "/v1/subscriptions":
			createdSubscription = true
			writeOpaquePNG(t, path, 1024, 1024)
			return jsonResponse(http.StatusCreated, `{"data":{"type":"subscriptions","id":"sub-1","attributes":{"name":"Pro Monthly","productId":"com.example.pro.monthly","subscriptionPeriod":"ONE_MONTH","state":"MISSING_METADATA"}}}`)
		default:
			t.Fatalf("unexpected request after the screenshot was replaced: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	_, stderr, err := runReviewScreenshotCommand(t, []string{
		"subscriptions", "setup",
		"--group-id", "group-1",
		"--reference-name", "Pro Monthly",
		"--product-id", "com.example.pro.monthly",
		"--subscription-period", "ONE_MONTH",
		"--review-screenshot", path,
		"--output", "json",
	})
	if err == nil || !createdSubscription {
		t.Fatalf("run error = %v, subscription created = %t, stderr = %q; want rejection after subscription creation", err, createdSubscription, stderr)
	}
	if want := fmt.Sprintf("review screenshot %q is 1024x1024 pixels", path); !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}
