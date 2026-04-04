package utils

import (
	_ "embed"
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"

	"github.com/disintegration/imaging"
	"github.com/skip2/go-qrcode"
)

//go:embed retena-logo.png
var retenaLogoBytes []byte

// WriteQRWithLogo generates a QR code PNG at the given path with the Retena
// logo composited at the centre. Error correction level H is used so the logo
// can safely cover ~30 % of the code area without losing scannability.
func WriteQRWithLogo(content string, size int, path string) error {
	// 1. Generate raw QR bitmap (H = highest error correction).
	qr, err := qrcode.New(content, qrcode.Highest)
	if err != nil {
		return err
	}
	qr.DisableBorder = false

	qrImg := qr.Image(size)

	// 2. Decode the embedded logo.
	logoImg, err := png.Decode(bytes.NewReader(retenaLogoBytes))
	if err != nil {
		return err
	}

	// 3. Resize logo to 30 % of the QR size.
	logoSize := int(math.Round(float64(size) * 0.30))
	logoResized := imaging.Resize(logoImg, logoSize, logoSize, imaging.Lanczos)

	// 4. Draw white rounded-square background behind the logo.
	//    We expand by a small padding so the QR modules don't bleed through.
	padding := int(math.Round(float64(logoSize) * 0.10))
	bgSize := logoSize + padding*2

	bgImg := image.NewRGBA(image.Rect(0, 0, bgSize, bgSize))
	// Fill with white.
	for y := 0; y < bgSize; y++ {
		for x := 0; x < bgSize; x++ {
			bgImg.SetRGBA(x, y, colorWhite)
		}
	}
	// Apply corner radius: clear pixels outside the rounded corners.
	// radius ≈ 20 % of bgSize.
	r := float64(bgSize) * 0.20
	for y := 0; y < bgSize; y++ {
		for x := 0; x < bgSize; x++ {
			fx, fy := float64(x), float64(y)
			// Nearest corner centre
			ncx := r
			if fx > float64(bgSize)/2 {
				ncx = float64(bgSize) - r
			}
			ncy := r
			if fy > float64(bgSize)/2 {
				ncy = float64(bgSize) - r
			}
			// Only apply rounding when inside the corner region
			if fx < r || fx > float64(bgSize)-r || fy < r || fy > float64(bgSize)-r {
				dx := fx - ncx
				dy := fy - ncy
				if math.Sqrt(dx*dx+dy*dy) > r {
					bgImg.SetRGBA(x, y, colorTransparent)
				}
			}
		}
	}

	// 5. Composite: QR → white bg → logo.
	out := image.NewRGBA(qrImg.Bounds())
	draw.Draw(out, out.Bounds(), qrImg, image.Point{}, draw.Src)

	// Centre offsets for the bg block.
	bgOffX := (size - bgSize) / 2
	bgOffY := (size - bgSize) / 2
	bgRect := image.Rect(bgOffX, bgOffY, bgOffX+bgSize, bgOffY+bgSize)
	draw.Draw(out, bgRect, bgImg, image.Point{}, draw.Over)

	// Centre offsets for the logo.
	logoOffX := (size - logoSize) / 2
	logoOffY := (size - logoSize) / 2
	logoRect := image.Rect(logoOffX, logoOffY, logoOffX+logoSize, logoOffY+logoSize)
	draw.Draw(out, logoRect, logoResized, image.Point{}, draw.Over)

	// 6. Write PNG.
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}
