// Package storagegcs - Arabic and Unicode-aware slugify implementation
package storagegcs

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// slugifyConfig holds configuration for slugification
type slugifyConfig struct {
	MaxLength      int
	Separator      string
	RemoveDuplicates bool
	ToLower        bool
}

// defaultSlugifyConfig returns default configuration
func defaultSlugifyConfig() slugifyConfig {
	return slugifyConfig{
		MaxLength:        50,
		Separator:        "_",
		RemoveDuplicates: true,
		ToLower:          true,
	}
}

// arabicTransliterationMap maps Arabic characters to Latin equivalents
var arabicTransliterationMap = map[rune]string{
	'ا': "a", 'أ': "a", 'إ': "i", 'آ': "aa",
	'ب': "b", 'ت': "t", 'ث': "th", 'ج': "j",
	'ح': "h", 'خ': "kh", 'د': "d", 'ذ': "dh",
	'ر': "r", 'ز': "z", 'س': "s", 'ش': "sh",
	'ص': "s", 'ض': "d", 'ط': "t", 'ظ': "z",
	'ع': "a", 'غ': "gh", 'ف': "f", 'ق': "q",
	'ك': "k", 'ل': "l", 'م': "m", 'ن': "n",
	'ه': "h", 'و': "w", 'ي': "y", 'ة': "h",
	'ى': "a", 'ء': "a",
	// Arabic numbers
	'٠': "0", '١': "1", '٢': "2", '٣': "3", '٤': "4",
	'٥': "5", '٦': "6", '٧': "7", '٨': "8", '٩': "9",
}

// specialCharMap maps special characters to safe alternatives
var specialCharMap = map[rune]string{
	'&': "and", '@': "at", '+': "plus", '%': "percent",
	'$': "dollar", '€': "euro", '£': "pound", '¥': "yen",
	'#': "hash", '!': "exclamation", '?': "question",
	'*': "star", '<': "lt", '>': "gt", '=': "eq",
	'|': "pipe", '\\': "backslash", '/': "slash",
	':': "colon", ';': "semicolon", '"': "quote",
	'\'': "apostrophe", '`': "backtick", '~': "tilde",
	'^': "caret", '[': "lbracket", ']': "rbracket",
	'{': "lbrace", '}': "rbrace", '(': "lparen", ')': "rparen",
}

// Generate creates a URL-safe slug from input text
// Supports Arabic, English, numbers, and special characters
func Generate(input string) string {
	return GenerateWithConfig(input, defaultSlugifyConfig())
}

// GenerateWithConfig creates a slug with custom configuration
func GenerateWithConfig(input string, config slugifyConfig) string {
	if input == "" {
		return ""
	}

	// Step 1: Normalize unicode and trim whitespace
	normalized := strings.TrimSpace(input)
	
	// Step 2: Convert to lowercase if configured
	if config.ToLower {
		normalized = strings.ToLower(normalized)
	}

	// Step 3: Process each character
	var result strings.Builder
	result.Grow(len(normalized) * 2) // Pre-allocate for performance

	for _, r := range normalized {
		// Handle Arabic characters
		if arabic, exists := arabicTransliterationMap[r]; exists {
			result.WriteString(arabic)
			continue
		}

		// Handle special characters
		if special, exists := specialCharMap[r]; exists {
			if result.Len() > 0 && !strings.HasSuffix(result.String(), config.Separator) {
				result.WriteString(config.Separator)
			}
			result.WriteString(special)
			result.WriteString(config.Separator)
			continue
		}

		// Handle Latin letters, numbers, and safe characters
		if isSlugSafe(r) {
			result.WriteRune(r)
			continue
		}

		// Handle spaces and whitespace
		if unicode.IsSpace(r) {
			if result.Len() > 0 && !strings.HasSuffix(result.String(), config.Separator) {
				result.WriteString(config.Separator)
			}
			continue
		}

		// Handle other Unicode letters (convert to ASCII if possible)
		if unicode.IsLetter(r) {
			// Try to convert accented characters to base form
			if ascii := convertToAscii(r); ascii != "" {
				result.WriteString(ascii)
				continue
			}
		}

		// Skip unsupported characters (don't add separators for them)
	}

	slug := result.String()

	// Step 4: Clean up separators
	if config.RemoveDuplicates {
		slug = removeDuplicateSeparators(slug, config.Separator)
	}

	// Step 5: Trim separators from start and end
	slug = strings.Trim(slug, config.Separator)

	// Step 6: Apply length limit
	if config.MaxLength > 0 && utf8.RuneCountInString(slug) > config.MaxLength {
		slug = truncateAtRune(slug, config.MaxLength)
		// Ensure we don't end with a separator after truncation
		slug = strings.TrimSuffix(slug, config.Separator)
	}

	// Step 7: Ensure minimum length and fallback
	if slug == "" {
		slug = "file"
	}

	return slug
}

// isSlugSafe checks if a character is safe for URLs without encoding
func isSlugSafe(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		   (r >= 'A' && r <= 'Z') ||
		   (r >= '0' && r <= '9') ||
		   r == '-' || r == '_' || r == '.'
}

// convertToAscii attempts to convert accented characters to ASCII
func convertToAscii(r rune) string {
	// Common accented character mappings
	accentMap := map[rune]string{
		'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
		'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
		'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
		'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o",
		'ù': "u", 'ú': "u", 'û': "u", 'ü': "u",
		'ý': "y", 'ÿ': "y",
		'ç': "c", 'ñ': "n",
		'ß': "ss",
	}

	if ascii, exists := accentMap[r]; exists {
		return ascii
	}

	return ""
}

// removeDuplicateSeparators removes consecutive separators
func removeDuplicateSeparators(s, separator string) string {
	if separator == "" {
		return s
	}

	// Use regex to replace multiple consecutive separators with single one
	pattern := regexp.QuoteMeta(separator) + "+"
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(s, separator)
}

// truncateAtRune truncates string at specified rune count
func truncateAtRune(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	return string(runes[:maxRunes])
}

// generateSlugFromArabic specifically handles Arabic text slugification
func generateSlugFromArabic(arabicText string) string {
	config := slugifyConfig{
		MaxLength:        40,
		Separator:        "_",
		RemoveDuplicates: true,
		ToLower:          true,
	}

	return GenerateWithConfig(arabicText, config)
}

// ValidateSlug checks if a slug is valid for file naming
func ValidateSlug(slug string) bool {
	if slug == "" {
		return false
	}

	// Check length
	if len(slug) > 100 {
		return false
	}

	// Check for invalid characters
	for _, r := range slug {
		if !isSlugSafe(r) && r != '_' && r != '-' {
			return false
		}
	}

	// Ensure it doesn't start or end with separators
	if strings.HasPrefix(slug, "_") || strings.HasPrefix(slug, "-") ||
	   strings.HasSuffix(slug, "_") || strings.HasSuffix(slug, "-") {
		return false
	}

	return true
}

// SanitizeForStorage creates a storage-safe filename
// Combines slugification with additional storage-specific rules
func SanitizeForStorage(filename string) string {
	// Extract extension first
	ext := strings.ToLower(filepath.Ext(filename))
	baseName := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Generate slug from base name
	slug := Generate(baseName)

	// Ensure minimum length
	if len(slug) < 3 {
		slug = "file_" + slug
	}

	// Apply storage-specific constraints
	if len(slug) > 50 {
		slug = slug[:50]
	}

	// Remove trailing separators that might have been created by truncation
	slug = strings.TrimSuffix(strings.TrimSuffix(slug, "_"), "-")

	return slug + ext
}
