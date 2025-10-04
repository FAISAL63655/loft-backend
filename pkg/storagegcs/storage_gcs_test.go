package storagegcs

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestGetMediaKind(t *testing.T) {
	client := &Client{}

	tests := []struct {
		fileName string
		expected MediaKind
	}{
		{"image.jpg", MediaKindImage},
		{"image.jpeg", MediaKindImage},
		{"image.png", MediaKindImage},
		{"video.mp4", MediaKindVideo},
		{"document.pdf", MediaKindFile},
		{"unknown.txt", MediaKindFile},
	}

	for _, test := range tests {
		t.Run(test.fileName, func(t *testing.T) {
			result := client.getMediaKind(test.fileName)
			if result != test.expected {
				t.Errorf("getMediaKind(%s) = %v, want %v", test.fileName, result, test.expected)
			}
		})
	}
}

func TestGeneratePath(t *testing.T) {
	client := &Client{}

	fileName := "test-image.jpg"
	path := client.generatePath(fileName)

	// Check that path follows expected format: media/YYYY/MM/DD/HH-MM-SS/test-image.jpg
	if !strings.HasPrefix(path, "media/") {
		t.Errorf("generatePath should start with 'media/', got: %s", path)
	}

	if !strings.HasSuffix(path, "/test-image.jpg") {
		t.Errorf("generatePath should end with '/test-image.jpg', got: %s", path)
	}
}

func TestGetContentType(t *testing.T) {
	client := &Client{}

	tests := []struct {
		fileName string
		expected string
	}{
		{"image.jpg", "image/jpeg"},
		{"image.jpeg", "image/jpeg"},
		{"image.png", "image/png"},
		{"video.mp4", "video/mp4"},
		{"document.pdf", "application/pdf"},
		{"unknown.txt", "application/octet-stream"},
	}

	for _, test := range tests {
		t.Run(test.fileName, func(t *testing.T) {
			result := client.getContentType(test.fileName)
			if result != test.expected {
				t.Errorf("getContentType(%s) = %s, want %s", test.fileName, result, test.expected)
			}
		})
	}
}

func TestGenerateThumbnail(t *testing.T) {
	client := &Client{}

	// Create a simple test image (100x100 red square)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, red)
		}
	}

	tests := []struct {
		name         string
		maxWidth     int
		expectResize bool
	}{
		{"Resize needed", 50, true},
		{"No resize needed", 150, false},
		{"Same size", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.generateThumbnail(img, tt.maxWidth)

			bounds := result.Bounds()
			width := bounds.Dx()

			if tt.expectResize {
				if width != tt.maxWidth {
					t.Errorf("Expected width %d, got %d", tt.maxWidth, width)
				}
			} else {
				if width != 100 { // Original width
					t.Errorf("Expected original width 100, got %d", width)
				}
			}
		})
	}
}

func TestUploadConfig(t *testing.T) {
	config := UploadConfig{
		GenerateThumbnails: true,
		ApplyWatermark:     true,
		WatermarkOpacity:   30,
		WatermarkPosition:  "center",
		ThumbnailSizes:     []int{200, 400},
	}

	if !config.GenerateThumbnails {
		t.Error("GenerateThumbnails should be true")
	}

	if !config.ApplyWatermark {
		t.Error("ApplyWatermark should be true")
	}

	if config.WatermarkOpacity != 30 {
		t.Errorf("WatermarkOpacity = %d, want 30", config.WatermarkOpacity)
	}

	if config.WatermarkPosition != "center" {
		t.Errorf("WatermarkPosition = %s, want center", config.WatermarkPosition)
	}

	if len(config.ThumbnailSizes) != 2 {
		t.Errorf("ThumbnailSizes length = %d, want 2", len(config.ThumbnailSizes))
	}
}

// TestNewClient tests client creation (mocked)
func TestNewClient(t *testing.T) {
	// This test would require actual GCS setup, so we skip it in unit tests
	t.Skip("Skipping GCS client creation test - requires GCS credentials")
}

