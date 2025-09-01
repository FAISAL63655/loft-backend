package slugify

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// arabicToLatinMap maps Arabic characters to Latin equivalents
var arabicToLatinMap = map[rune]string{
	'ا': "a", 'أ': "a", 'إ': "i", 'آ': "aa",
	'ب': "b", 'ت': "t", 'ث': "th",
	'ج': "j", 'ح': "h", 'خ': "kh",
	'د': "d", 'ذ': "th", 'ر': "r",
	'ز': "z", 'س': "s", 'ش': "sh",
	'ص': "s", 'ض': "d", 'ط': "t", 'ظ': "z",
	'ع': "a", 'غ': "gh", 'ف': "f",
	'ق': "q", 'ك': "k", 'ل': "l",
	'م': "m", 'ن': "n", 'ه': "h",
	'و': "w", 'ي': "y", 'ى': "a",
	'ة': "h", 'ء': "a",
	// Arabic numbers
	'٠': "0", '١': "1", '٢': "2", '٣': "3", '٤': "4",
	'٥': "5", '٦': "6", '٧': "7", '٨': "8", '٩': "9",
	// Additional diacritics (usually ignored in slugs)
	'َ': "", 'ُ': "", 'ِ': "", 'ً': "", 'ٌ': "", 'ٍ': "",
	'ْ': "", 'ّ': "", 'ـ': "",
}

// Slugifier handles slug generation with uniqueness checking
type Slugifier struct {
	db *sql.DB
}

// Config holds configuration for the slugifier
type Config struct {
	MaxLength    int    // Maximum length of the slug (default: 100)
	PreserveCase bool   // Whether to preserve case (default: false - converts to lowercase)
	AllowUnicode bool   // Whether to allow unicode characters (default: false)
	Separator    string // Character to use as separator (default: "-")
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		MaxLength:    100,
		PreserveCase: false,
		AllowUnicode: false,
		Separator:    "-",
	}
}

// NewSlugifier creates a new slugifier instance
func NewSlugifier(db *sql.DB) *Slugifier {
	return &Slugifier{db: db}
}

// Generate creates a slug from input text with default configuration
func Generate(input string) string {
	return GenerateWithConfig(input, DefaultConfig())
}

// GenerateWithConfig creates a slug from input text with custom configuration
func GenerateWithConfig(input string, config Config) string {
	if input == "" {
		return ""
	}

	// Step 1: Convert Arabic characters to Latin
	result := convertArabicToLatin(input)

	// Step 2: Handle case conversion
	if !config.PreserveCase {
		result = strings.ToLower(result)
	}

	// Step 3: Replace spaces and special characters with separator
	result = replaceSpecialChars(result, config.Separator)

	// Step 4: Handle unicode if not allowed
	if !config.AllowUnicode {
		result = removeNonASCII(result)
	}

	// Step 5: Clean up multiple separators
	result = cleanupSeparators(result, config.Separator)

	// Step 6: Trim separators from ends
	result = strings.Trim(result, config.Separator)

	// Step 7: Enforce maximum length
	if config.MaxLength > 0 && len(result) > config.MaxLength {
		result = result[:config.MaxLength]
		result = strings.Trim(result, config.Separator)
	}

	// Step 8: Ensure we have something
	if result == "" {
		result = "slug"
	}

	return result
}

// GenerateUnique creates a unique slug by checking against a database table
func (s *Slugifier) GenerateUnique(ctx context.Context, input, tableName, columnName string) (string, error) {
	return s.GenerateUniqueWithConfig(ctx, input, tableName, columnName, DefaultConfig())
}

