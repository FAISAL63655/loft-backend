// Package storagegcs tests
package storagegcs

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"
)

// Mock database client for testing
type mockDBClient struct {
	settings *MediaSettings
	error    error
}

func (m *mockDBClient) QueryRow(query string, args ...interface{}) *sql.Row {
	// This is a simplified mock - in real tests you'd use a proper mock library
	return nil
}

func (m *mockDBClient) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return nil, m.error
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Arabic text",
			input:    "لوفت الدغيري",
			expected: "lawft_aldghayray",
		},
		{
			name:     "English text",
			input:    "Hello World",
			expected: "hello_world",
		},
		{
			name:     "Mixed Arabic and English",
			input:    "Loft لوفت 2024",
			expected: "loft_lawft_2024",
		},
		{
			name:     "Special characters",
			input:    "File@Name&Test",
			expected: "file_at_name_and_test",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Numbers only",
			input:    "12345",
			expected: "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Generate(tt.input)
			if result != tt.expected {
				t.Errorf("Generate(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name     string
		slug     string
		expected bool
	}{
		{"Valid slug", "valid_file_name", true},
		{"Valid with numbers", "file_123", true},
		{"Valid with hyphens", "file-name", true},
		{"Empty string", "", false},
		{"Too long", strings.Repeat("a", 150), false},
		{"Invalid characters", "file@name", false},
		{"Starts with separator", "_filename", false},
		{"Ends with separator", "filename_", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateSlug(tt.slug)
			if result != tt.expected {
				t.Errorf("ValidateSlug(%v) = %v, want %v", tt.slug, result, tt.expected)
			}
		})
	}
}

func TestGenerateSecureFileName(t *testing.T) {
	originalName := "ملف الصورة.jpg"
	
	result, err := generateSecureFileName(originalName)
	if err != nil {
		t.Fatalf("generateSecureFileName failed: %v", err)
	}

	// Check that result contains expected elements
	if !strings.Contains(result, ".jpg") {
		t.Error("Result should contain original extension")
	}

	if !strings.Contains(result, "_") {
		t.Error("Result should contain separator")
	}

	// Check length is reasonable
	if len(result) < 10 || len(result) > 100 {
		t.Errorf("Result length %d is not reasonable", len(result))
	}
}

func TestLoadMediaSettingsFromDatabase(t *testing.T) {
	// Test with nil database
	settings, err := LoadMediaSettingsFromDatabase(nil)
	if err != nil {
		t.Errorf("LoadMediaSettingsFromDatabase(nil) should not return error, got: %v", err)
	}
	if settings == nil {
		t.Error("LoadMediaSettingsFromDatabase(nil) should return default settings")
	}

	// Test with mock database
	mockDB := &mockDBClient{
		settings: &MediaSettings{
			WatermarkEnabled:  true,
			WatermarkPosition: "center",
			WatermarkOpacity:  0.8,
			MaxFileSize:       5242880, // 5MB
		},
	}

	settings, err = LoadMediaSettingsFromDatabase(mockDB)
	if err != nil {
		t.Errorf("LoadMediaSettingsFromDatabase should not return error, got: %v", err)
	}
	if settings == nil {
		t.Error("LoadMediaSettingsFromDatabase should return settings")
	}
}

