package storagegcs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	xdraw "golang.org/x/image/draw" // High-quality scaling
	"google.golang.org/api/option"
)

// Client wraps Google Cloud Storage operations
type Client struct {
	client     *storage.Client
	bucketName string
	projectID  string
	isPublic   bool
}

// Config holds configuration for GCS client
type Config struct {
	ProjectID      string
	BucketName     string
	CredentialsKey string // JSON key as string
	IsPublic       bool   // Whether the bucket is public (affects URL generation)
}

// MediaKind represents the type of media
type MediaKind string

const (
	MediaKindImage MediaKind = "image"
	MediaKindVideo MediaKind = "video"
	MediaKindFile  MediaKind = "file"
)

// UploadConfig holds configuration for upload operations
type UploadConfig struct {
	GenerateThumbnails bool
	ApplyWatermark     bool
	WatermarkOpacity   int
	WatermarkPosition  string // "center", "top-left", "top-right", "bottom-left", "bottom-right"
	ThumbnailSizes     []int  // widths in pixels
}

// MediaSettings represents media processing configuration from system settings
type MediaSettings struct {
	WatermarkEnabled  bool
	WatermarkPosition string
	WatermarkOpacity  float64
	ThumbnailsEnabled bool
	ThumbnailSizes    []int
	MaxFileSize       int64
	AllowedTypes      []string
}

// UploadResult contains information about uploaded files
type UploadResult struct {
	GCSPath          string
	ThumbPath        string
	WatermarkApplied bool
	Kind             MediaKind
	Size             int64
}

// NewClient creates a new GCS client
func NewClient(ctx context.Context, config Config) (*Client, error) {
	var opts []option.ClientOption
	if config.CredentialsKey != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(config.CredentialsKey)))
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return &Client{
		client:     client,
		bucketName: config.BucketName,
		projectID:  config.ProjectID,
		isPublic:   config.IsPublic,
	}, nil
}

// Upload uploads a file to GCS with optional thumbnail generation and watermarking
func (c *Client) Upload(ctx context.Context, reader io.Reader, fileName string, config UploadConfig) (*UploadResult, error) {
	// Generate secure file name first
	secureFileName, err := generateSecureFileName(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure filename: %w", err)
	}

	// Determine media kind from file extension
	kind := c.getMediaKind(secureFileName)

	// Generate unique file path with secure name
	gcsPath := c.generatePath(secureFileName)

	// For large files or non-image files, use streaming upload
	if kind != MediaKindImage || (!config.GenerateThumbnails && !config.ApplyWatermark) {
		return c.uploadStream(ctx, reader, gcsPath, kind, fileName)
	}

	// For images that need processing, read into memory
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	// Validate file content and MIME type
	if err := c.validateFileContent(content, fileName); err != nil {
		return nil, fmt.Errorf("file validation failed: %w", err)
	}

	result := &UploadResult{
		GCSPath: gcsPath,
		Kind:    kind,
		Size:    int64(len(content)),
	}

	// Process image if it's an image type
	if kind == MediaKindImage {
		processedContent, thumbPath, watermarkApplied, err := c.processImage(ctx, content, fileName, config)
		if err != nil {
			return nil, fmt.Errorf("failed to process image: %w", err)
		}

		result.ThumbPath = thumbPath
		result.WatermarkApplied = watermarkApplied
		content = processedContent
	}

	// Upload main file
	if err := c.uploadFile(ctx, content, gcsPath); err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return result, nil
}

// Delete removes a file from GCS
func (c *Client) Delete(ctx context.Context, gcsPath string) error {
	obj := c.client.Bucket(c.bucketName).Object(gcsPath)
	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete object %s: %w", gcsPath, err)
	}
	return nil
}

