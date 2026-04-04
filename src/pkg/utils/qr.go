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

// WriteQRWithLogo generates a QR code PNG with the Retena logo composited at
// the centre. Error correction level H allows the logo to cover ~30 % of the
// code area without losing scannability.
// The logo asset (ic_launcher_512) already has built-in rounded-square edges,
// so no extra background drawing is needed.
func WriteQRWithLogo(content string, size int, path string) error {
	// 1. Generate QR bitmap at highest error correction.
	qr, err := qrcode.New(content, qrcode.Highest)
	if err != nil {
		return err
	}
	qrImg := qr.Image(size)

	// 2. Decode embedded logo.
	logoImg, err := png.Decode(bytes.NewReader(retenaLogoBytes))
	if err != nil {
		return err
	}

	// 3. Resize logo to 30 % of the QR size.
	logoSize := int(math.Round(float64(size) * 0.30))
	logoResized := imaging.Resize(logoImg, logoSize, logoSize, imaging.Lanczos)

	// 4. Composite: QR → logo (the logo asset already has its own rounded bg).
	out := image.NewRGBA(qrImg.Bounds())
	draw.Draw(out, out.Bounds(), qrImg, image.Point{}, draw.Src)

	offX := (size - logoSize) / 2
	offY := (size - logoSize) / 2
	logoRect := image.Rect(offX, offY, offX+logoSize, offY+logoSize)
	draw.Draw(out, logoRect, logoResized, image.Point{}, draw.Over)

	// 5. Write PNG.
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}