// GenerateUniqueWithConfig creates a unique slug with custom configuration
func (s *Slugifier) GenerateUniqueWithConfig(ctx context.Context, input, tableName, columnName string, config Config) (string, error) {
	baseSlug := GenerateWithConfig(input, config)

	// Check if base slug is unique
	exists, err := s.slugExists(ctx, baseSlug, tableName, columnName)
	if err != nil {
		return "", fmt.Errorf("failed to check slug existence: %w", err)
	}

	if !exists {
		return baseSlug, nil
	}

	// Generate unique slug with counter
	counter := 1
	for {
		candidateSlug := fmt.Sprintf("%s%s%d", baseSlug, config.Separator, counter)

		// Check length constraint
		if config.MaxLength > 0 && len(candidateSlug) > config.MaxLength {
			// Truncate base slug to make room for counter
			counterStr := fmt.Sprintf("%s%d", config.Separator, counter)
			maxBaseLength := config.MaxLength - len(counterStr)
			if maxBaseLength <= 0 {
				return "", fmt.Errorf("cannot generate unique slug within length constraint")
			}

			truncatedBase := baseSlug
			if len(truncatedBase) > maxBaseLength {
				truncatedBase = truncatedBase[:maxBaseLength]
				truncatedBase = strings.Trim(truncatedBase, config.Separator)
			}
			candidateSlug = truncatedBase + counterStr
		}

		exists, err := s.slugExists(ctx, candidateSlug, tableName, columnName)
		if err != nil {
			return "", fmt.Errorf("failed to check slug existence: %w", err)
		}

		if !exists {
			return candidateSlug, nil
		}

		counter++
		if counter > 9999 { // Prevent infinite loop
			return "", fmt.Errorf("unable to generate unique slug after 9999 attempts")
		}
	}
}

// convertArabicToLatin converts Arabic characters to their Latin equivalents
func convertArabicToLatin(input string) string {
	var result strings.Builder

	for _, r := range input {
		if latin, exists := arabicToLatinMap[r]; exists {
			result.WriteString(latin)
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// replaceSpecialChars replaces spaces and special characters with the separator
func replaceSpecialChars(input, separator string) string {
	// Replace common separators and spaces
	replacements := []string{" ", "_", ".", ",", ";", ":", "/", "\\", "+", "=", "&", "%", "$", "#", "@", "!", "?", "*", "(", ")", "[", "]", "{", "}", "<", ">", "\"", "'", "`", "~"}

	result := input
	for _, char := range replacements {
		result = strings.ReplaceAll(result, char, separator)
	}

	return result
}

// removeNonASCII removes non-ASCII characters
func removeNonASCII(input string) string {
	return strings.Map(func(r rune) rune {
		if r > 127 {
			return -1 // Remove non-ASCII characters
		}
		return r
	}, input)
}

// cleanupSeparators removes multiple consecutive separators
func cleanupSeparators(input, separator string) string {
	// Create a regex to match multiple separators
	pattern := regexp.QuoteMeta(separator) + "+"
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(input, separator)
}

// slugExists checks if a slug already exists in the database
func (s *Slugifier) slugExists(ctx context.Context, slug, tableName, columnName string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database connection not provided")
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", tableName, columnName)

	var count int
	err := s.db.QueryRowContext(ctx, query, slug).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// ValidateSlug checks if a slug is valid according to basic rules
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}

	if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") {
		return fmt.Errorf("slug cannot start or end with separator")
	}

	if strings.Contains(slug, "--") {
		return fmt.Errorf("slug cannot contain consecutive separators")
	}

	// Check for invalid characters (only allow alphanumeric and hyphens)
	for _, r := range slug {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return fmt.Errorf("slug contains invalid character: %c", r)
		}
	}

	return nil
}

// SanitizeSlug cleans and validates a slug
func SanitizeSlug(input string) (string, error) {
	slug := Generate(input)

	if err := ValidateSlug(slug); err != nil {
		return "", err
	}

	return slug, nil
}

// BatchGenerate generates slugs for multiple inputs
func BatchGenerate(inputs []string, config Config) []string {
	results := make([]string, len(inputs))

	for i, input := range inputs {
		results[i] = GenerateWithConfig(input, config)
	}

	return results
}

// BatchGenerateUnique generates unique slugs for multiple inputs
func (s *Slugifier) BatchGenerateUnique(ctx context.Context, inputs []string, tableName, columnName string, config Config) ([]string, error) {
	results := make([]string, len(inputs))

	for i, input := range inputs {
		slug, err := s.GenerateUniqueWithConfig(ctx, input, tableName, columnName, config)
		if err != nil {
			return nil, fmt.Errorf("failed to generate unique slug for input %d: %w", i, err)
		}
		results[i] = slug
	}

	return results, nil
}
