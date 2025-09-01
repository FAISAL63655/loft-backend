// Package storagegcs - Advanced watermark implementation with Arabic text support
package storagegcs

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// WatermarkConfig holds configuration for watermarking
type WatermarkConfig struct {
	Text       string
	Opacity    int    // 0-100
	Position   string // "center", "top-left", "top-right", "bottom-left", "bottom-right"
	FontSize   int    // Font size in pixels
	Color      color.RGBA
	Background bool   // Add background to text for better visibility
}

// DefaultWatermarkConfig returns default watermark configuration
func DefaultWatermarkConfig() WatermarkConfig {
	return WatermarkConfig{
		Text:       "لوفت الدغيري", // Loft Dughairi in Arabic
		Opacity:    70,
		Position:   "bottom-right",
		FontSize:   16,
		Color:      color.RGBA{R: 255, G: 255, B: 255, A: 255}, // White
		Background: true,
	}
}

// ApplyAdvancedWatermark applies an advanced watermark with better text rendering
func (c *Client) ApplyAdvancedWatermark(img image.Image, config WatermarkConfig) (image.Image, error) {
	bounds := img.Bounds()
	watermarked := image.NewRGBA(bounds)

	// Copy original image to new image
	draw.Draw(watermarked, bounds, img, bounds.Min, draw.Src)

	// Calculate watermark dimensions based on text length and font size
	textWidth := len([]rune(config.Text)) * config.FontSize * 3 / 4 // Approximate character width
	textHeight := config.FontSize

	// Add padding for background
	paddingX := config.FontSize / 4
	paddingY := config.FontSize / 6
	watermarkWidth := textWidth + (paddingX * 2)
	watermarkHeight := textHeight + (paddingY * 2)

	// Calculate position
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()
	startX, startY := c.calculateWatermarkPosition(
		imgWidth, imgHeight,
		watermarkWidth, watermarkHeight,
		config.Position,
	)

	// Create watermark with background if enabled
	if config.Background {
		c.drawWatermarkBackground(watermarked, startX, startY, watermarkWidth, watermarkHeight, config.Opacity)
	}

	// Draw text watermark
	c.drawTextWatermark(watermarked, config.Text, startX+paddingX, startY+paddingY, config)

	return watermarked, nil
}

// calculateWatermarkPosition calculates the position for the watermark
func (c *Client) calculateWatermarkPosition(imgWidth, imgHeight, watermarkWidth, watermarkHeight int, position string) (int, int) {
	margin := int(math.Max(float64(imgWidth), float64(imgHeight)) * 0.05) // 5% margin

	switch position {
	case "center":
		return (imgWidth - watermarkWidth) / 2, (imgHeight - watermarkHeight) / 2
	case "top-left":
		return margin, margin
	case "top-right":
		return imgWidth - watermarkWidth - margin, margin
	case "bottom-left":
		return margin, imgHeight - watermarkHeight - margin
	case "bottom-right":
		return imgWidth - watermarkWidth - margin, imgHeight - watermarkHeight - margin
	default:
		// Default to bottom-right
		return imgWidth - watermarkWidth - margin, imgHeight - watermarkHeight - margin
	}
}

// drawWatermarkBackground draws a semi-transparent background for the watermark
func (c *Client) drawWatermarkBackground(img *image.RGBA, x, y, width, height, opacity int) {
	// Background color (dark semi-transparent)
	bgColor := color.RGBA{R: 0, G: 0, B: 0, A: uint8(opacity * 128 / 100)} // Darker background

	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			px := x + dx
			py := y + dy

			if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
				// Blend background with existing pixel
				existing := img.RGBAAt(px, py)
				blended := c.blendColors(existing, bgColor)
				img.Set(px, py, blended)
			}
		}
	}
}

// drawTextWatermark draws the text watermark using a simple bitmap font approach
func (c *Client) drawTextWatermark(img *image.RGBA, text string, x, y int, config WatermarkConfig) {
	// Convert opacity to alpha
	alpha := uint8(config.Opacity * 255 / 100)
	textColor := color.RGBA{
		R: config.Color.R,
		G: config.Color.G,
		B: config.Color.B,
		A: alpha,
	}

	// Simple character rendering - each character is rendered as a rectangular pattern
	charWidth := config.FontSize * 3 / 4
	charHeight := config.FontSize

	runes := []rune(text)
	for i, r := range runes {
		charX := x + (i * charWidth)
		charY := y

		// Render character using simple patterns
		c.renderCharacter(img, r, charX, charY, charWidth, charHeight, textColor)
	}
}

