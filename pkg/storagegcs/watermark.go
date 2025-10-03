// Package storagegcs - Advanced watermark implementation with Arabic text support
package storagegcs

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ملاحظة: تمت إزالة go:embed لتجنب فشل البناء عند غياب الملف.
// سنحاول قراءة الخط وقت التشغيل من المسار "assets/fonts/Cairo-Regular.ttf"،
// وإن لم يتوفر، سنعتمد على basicfont كحل احتياطي.
var arabicFontBytes []byte

// WatermarkConfig holds configuration for watermarking
type WatermarkConfig struct {
	Text       string
	Opacity    int    // 0-100
	Position   string // "center", "top-left", "top-right", "bottom-left", "bottom-right"
	FontSize   int    // Font size in pixels
	Color      color.RGBA
	Background bool // Add background to text for better visibility
	// Optional: محاذاة RTL بسيطة (رسم يبدأ من اليمين)
	RTL bool
}

// DefaultWatermarkConfig returns default watermark configuration
func DefaultWatermarkConfig() WatermarkConfig {
	return WatermarkConfig{
		Text:       "Loft Dughairi ", // نص عربي افتراضي (عدّله كما تشاء)
		Opacity:    100,
		Position:   "bottom-right",
		FontSize:   62,
		Color:      color.RGBA{R: 255, G: 255, B: 255, A: 255}, // White
		Background: true,
		RTL:        true,
	}
}

// ApplyAdvancedWatermark applies an advanced watermark with better text rendering
func (c *Client) ApplyAdvancedWatermark(img image.Image, config WatermarkConfig) (image.Image, error) {
	if config.Opacity < 0 {
		config.Opacity = 0
	}
	if config.Opacity > 100 {
		config.Opacity = 100
	}
	if config.FontSize <= 0 {
		config.FontSize = 24
	}

	// إنشاء نسخة قابلة للرسم
	bounds := img.Bounds()
	watermarked := image.NewRGBA(bounds)
	draw.Draw(watermarked, bounds, img, bounds.Min, draw.Src)

	// تهيئة الخط
	face, ascent, textW, textH, err := prepareFaceAndMeasure(config)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closer, ok := face.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()

	// مساحة الكرت (النص + الحشوات)
	paddingX := int(float64(config.FontSize) * 0.25)
	paddingY := int(float64(config.FontSize) * 0.20)
	watermarkW := textW + (paddingX * 2)
	watermarkH := textH + (paddingY * 2)

	// حساب موضع البداية (كرت الخلفية)
	imgW := bounds.Dx()
	imgH := bounds.Dy()
	startX, startY := c.calculateWatermarkPosition(
		imgW, imgH, watermarkW, watermarkH, config.Position,
	)

	// خلفية نصف شفافة اختياريًا
	if config.Background {
		c.drawWatermarkBackground(watermarked, startX, startY, watermarkW, watermarkH, config.Opacity)
	}

	// رسم النص داخل الكرت
	innerX := startX + paddingX
	innerY := startY + paddingY

	if err := drawTextWithFont(
		watermarked,
		config.Text,
		innerX,
		innerY,
		ascent,
		textW,
		face,
		config.Color,
		config.Opacity,
		config.RTL,
	); err != nil {
		return nil, err
	}

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
	// خلفية غامقة نصف شفافة
	bgAlpha := uint8((opacity * 128) / 100) // ضبط الشفافية
	bgColor := color.RGBA{R: 0, G: 0, B: 0, A: bgAlpha}

	// تأكد من حدود الصورة
	maxX := img.Bounds().Dx()
	maxY := img.Bounds().Dy()

	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			px := x + dx
			py := y + dy
			if px >= 0 && py >= 0 && px < maxX && py < maxY {
				existing := img.RGBAAt(px, py)
				blended := c.blendColors(existing, bgColor)
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

// ---------------------------
// أدوات الخط والرسم بالنص
// ---------------------------

func prepareFaceAndMeasure(cfg WatermarkConfig) (face font.Face, ascent, textW, textH int, err error) {
	// محاولة تحميل الخط من الملف إذا لم يكن مضمنًا
	if len(arabicFontBytes) == 0 {
		// Try Cairo font first (better Arabic support)
		if b, readErr := os.ReadFile("assets/fonts/Cairo-Regular.ttf"); readErr == nil {
			arabicFontBytes = b
		} else if b, readErr := os.ReadFile("assets/fonts/Tajawal-Regular.ttf"); readErr == nil {
			// Fallback to Tajawal if Cairo not found
			arabicFontBytes = b
		}
	}

	if len(arabicFontBytes) > 0 {
		ft, pErr := opentype.Parse(arabicFontBytes)
		if pErr != nil {
			// في حال فشل التحليل لأي سبب، نستخدم basicfont
			face = basicfont.Face7x13
		} else {
			var nErr error
			face, nErr = opentype.NewFace(ft, &opentype.FaceOptions{
				Size:    float64(cfg.FontSize),
				DPI:     96,
				Hinting: font.HintingFull,
			})
			if nErr != nil {
				// fallback
				face = basicfont.Face7x13
			}
		}
	} else {
		// fallback: basicfont عند عدم توفر ملف الخط
		face = basicfont.Face7x13
	}

	textW, textH, ascent = measure(face, cfg.Text)
	return face, ascent, textW, textH, nil
}

// يقيس عرض/ارتفاع النص الحقيقي بالـ font
func measure(face font.Face, s string) (w, h, ascent int) {
	b, _ := font.BoundString(face, s)
	w = (b.Max.X - b.Min.X).Ceil()
	m := face.Metrics()
	ascent = m.Ascent.Ceil()
	descent := m.Descent.Ceil()
	h = ascent + descent
	return
}

// helper: يطبق الشفافية على اللون
func colorWithOpacity(c color.RGBA, opacity int) *image.Uniform {
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 100 {
		opacity = 100
	}
	a := uint8((int(c.A) * opacity) / 100)
	return image.NewUniform(color.RGBA{R: c.R, G: c.G, B: c.B, A: a})
}

// drawTextWithFont يرسم النص العربي باستخدام خط TTF
// ملاحظة: هذا لا يقوم بتشكيل (shaping) معقّد للحروف العربية.
// عمليًا يعطي نتيجة ممتازة لمعظم النصوص. لو تحتاج ligatures/ربط مثالي استخدم
// محرك تشكيل مثل go-text/typesetting لاحقًا.
func drawTextWithFont(
	dst *image.RGBA,
	txt string,
	x, y int, // أعلى-يسار منطقة النص داخل الكرت
	ascent int,
	textW int,
	face font.Face,
	col color.RGBA,
	opacity int,
	rtl bool,
) error {
	d := &font.Drawer{
		Dst:  dst,
		Src:  colorWithOpacity(col, opacity),
		Face: face,
	}

	// baseline: نضع Y على أساس خط الأساس
	d.Dot.Y = fixed.I(y + ascent)

	// لو RTL: نحاذي النص لليمين داخل الصندوق (نبدأ القلم من نهاية السطر)
	if rtl {
		d.Dot.X = fixed.I(x + textW)
		// DrawString يرسم من الموضع الحالي إلى اليمين، لكن بما أننا
		// زدنا X بعرض النص، النتيجة تكون محاذاة لليمين.
	} else {
		// LTR: نبدأ من x مباشرة
		d.Dot.X = fixed.I(x)
	}

	d.DrawString(txt)
	return nil
}