// GetSignedURL generates a signed URL for downloading a file
func (c *Client) GetSignedURL(ctx context.Context, gcsPath string, expiration time.Duration) (string, error) {
	opts := &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  "GET",
		Expires: time.Now().UTC().Add(expiration),
	}

	url, err := c.client.Bucket(c.bucketName).SignedURL(gcsPath, opts)
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL for %s: %w", gcsPath, err)
	}

	return url, nil
}

// CreateUploadConfigFromSettings creates UploadConfig from MediaSettings
func CreateUploadConfigFromSettings(settings MediaSettings) UploadConfig {
	// Convert opacity from 0.0-1.0 to 0-100
	opacity := int(settings.WatermarkOpacity * 100)
	if opacity > 100 {
		opacity = 100
	} else if opacity < 0 {
		opacity = 0
	}

	// Default thumbnail sizes if not specified
	thumbnailSizes := []int{200, 400}
	if len(settings.ThumbnailSizes) > 0 {
		thumbnailSizes = settings.ThumbnailSizes
	}

	return UploadConfig{
		GenerateThumbnails: settings.ThumbnailsEnabled,
		ApplyWatermark:     settings.WatermarkEnabled,
		WatermarkOpacity:   opacity,
		WatermarkPosition:  settings.WatermarkPosition,
		ThumbnailSizes:     thumbnailSizes,
	}
}

// generateSecureFileName creates a secure file name using UUID and slugify
func generateSecureFileName(originalName string) (string, error) {
	// Generate UUID for uniqueness
	uuid, err := generateUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}

	// Extract extension
	ext := strings.ToLower(filepath.Ext(originalName))

	// Clean base name (remove extension)
	baseName := strings.TrimSuffix(originalName, filepath.Ext(originalName))

	// Use our professional slugify implementation
	cleanName := Generate(baseName)

	// Combine: cleanName_uuid.ext
	return fmt.Sprintf("%s_%s%s", cleanName, uuid, ext), nil
}

// generateUUID creates a random UUID-like string
func generateUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// sanitizeFileName removes unsafe characters from filename
func sanitizeFileName(name string) string {
	// Replace unsafe characters with underscores
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
	clean := name
	for _, char := range unsafe {
		clean = strings.ReplaceAll(clean, char, "_")
	}

	// Limit length
	if len(clean) > 50 {
		clean = clean[:50]
	}

	return clean
}

// validateFileContent performs MIME type validation and security checks
func (c *Client) validateFileContent(content []byte, fileName string) error {
	if len(content) == 0 {
		return fmt.Errorf("file content is empty")
	}

	// Detect MIME type from content
	mimeType := http.DetectContentType(content)

	// Get expected MIME type from extension
	expectedMime := c.getContentType(fileName)

	// Validate MIME type matches extension
	if !c.isValidMimeType(mimeType, expectedMime) {
		return fmt.Errorf("file content type %s does not match extension, expected %s", mimeType, expectedMime)
	}

	// Additional security checks for images
	if strings.HasPrefix(mimeType, "image/") {
		if err := c.validateImageContent(content); err != nil {
			return fmt.Errorf("image validation failed: %w", err)
		}
	}

	return nil
}

// isValidMimeType checks if detected MIME type is compatible with expected type
func (c *Client) isValidMimeType(detected, expected string) bool {
	// Handle common MIME type variations
	validMimes := map[string][]string{
		"image/jpeg":               {"image/jpeg", "image/jpg"},
		"image/png":                {"image/png"},
		"image/webp":               {"image/webp"},
		"video/mp4":                {"video/mp4"},
		"application/pdf":          {"application/pdf"},
		"application/octet-stream": {"application/octet-stream"},
	}

	if allowedTypes, exists := validMimes[expected]; exists {
		for _, allowed := range allowedTypes {
			if detected == allowed {
				return true
			}
		}
	}

	return detected == expected
}