func TestValidateMediaSettings(t *testing.T) {
	tests := []struct {
		name        string
		settings    *MediaSettings
		expectError bool
	}{
		{
			name:        "Nil settings",
			settings:    nil,
			expectError: true,
		},
		{
			name: "Valid settings",
			settings: &MediaSettings{
				WatermarkEnabled:  true,
				WatermarkPosition: "center",
				WatermarkOpacity:  0.7,
				MaxFileSize:       10485760,
				AllowedTypes:      []string{"image/jpeg"},
				ThumbnailSizes:    []int{200, 400},
			},
			expectError: false,
		},
		{
			name: "Invalid file size",
			settings: &MediaSettings{
				MaxFileSize:       -1,
				WatermarkOpacity:  0.7,
				WatermarkPosition: "center",
				AllowedTypes:      []string{"image/jpeg"},
			},
			expectError: true,
		},
		{
			name: "Invalid opacity",
			settings: &MediaSettings{
				MaxFileSize:       10485760,
				WatermarkOpacity:  1.5,
				WatermarkPosition: "center",
				AllowedTypes:      []string{"image/jpeg"},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMediaSettings(tt.settings)
			if (err != nil) != tt.expectError {
				t.Errorf("ValidateMediaSettings() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestGetMediaKind(t *testing.T) {
	client := &Client{}
	
	tests := []struct {
		fileName string
		expected MediaKind
	}{
		{"image.jpg", MediaKindImage},
		{"image.jpeg", MediaKindImage},
		{"image.png", MediaKindImage},
		{"image.webp", MediaKindImage},
		{"video.mp4", MediaKindVideo},
		{"document.pdf", MediaKindFile},
		{"unknown.xyz", MediaKindFile},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			result := client.getMediaKind(tt.fileName)
			if result != tt.expected {
				t.Errorf("getMediaKind(%v) = %v, want %v", tt.fileName, result, tt.expected)
			}
		})
	}
}

func TestGetContentType(t *testing.T) {
	client := &Client{}
	
	tests := []struct {
		fileName string
		expected string
	}{
		{"file.jpg", "image/jpeg"},
		{"file.jpeg", "image/jpeg"},
		{"file.png", "image/png"},
		{"file.webp", "image/webp"},
		{"file.mp4", "video/mp4"},
		{"file.pdf", "application/pdf"},
		{"file.unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			result := client.getContentType(tt.fileName)
			if result != tt.expected {
				t.Errorf("getContentType(%v) = %v, want %v", tt.fileName, result, tt.expected)
			}
		})
	}
}

func TestGeneratePath(t *testing.T) {
	client := &Client{}
	fileName := "test_file.jpg"
	
	path := client.generatePath(fileName)
	
	// Check path structure
	if !strings.HasPrefix(path, "media/") {
		t.Error("Path should start with media/")
	}
	
	if !strings.HasSuffix(path, "/test_file.jpg") {
		t.Error("Path should end with filename")
	}
	
	// Check that path contains timestamp structure (YYYY/MM/DD/HH-MM-SS)
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		t.Error("Path should contain timestamp structure")
	}
}

func TestCreateUploadConfigFromSettings(t *testing.T) {
	settings := MediaSettings{
		WatermarkEnabled:  true,
		WatermarkPosition: "top-left",
		WatermarkOpacity:  0.8,
		ThumbnailsEnabled: true,
		ThumbnailSizes:    []int{150, 300},
	}

	config := CreateUploadConfigFromSettings(settings)

	if !config.ApplyWatermark {
		t.Error("ApplyWatermark should be true")
	}

	if config.WatermarkPosition != "top-left" {
		t.Errorf("WatermarkPosition = %v, want top-left", config.WatermarkPosition)
	}

	if config.WatermarkOpacity != 80 {
		t.Errorf("WatermarkOpacity = %v, want 80", config.WatermarkOpacity)
	}

	if !config.GenerateThumbnails {
		t.Error("GenerateThumbnails should be true")
	}

	if len(config.ThumbnailSizes) != 2 || config.ThumbnailSizes[0] != 150 {
		t.Error("ThumbnailSizes not set correctly")
	}
}

func TestApplyAdvancedWatermark(t *testing.T) {
	client := &Client{}
	
	// Create a simple test image
	testImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	
	// Fill with white
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			testImg.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	config := WatermarkConfig{
		Text:       "Test",
		Opacity:    70,
		Position:   "center",
		FontSize:   16,
		Color:      color.RGBA{R: 0, G: 0, B: 0, A: 255},
		Background: true,
	}

	result, err := client.ApplyAdvancedWatermark(testImg, config)
	if err != nil {
		t.Fatalf("ApplyAdvancedWatermark failed: %v", err)
	}

	if result == nil {
		t.Error("Result should not be nil")
	}

	// Check that the result has the same dimensions
	if result.Bounds() != testImg.Bounds() {
		t.Error("Result dimensions should match original")
	}
}

func TestIsValidMimeType(t *testing.T) {
	client := &Client{}
	
	tests := []struct {
		detected string
		expected string
		valid    bool
	}{
		{"image/jpeg", "image/jpeg", true},
		{"image/jpg", "image/jpeg", true},
		{"image/png", "image/png", true},
		{"image/webp", "image/webp", true},
		{"text/plain", "image/jpeg", false},
		{"application/pdf", "application/pdf", true},
	}

	for _, tt := range tests {
		t.Run(tt.detected, func(t *testing.T) {
			result := client.isValidMimeType(tt.detected, tt.expected)
			if result != tt.valid {
				t.Errorf("isValidMimeType(%v, %v) = %v, want %v", 
					tt.detected, tt.expected, result, tt.valid)
			}
		})
	}
}

func TestValidateImageContent(t *testing.T) {
	client := &Client{}
	
	// Create a valid test image
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	// Test valid image
	err = client.validateImageContent(buf.Bytes())
	if err != nil {
		t.Errorf("validateImageContent should pass for valid image: %v", err)
	}

	// Test invalid image data
	err = client.validateImageContent([]byte("not an image"))
	if err == nil {
		t.Error("validateImageContent should fail for invalid image data")
	}

	// Test empty data
	err = client.validateImageContent([]byte{})
	if err == nil {
		t.Error("validateImageContent should fail for empty data")
	}
}

func TestGenerateThumbnail(t *testing.T) {
	client := &Client{}
	
	// Create test image
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	
	// Generate thumbnail
	thumb := client.generateThumbnail(img, 100)
	
	thumbBounds := thumb.Bounds()
	
	// Check that width is 100 (or less if original was smaller)
	if thumbBounds.Dx() > 100 {
		t.Errorf("Thumbnail width %d should be <= 100", thumbBounds.Dx())
	}
	
	// Check aspect ratio preservation
	originalRatio := float64(200) / float64(100)
	thumbRatio := float64(thumbBounds.Dx()) / float64(thumbBounds.Dy())
	
	if abs(originalRatio-thumbRatio) > 0.1 {
		t.Errorf("Aspect ratio not preserved: original=%.2f, thumb=%.2f", 
			originalRatio, thumbRatio)
	}
}

func TestEncodeImage(t *testing.T) {
	client := &Client{}
	
	// Create test image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	
	// Test JPEG encoding
	jpegData, err := client.encodeImage(img, "jpeg")
	if err != nil {
		t.Errorf("JPEG encoding failed: %v", err)
	}
	if len(jpegData) == 0 {
		t.Error("JPEG data should not be empty")
	}
	
	// Test PNG encoding
	pngData, err := client.encodeImage(img, "png")
	if err != nil {
		t.Errorf("PNG encoding failed: %v", err)
	}
	if len(pngData) == 0 {
		t.Error("PNG data should not be empty")
	}
	
	// Test unknown format (should default to JPEG)
	unknownData, err := client.encodeImage(img, "unknown")
	if err != nil {
		t.Errorf("Unknown format encoding failed: %v", err)
	}
	if len(unknownData) == 0 {
		t.Error("Unknown format data should not be empty")
	}
}

func TestGetPublicURL(t *testing.T) {
	tests := []struct {
		name     string
		isPublic bool
		gcsPath  string
		expected string
	}{
		{
			name:     "Public bucket",
			isPublic: true,
			gcsPath:  "media/test.jpg",
			expected: "https://storage.googleapis.com/test-bucket/media/test.jpg",
		},
		{
			name:     "Private bucket",
			isPublic: false,
			gcsPath:  "media/test.jpg",
			expected: "gs://test-bucket/media/test.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				bucketName: "test-bucket",
				isPublic:   tt.isPublic,
			}
			
			result := client.GetPublicURL(tt.gcsPath)
			if result != tt.expected {
				t.Errorf("GetPublicURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper function for floating point comparison
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Benchmark tests
func BenchmarkGenerate(b *testing.B) {
	input := "ملف صورة مع اسم طويل نسبياً"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Generate(input)
	}
}

func BenchmarkGenerateSecureFileName(b *testing.B) {
	input := "ملف الصورة الجديد.jpg"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generateSecureFileName(input)
	}
}

func BenchmarkApplyAdvancedWatermark(b *testing.B) {
	client := &Client{}
	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	config := DefaultWatermarkConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ApplyAdvancedWatermark(img, config)
	}
}