// renderCharacter renders a single character using simple geometric patterns
func (c *Client) renderCharacter(img *image.RGBA, char rune, x, y, width, height int, color color.RGBA) {
	// For Arabic characters, use geometric approximations
	// This is a simplified implementation - a full implementation would use proper font rendering
	
	switch char {
	case 'ل': // Arabic Lam
		c.drawVerticalLine(img, x+width/4, y, height, color)
		c.drawHorizontalLine(img, x, y+height-2, width/2, color)
	case 'و': // Arabic Waw
		c.drawCircle(img, x+width/2, y+height/2, width/4, color)
	case 'ف': // Arabic Fa
		c.drawCircle(img, x+width/2, y+height/3, width/6, color)
		c.drawHorizontalLine(img, x, y+height*2/3, width, color)
	case 'ت': // Arabic Ta
		c.drawHorizontalLine(img, x, y+height/2, width, color)
		c.drawDot(img, x+width/4, y+height/4, color)
		c.drawDot(img, x+width*3/4, y+height/4, color)
	case 'ا': // Arabic Alif
		c.drawVerticalLine(img, x+width/2, y, height, color)
	case 'د': // Arabic Dal
		c.drawHorizontalLine(img, x, y+height-2, width, color)
		c.drawVerticalLine(img, x+width-2, y+height/2, height/2, color)
	case 'غ': // Arabic Ghain
		c.drawCircle(img, x+width/2, y+height/2, width/3, color)
		c.drawDot(img, x+width/2, y+height/4, color)
	case 'ي': // Arabic Ya
		c.drawHorizontalLine(img, x, y+height-2, width, color)
		c.drawDot(img, x+width/3, y+height*3/4, color)
		c.drawDot(img, x+width*2/3, y+height*3/4, color)
	case 'ر': // Arabic Ra
		c.drawHorizontalLine(img, x, y+height-2, width/2, color)
		c.drawVerticalLine(img, x+width/2, y+height/2, height/2, color)
	case 'ح': // Arabic Ha
		c.drawHorizontalLine(img, x, y+height/2, width, color)
		c.drawVerticalLine(img, x, y+height/2, height/2, color)
		c.drawVerticalLine(img, x+width-2, y+height/2, height/2, color)
	default:
		// For unknown characters or Latin, draw a simple rectangle
		c.drawRectangle(img, x, y, width, height, color)
	}
}

// Helper drawing functions
func (c *Client) drawVerticalLine(img *image.RGBA, x, y, length int, color color.RGBA) {
	for i := 0; i < length && y+i < img.Bounds().Dy(); i++ {
		if x >= 0 && x < img.Bounds().Dx() && y+i >= 0 {
			existing := img.RGBAAt(x, y+i)
			blended := c.blendColors(existing, color)
			img.Set(x, y+i, blended)
		}
	}
}

func (c *Client) drawHorizontalLine(img *image.RGBA, x, y, length int, color color.RGBA) {
	for i := 0; i < length && x+i < img.Bounds().Dx(); i++ {
		if x+i >= 0 && y >= 0 && y < img.Bounds().Dy() {
			existing := img.RGBAAt(x+i, y)
			blended := c.blendColors(existing, color)
			img.Set(x+i, y, blended)
		}
	}
}

func (c *Client) drawCircle(img *image.RGBA, centerX, centerY, radius int, color color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				px := centerX + x
				py := centerY + y
				if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
					existing := img.RGBAAt(px, py)
					blended := c.blendColors(existing, color)
					img.Set(px, py, blended)
				}
			}
		}
	}
}

func (c *Client) drawDot(img *image.RGBA, x, y int, color color.RGBA) {
	c.drawCircle(img, x, y, 2, color)
}

func (c *Client) drawRectangle(img *image.RGBA, x, y, width, height int, color color.RGBA) {
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			px := x + dx
			py := y + dy
			if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
				existing := img.RGBAAt(px, py)
				blended := c.blendColors(existing, color)
				img.Set(px, py, blended)
			}
		}
	}
}

// blendColors blends two colors using alpha blending
func (c *Client) blendColors(existing, overlay color.RGBA) color.RGBA {
	alpha := float64(overlay.A) / 255.0
	invAlpha := 1.0 - alpha

	return color.RGBA{
		R: uint8(float64(existing.R)*invAlpha + float64(overlay.R)*alpha),
		G: uint8(float64(existing.G)*invAlpha + float64(overlay.G)*alpha),
		B: uint8(float64(existing.B)*invAlpha + float64(overlay.B)*alpha),
		A: existing.A,
	}
}