// validateImageContent performs additional validation for image files
func (c *Client) validateImageContent(content []byte) error {
	// Try to decode as image to ensure it's valid
	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("invalid image format: %w", err)
	}

	// Check image dimensions (prevent extremely large images)
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	const maxDimension = 10000 // 10k pixels max
	if width > maxDimension || height > maxDimension {
		return fmt.Errorf("image dimensions too large: %dx%d (max: %d)", width, height, maxDimension)
	}

	// Check for minimum dimensions
	const minDimension = 10
	if width < minDimension || height < minDimension {
		return fmt.Errorf("image dimensions too small: %dx%d (min: %d)", width, height, minDimension)
	}

	return nil
}

// LoadMediaSettingsFromDB loads media settings from database
// This function should be called from the service layer with proper database connection
func LoadMediaSettingsFromDB(db interface{}) (*MediaSettings, error) {
	// Use the new database integration if a proper DB client is provided
	if dbClient, ok := db.(DatabaseClient); ok {
		return LoadMediaSettingsFromDatabase(dbClient)
	}

	// Fallback to default settings for invalid/nil database connections
	return getDefaultMediaSettings(), nil
}

// GetPublicURL returns the public URL for a file (if bucket is public)
// GetPublicURL returns the public URL for a GCS object
// For private buckets, this should be replaced with GetSignedURL
func (c *Client) GetPublicURL(gcsPath string) string {
	if c.isPublic {
		// For public buckets, return direct public URL
		return fmt.Sprintf("https://storage.googleapis.com/%s/%s", c.bucketName, gcsPath)
	} else {
		// For private buckets, return a placeholder that indicates signed URL is needed
		// This should not be used directly - use GetSecureURL instead
		return fmt.Sprintf("gs://%s/%s", c.bucketName, gcsPath)
	}
}

// GetSecureURL returns a signed URL for private bucket access
// This should be used instead of GetPublicURL for private buckets
func (c *Client) GetSecureURL(ctx context.Context, gcsPath string, expiration time.Duration) (string, error) {
	if c.isPublic {
		// For public buckets, return public URL
		return c.GetPublicURL(gcsPath), nil
	}

	// For private buckets, generate signed URL
	return c.GetSignedURL(ctx, gcsPath, expiration)
}

// Close closes the GCS client
func (c *Client) Close() error {
	return c.client.Close()
}

// getMediaKind determines media kind from file extension
func (c *Client) getMediaKind(fileName string) MediaKind {
	ext := strings.ToLower(filepath.Ext(fileName))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return MediaKindImage
	case ".mp4":
		return MediaKindVideo
	case ".pdf":
		return MediaKindFile
	default:
		return MediaKindFile
	}
}

// generatePath creates a unique path for the file
func (c *Client) generatePath(fileName string) string {
	ext := filepath.Ext(fileName)
	name := strings.TrimSuffix(fileName, ext)

	// Use current timestamp for uniqueness
	timestamp := time.Now().UTC().Format("2006/01/02/15-04-05")

	return fmt.Sprintf("media/%s/%s%s", timestamp, name, ext)
}

// uploadStream uploads content directly from reader to GCS (for large files)
func (c *Client) uploadStream(ctx context.Context, reader io.Reader, gcsPath string, kind MediaKind, originalFileName string) (*UploadResult, error) {
	obj := c.client.Bucket(c.bucketName).Object(gcsPath)
	w := obj.NewWriter(ctx)
	w.ContentType = c.getContentType(gcsPath)
	defer w.Close()

	size, err := io.Copy(w, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to stream to GCS: %w", err)
	}

	return &UploadResult{
		GCSPath: gcsPath,
		Kind:    kind,
		Size:    size,
		// No thumbnail or watermark for streamed uploads
		ThumbPath:        "",
		WatermarkApplied: false,
	}, nil
}

// uploadFile uploads content to GCS
func (c *Client) uploadFile(ctx context.Context, content []byte, gcsPath string) error {
	obj := c.client.Bucket(c.bucketName).Object(gcsPath)
	w := obj.NewWriter(ctx)
	w.ContentType = c.getContentType(gcsPath)
	defer w.Close()

	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("failed to write to GCS: %w", err)
	}

	return nil
}