func TestGetPublicURL(t *testing.T) {
	client := &Client{bucketName: "test-bucket", isPublic: true}

	gcsPath := "media/2025/01/01/12-00-00/test.jpg"
	expected := "https://storage.googleapis.com/test-bucket/media/2025/01/01/12-00-00/test.jpg"

	result := client.GetPublicURL(gcsPath)
	if result != expected {
		t.Errorf("GetPublicURL = %s, want %s", result, expected)
	}
}

func TestApplyWatermark(t *testing.T) {
	client := &Client{}

	// Create a simple test image (100x100 blue square)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	blue := color.RGBA{0, 0, 255, 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, blue)
		}
	}

	tests := []struct {
		name     string
		opacity  int
		position string
	}{
		{"Bottom right watermark", 50, "bottom-right"},
		{"Center watermark", 30, "center"},
		{"Top left watermark", 70, "top-left"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.applyWatermark(img, tt.opacity, tt.position)
			if err != nil {
				t.Errorf("applyWatermark failed: %v", err)
				return
			}

			// Check that result is not nil and has same dimensions
			if result == nil {
				t.Error("applyWatermark returned nil image")
				return
			}

			bounds := result.Bounds()
			if bounds.Dx() != 100 || bounds.Dy() != 100 {
				t.Errorf("Expected dimensions 100x100, got %dx%d", bounds.Dx(), bounds.Dy())
			}
		})
	}
}

func TestCreateUploadConfigFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings MediaSettings
		expected UploadConfig
	}{
		{
			name: "Default settings",
			settings: MediaSettings{
				WatermarkEnabled:  true,
				WatermarkPosition: "bottom-right",
				WatermarkOpacity:  0.7,
				ThumbnailsEnabled: true,
				ThumbnailSizes:    []int{200, 400},
			},
			expected: UploadConfig{
				GenerateThumbnails: true,
				ApplyWatermark:     true,
				WatermarkOpacity:   70,
				WatermarkPosition:  "bottom-right",
				ThumbnailSizes:     []int{200, 400},
			},
		},
		{
			name: "Disabled features",
			settings: MediaSettings{
				WatermarkEnabled:  false,
				WatermarkPosition: "center",
				WatermarkOpacity:  0.5,
				ThumbnailsEnabled: false,
				ThumbnailSizes:    []int{},
			},
			expected: UploadConfig{
				GenerateThumbnails: false,
				ApplyWatermark:     false,
				WatermarkOpacity:   50,
				WatermarkPosition:  "center",
				ThumbnailSizes:     []int{200, 400}, // Default sizes
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateUploadConfigFromSettings(tt.settings)

			if result.GenerateThumbnails != tt.expected.GenerateThumbnails {
				t.Errorf("GenerateThumbnails = %v, want %v", result.GenerateThumbnails, tt.expected.GenerateThumbnails)
			}
			if result.ApplyWatermark != tt.expected.ApplyWatermark {
				t.Errorf("ApplyWatermark = %v, want %v", result.ApplyWatermark, tt.expected.ApplyWatermark)
			}
			if result.WatermarkOpacity != tt.expected.WatermarkOpacity {
				t.Errorf("WatermarkOpacity = %v, want %v", result.WatermarkOpacity, tt.expected.WatermarkOpacity)
			}
		})
	}
}

func TestLoadMediaSettingsFromDB(t *testing.T) {
	settings, err := LoadMediaSettingsFromDB(nil)
	if err != nil {
		t.Errorf("LoadMediaSettingsFromDB failed: %v", err)
		return
	}

	if settings == nil {
		t.Error("LoadMediaSettingsFromDB returned nil settings")
		return
	}

	// Check default values match system_settings
	if !settings.WatermarkEnabled {
		t.Error("Expected WatermarkEnabled to be true by default")
	}
	if settings.WatermarkPosition != "bottom-right" {
		t.Errorf("Expected WatermarkPosition 'bottom-right', got '%s'", settings.WatermarkPosition)
	}
	if settings.WatermarkOpacity != 0.7 {
		t.Errorf("Expected WatermarkOpacity 0.7, got %f", settings.WatermarkOpacity)
	}
}
