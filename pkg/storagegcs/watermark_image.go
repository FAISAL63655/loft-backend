// Package storagegcs - Image-based watermark implementation
package storagegcs

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
)

// ImageWatermarkConfig holds configuration for image-based watermarking
type ImageWatermarkConfig struct {
	LogoPath   string  // Path to logo image file
	Opacity    int     // 0-100
	Position   string  // "center", "top-left", "top-right", "bottom-left", "bottom-right"
	Scale      float64 // Scale factor (e.g., 0.2 = 20% of image width)
	Background bool    // Add semi-transparent background
}

// DefaultImageWatermarkConfig returns default image watermark configuration
func DefaultImageWatermarkConfig() ImageWatermarkConfig {
	return ImageWatermarkConfig{
		LogoPath:   "assets/logo.png",
		Opacity:    80,
		Position:   "bottom-right", // أسفل اليمين
		Scale:      0.15,           // 15% من عرض الصورة
		Background: false,          // مفرغ بدون خلفية
	}
}

// ApplyImageWatermark applies a logo image as watermark
func (c *Client) ApplyImageWatermark(img image.Image, config ImageWatermarkConfig) (image.Image, error) {
	// Validate opacity
	if config.Opacity < 0 {
		config.Opacity = 0
	}
	if config.Opacity > 100 {
		config.Opacity = 100
	}
	if config.Scale <= 0 || config.Scale > 1 {
		config.Scale = 0.15 // Default 15%
	}

	// Load logo image
	logoImg, err := loadLogoImage(config.LogoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load logo: %w", err)
	}

	// Create a copy of the original image
	bounds := img.Bounds()
	watermarked := image.NewRGBA(bounds)
	draw.Draw(watermarked, bounds, img, bounds.Min, draw.Src)

	// Calculate logo size based on image width
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()
	
	logoBounds := logoImg.Bounds()
	logoWidth := logoBounds.Dx()
	logoHeight := logoBounds.Dy()
	
	// Calculate scaled logo dimensions
	targetLogoWidth := int(float64(imgWidth) * config.Scale)
	logoAspectRatio := float64(logoHeight) / float64(logoWidth)
	targetLogoHeight := int(float64(targetLogoWidth) * logoAspectRatio)
	
	// Ensure minimum size
	if targetLogoWidth < 50 {
		targetLogoWidth = 50
		targetLogoHeight = int(float64(targetLogoWidth) * logoAspectRatio)
	}

	// Scale logo using high-quality interpolation
	scaledLogo := image.NewRGBA(image.Rect(0, 0, targetLogoWidth, targetLogoHeight))
	xdraw.CatmullRom.Scale(scaledLogo, scaledLogo.Bounds(), logoImg, logoBounds, xdraw.Over, nil)

	// Calculate position
	margin := int(math.Max(float64(imgWidth), float64(imgHeight)) * 0.03) // 3% margin
	var startX, startY int

	switch config.Position {
	case "center":
		startX = (imgWidth - targetLogoWidth) / 2
		startY = (imgHeight - targetLogoHeight) / 2
	case "top-left":
		startX = margin
		startY = margin
	case "top-right":
		startX = imgWidth - targetLogoWidth - margin
		startY = margin
	case "bottom-left":
		startX = margin
		startY = imgHeight - targetLogoHeight - margin
	case "bottom-right":
		startX = imgWidth - targetLogoWidth - margin
		startY = imgHeight - targetLogoHeight - margin
	default:
		// Default to bottom-right
		startX = imgWidth - targetLogoWidth - margin
		startY = imgHeight - targetLogoHeight - margin
	}

	// Add background if enabled
	if config.Background {
		paddingX := int(float64(targetLogoWidth) * 0.1)
		paddingY := int(float64(targetLogoHeight) * 0.1)
		bgStartX := startX - paddingX
		bgStartY := startY - paddingY
		bgWidth := targetLogoWidth + (paddingX * 2)
		bgHeight := targetLogoHeight + (paddingY * 2)
		
		c.drawWatermarkBackground(watermarked, bgStartX, bgStartY, bgWidth, bgHeight, config.Opacity)
	}

	// Apply logo with opacity
	c.drawLogoWithOpacity(watermarked, scaledLogo, startX, startY, config.Opacity)

	return watermarked, nil
}

// loadLogoImage loads a logo image from file
func loadLogoImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open logo file: %w", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logo image: %w", err)
	}

	return img, nil
}

// drawLogoWithOpacity draws the logo on the destination image with specified opacity
func (c *Client) drawLogoWithOpacity(dst *image.RGBA, logo image.Image, x, y int, opacity int) {
	logoBounds := logo.Bounds()
	alpha := float64(opacity) / 100.0

	for dy := 0; dy < logoBounds.Dy(); dy++ {
		for dx := 0; dx < logoBounds.Dx(); dx++ {
			px := x + dx
			py := y + dy

			// Check bounds
			if px >= 0 && py >= 0 && px < dst.Bounds().Dx() && py < dst.Bounds().Dy() {
				logoColor := logo.At(dx+logoBounds.Min.X, dy+logoBounds.Min.Y)
				r, g, b, a := logoColor.RGBA()

				// Apply opacity to logo alpha channel
				newAlpha := float64(a>>8) * alpha / 255.0

				if newAlpha > 0 {
					// Get existing pixel
					existing := dst.RGBAAt(px, py)

					// Alpha blending
					outAlpha := newAlpha + float64(existing.A)*(1-newAlpha)/255.0
					
					var outR, outG, outB uint8
					if outAlpha > 0 {
						outR = uint8((float64(r>>8)*newAlpha + float64(existing.R)*float64(existing.A)*(1-newAlpha)/255.0) / outAlpha)
						outG = uint8((float64(g>>8)*newAlpha + float64(existing.G)*float64(existing.A)*(1-newAlpha)/255.0) / outAlpha)
						outB = uint8((float64(b>>8)*newAlpha + float64(existing.B)*float64(existing.A)*(1-newAlpha)/255.0) / outAlpha)
					}

					dst.SetRGBA(px, py, color.RGBA{
						R: outR,
						G: outG,
						B: outB,
						A: uint8(outAlpha * 255.0),
					})
				}
			}
		}
	}
}