// getContentType determines content type from file extension
func (c *Client) getContentType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))

	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// processImage handles thumbnail generation and watermarking for images
func (c *Client) processImage(ctx context.Context, content []byte, fileName string, config UploadConfig) ([]byte, string, bool, error) {
	// Skip processing for WebP files (Go standard library doesn't support WebP decode)
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == ".webp" {
		return content, "", false, nil // Return WebP files as-is without processing
	}

	// Decode image
	img, format, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return content, "", false, nil // Return original if can't decode
	}

	var processedImg image.Image = img
	watermarkApplied := false

	// Apply watermark if enabled
	if config.ApplyWatermark {
		// Use image-based watermark (logo) implementation
		// الشعار مفرغ بدون خلفية، في أسفل اليمين
		watermarkConfig := ImageWatermarkConfig{
			LogoPath:   "assets/logo.png",
			Opacity:    config.WatermarkOpacity,
			Position:   "bottom-right", // أسفل اليمين
			Scale:      0.15,           // 15% من عرض الصورة
			Background: false,          // بدون خلفية (مفرغ)
		}

		watermarkedImg, err := c.ApplyImageWatermark(processedImg, watermarkConfig)
		if err == nil {
			processedImg = watermarkedImg
			watermarkApplied = true
		} else {
			// Log watermark error but continue with upload
			fmt.Printf("Warning: Failed to apply watermark: %v\n", err)
		}
	}

	// Generate thumbnail if enabled
	var thumbPath string
	if config.GenerateThumbnails && len(config.ThumbnailSizes) > 0 {
		// Use first thumbnail size
		thumbSize := config.ThumbnailSizes[0]
		if thumbSize == 0 {
			thumbSize = 200 // default thumbnail size
		}

		thumbImg := c.generateThumbnail(processedImg, thumbSize)
		thumbPath, err = c.uploadThumbnail(ctx, thumbImg, fileName, format)
		if err != nil {
			// Log error but don't fail the main upload
			fmt.Printf("Warning: Failed to upload thumbnail: %v\n", err)
			thumbPath = ""
		}
	}

	// Encode processed image
	processedContent, err := c.encodeImage(processedImg, format)
	if err != nil {
		return content, thumbPath, watermarkApplied, nil // Return original if encoding fails
	}

	return processedContent, thumbPath, watermarkApplied, nil
}

