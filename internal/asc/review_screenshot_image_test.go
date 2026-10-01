package asc

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func encodeOpaquePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeTranslucentPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodeGIF(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

func TestCheckReviewScreenshotImageAcceptsDocumentedScreenshotSizes(t *testing.T) {
	tests := []struct {
		name string
		path string
		data func(t *testing.T) []byte
	}{
		{name: "iPhone 6.9 portrait PNG", path: "review.png", data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 1290, 2796) }},
		{name: "iPhone 6.9 landscape PNG", path: "review.png", data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 2796, 1290) }},
		{name: "iPhone 6.5 JPEG with .jpg", path: "review.jpg", data: func(t *testing.T) []byte { return encodeJPEG(t, 1242, 2688) }},
		{name: "iPhone 6.3 JPEG with uppercase .JPEG", path: "review.JPEG", data: func(t *testing.T) []byte { return encodeJPEG(t, 1206, 2622) }},
		{name: "iPad 13 PNG", path: "review.png", data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 2064, 2752) }},
		{name: "Mac PNG", path: "review.png", data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 2880, 1800) }},
		{name: "iPhone 3.5 without status bar", path: "review.png", data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 640, 920) }},
		{name: "iPad 9.7 without status bar", path: "review.png", data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 768, 1004) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, err := CheckReviewScreenshotImage(tt.path, bytes.NewReader(tt.data(t)))
			if err != nil {
				t.Fatalf("CheckReviewScreenshotImage() error = %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("CheckReviewScreenshotImage() warnings = %q, want none", warnings)
			}
		})
	}
}

func TestCheckReviewScreenshotImageRejectsUnsupportedDimensions(t *testing.T) {
	_, err := CheckReviewScreenshotImage("./paywall.png", bytes.NewReader(encodeOpaquePNG(t, 1179, 2560)))
	if err == nil {
		t.Fatal("expected unsupported dimensions error")
	}
	message := err.Error()
	for _, want := range []string{
		`review screenshot "./paywall.png" is 1179x2560 pixels`,
		"matches no App Store screenshot size",
		"nearest accepted size: 1179x2556",
		"accepted sizes:",
		"1290x2796",
		"2064x2752",
		"640x920",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
}

func TestCheckReviewScreenshotImageRejectsSquareImages(t *testing.T) {
	_, err := CheckReviewScreenshotImage("icon.png", bytes.NewReader(encodeOpaquePNG(t, 1024, 1024)))
	if err == nil {
		t.Fatal("expected unsupported dimensions error")
	}
	if !strings.Contains(err.Error(), "is 1024x1024 pixels") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckReviewScreenshotImageRejectsUnsupportedFormats(t *testing.T) {
	tests := []struct {
		name string
		path string
		data func(t *testing.T) []byte
		want []string
	}{
		{
			name: "GIF data",
			path: "review.gif",
			data: func(t *testing.T) []byte { return encodeGIF(t, 1290, 2796) },
			want: []string{`review screenshot "review.gif" is GIF data`, "PNG or JPEG"},
		},
		{
			name: "undecodable data",
			path: "review.webp",
			data: func(t *testing.T) []byte { return []byte("RIFF\x00\x00\x00\x00WEBPVP8 ") },
			want: []string{`review screenshot "review.webp" is not a PNG or JPEG image`, ".png, .jpg, or .jpeg"},
		},
		{
			name: "JPEG data with .png extension",
			path: "review.png",
			data: func(t *testing.T) []byte { return encodeJPEG(t, 1290, 2796) },
			want: []string{`review screenshot "review.png" is JPEG data but has a .png extension`, "rename it to review.jpg"},
		},
		{
			name: "PNG data without an accepted extension",
			path: "review.img",
			data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 1290, 2796) },
			want: []string{`review screenshot "review.img" has a .img extension`, ".png, .jpg, or .jpeg", "rename it to review.png"},
		},
		{
			name: "PNG data without an extension",
			path: "review",
			data: func(t *testing.T) []byte { return encodeOpaquePNG(t, 1290, 2796) },
			want: []string{`review screenshot "review" has no file extension`, "rename it to review.png"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CheckReviewScreenshotImage(tt.path, bytes.NewReader(tt.data(t)))
			if err == nil {
				t.Fatal("expected format error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

func encodePNGImage(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestCheckReviewScreenshotImageDoesNotWarnForOpaqueColorPNGs(t *testing.T) {
	opaqueRGB := image.NewRGBA(image.Rect(0, 0, 1290, 2796))
	for i := 3; i < len(opaqueRGB.Pix); i += 4 {
		opaqueRGB.Pix[i] = 0xff
	}
	opaqueRGB64 := image.NewRGBA64(image.Rect(0, 0, 640, 920))
	for i := 6; i < len(opaqueRGB64.Pix); i += 8 {
		opaqueRGB64.Pix[i], opaqueRGB64.Pix[i+1] = 0xff, 0xff
	}
	opaquePalette := image.NewPaletted(image.Rect(0, 0, 640, 920), color.Palette{color.Black, color.White})

	for name, img := range map[string]image.Image{
		"8-bit RGB":      opaqueRGB,
		"16-bit RGB":     opaqueRGB64,
		"opaque palette": opaquePalette,
	} {
		t.Run(name, func(t *testing.T) {
			warnings, err := CheckReviewScreenshotImage("review.png", bytes.NewReader(encodePNGImage(t, img)))
			if err != nil {
				t.Fatalf("CheckReviewScreenshotImage() error = %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %q, want none for an opaque PNG", warnings)
			}
		})
	}
}

func TestCheckReviewScreenshotImageWarnsAboutTransparentPalette(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 640, 920), color.Palette{color.Black, color.Transparent})
	warnings, err := CheckReviewScreenshotImage("review.png", bytes.NewReader(encodePNGImage(t, img)))
	if err != nil {
		t.Fatalf("CheckReviewScreenshotImage() error = %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "has an alpha channel or transparency") {
		t.Fatalf("warnings = %q, want one transparency warning", warnings)
	}
}

func TestCheckReviewScreenshotImageWarnsAboutAlphaChannel(t *testing.T) {
	warnings, err := CheckReviewScreenshotImage("review.png", bytes.NewReader(encodeTranslucentPNG(t, 1290, 2796)))
	if err != nil {
		t.Fatalf("CheckReviewScreenshotImage() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %q, want one alpha warning", warnings)
	}
	if !strings.Contains(warnings[0], `review screenshot "review.png" has an alpha channel`) {
		t.Fatalf("unexpected warning %q", warnings[0])
	}
}