// applyWatermark applies a text watermark to the image
func (c *Client) applyWatermark(img image.Image, opacity int, position string) (image.Image, error) {
	bounds := img.Bounds()
	watermarked := image.NewRGBA(bounds)

	// Copy original image to new image
	draw.Draw(watermarked, bounds, img, bounds.Min, draw.Src)

	// Create watermark overlay
	watermarkColor := color.RGBA{
		R: 255,
		G: 255,
		B: 255,
		A: uint8(opacity * 255 / 100), // Convert percentage to alpha value
	}

	// Simple text-based watermark (company name)
	watermarkText := "لوفت الدغيري" // Loft Dughairi in Arabic

	// Calculate watermark dimensions and position
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()
	watermarkWidth := len(watermarkText) * 8 // Approximate character width
	watermarkHeight := 16                    // Approximate character height

	var startX, startY int

	switch strings.ToLower(position) {
	case "center":
		startX = (imgWidth - watermarkWidth) / 2
		startY = (imgHeight - watermarkHeight) / 2
	case "top-left":
		startX = imgWidth / 20 // 5% margin
		startY = imgHeight / 20
	case "top-right":
		startX = imgWidth - watermarkWidth - (imgWidth / 20)
		startY = imgHeight / 20
	case "bottom-left":
		startX = imgWidth / 20
		startY = imgHeight - watermarkHeight - (imgHeight / 20)
	case "bottom-right":
		startX = imgWidth - watermarkWidth - (imgWidth / 20)
		startY = imgHeight - watermarkHeight - (imgHeight / 20)
	default:
		// Default to bottom-right
		startX = imgWidth - watermarkWidth - (imgWidth / 20)
		startY = imgHeight - watermarkHeight - (imgHeight / 20)
	}

	// Apply simple rectangular watermark overlay
	// In a real implementation, this would render actual text
	watermarkRect := image.Rect(startX, startY, startX+watermarkWidth, startY+watermarkHeight)

	// Draw semi-transparent rectangle as watermark placeholder
	for y := watermarkRect.Min.Y; y < watermarkRect.Max.Y && y < bounds.Max.Y; y++ {
		for x := watermarkRect.Min.X; x < watermarkRect.Max.X && x < bounds.Max.X; x++ {
			if x >= 0 && y >= 0 {
				// Blend watermark color with existing pixel
				existingColor := watermarked.RGBAAt(x, y)
				alpha := float64(watermarkColor.A) / 255.0
				invAlpha := 1.0 - alpha

				blended := color.RGBA{
					R: uint8(float64(existingColor.R)*invAlpha + float64(watermarkColor.R)*alpha),
					G: uint8(float64(existingColor.G)*invAlpha + float64(watermarkColor.G)*alpha),
					B: uint8(float64(existingColor.B)*invAlpha + float64(watermarkColor.B)*alpha),
					A: existingColor.A,
				}

				watermarked.Set(x, y, blended)
			}
		}
	}

	return watermarked, nil
}

// generateThumbnail creates a high-quality thumbnail using Catmull-Rom interpolation
func (c *Client) generateThumbnail(img image.Image, maxWidth int) image.Image {
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	// Skip if image is already smaller than desired width
	if originalWidth <= maxWidth {
		return img
	}

	// Calculate proportional height
	ratio := float64(originalHeight) / float64(originalWidth)
	newWidth := maxWidth
	newHeight := int(math.Round(float64(newWidth) * ratio))

	// Ensure minimum dimensions
	if newHeight < 1 {
		newHeight = 1
	}

	// Create new thumbnail image
	thumbnail := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Use high-quality Catmull-Rom interpolation for better results
	// This is much better than nearest neighbor and produces sharp, clear thumbnails
	xdraw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), img, bounds, xdraw.Over, nil)

	return thumbnail
}

// uploadThumbnail uploads a thumbnail image
func (c *Client) uploadThumbnail(ctx context.Context, img image.Image, originalFileName, format string) (string, error) {
	// Generate thumbnail path
	ext := filepath.Ext(originalFileName)
	name := strings.TrimSuffix(originalFileName, ext)
	thumbPath := c.generatePath(name + "_thumb" + ext)

	// Encode thumbnail
	content, err := c.encodeImage(img, format)
	if err != nil {
		return "", err
	}

	// Upload thumbnail
	if err := c.uploadFile(ctx, content, thumbPath); err != nil {
		return "", err
	}

	return thumbPath, nil
}

// encodeImage encodes an image to bytes with optimized compression
func (c *Client) encodeImage(img image.Image, format string) ([]byte, error) {
	var buf bytes.Buffer

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		// High quality JPEG (85 is optimal balance between size and quality)
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
		if err != nil {
			return nil, fmt.Errorf("failed to encode as JPEG: %w", err)
		}
	case "png":
		// PNG with best compression
		encoder := png.Encoder{
			CompressionLevel: png.BestCompression,
		}
		err := encoder.Encode(&buf, img)
		if err != nil {
			return nil, fmt.Errorf("failed to encode as PNG: %w", err)
		}
	default:
		// Default to JPEG for unsupported formats
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
		if err != nil {
			return nil, fmt.Errorf("failed to encode as JPEG (default): %w", err)
		}
	}

	return buf.Bytes(), nil
}
