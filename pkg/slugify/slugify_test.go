package slugify

import (
	"strings"
	"testing"
	// TODO: Add these dependencies for full database testing:
	// "context"
	// "github.com/DATA-DOG/go-sqlmock"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Arabic text",
			input:    "حمام زاجل أصيل",
			expected: "hamam-zajil-asil",
		},
		{
			name:     "Mixed Arabic and English",
			input:    "حمام Pigeon زاجل",
			expected: "hamam-pigeon-zajil",
		},
		{
			name:     "English text",
			input:    "Beautiful Racing Pigeon",
			expected: "beautiful-racing-pigeon",
		},
		{
			name:     "Text with numbers",
			input:    "حمام رقم ١٢٣",
			expected: "hamam-rqm-123",
		},
		{
			name:     "Text with special characters",
			input:    "حمام زاجل... أصيل!!!",
			expected: "hamam-zajil-asil",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only special characters",
			input:    "!@#$%^&*()",
			expected: "slug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Generate(tt.input)
			if result != tt.expected {
				t.Errorf("Generate(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateWithConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		config   Config
		expected string
	}{
		{
			name:  "Preserve case",
			input: "حمام Pigeon",
			config: Config{
				MaxLength:    100,
				PreserveCase: true,
				Separator:    "-",
			},
			expected: "hamam-Pigeon",
		},
		{
			name:  "Custom separator",
			input: "حمام زاجل",
			config: Config{
				MaxLength: 100,
				Separator: "_",
			},
			expected: "hamam_zajil",
		},
		{
			name:  "Max length constraint",
			input: "حمام زاجل أصيل جميل جداً",
			config: Config{
				MaxLength: 15,
				Separator: "-",
			},
			expected: "hamam-zajil-asi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWithConfig(tt.input, tt.config)
			if result != tt.expected {
				t.Errorf("GenerateWithConfig(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConvertArabicToLatin(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"حمام", "hamam"},
		{"زاجل", "zajil"},
		{"أصيل", "asil"},
		{"١٢٣", "123"},
		{"الحمام", "alhamam"},
		{"مستلزمات", "mstlzmat"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertArabicToLatin(tt.input)
			if result != tt.expected {
				t.Errorf("convertArabicToLatin(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name      string
		slug      string
		shouldErr bool
	}{
		{"Valid slug", "valid-slug", false},
		{"Valid with numbers", "valid-slug-123", false},
		{"Empty slug", "", true},
		{"Starts with separator", "-invalid", true},
		{"Ends with separator", "invalid-", true},
		{"Consecutive separators", "invalid--slug", true},
		{"Invalid characters", "invalid@slug", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlug(tt.slug)
			if tt.shouldErr && err == nil {
				t.Errorf("ValidateSlug(%q) expected error but got none", tt.slug)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("ValidateSlug(%q) unexpected error: %v", tt.slug, err)
			}
		})
	}
}

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"حمام زاجل", "hamam-zajil"},
		{"  حمام  زاجل  ", "hamam-zajil"},
		{"حمام@زاجل#أصيل", "hamam-zajil-asil"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := SanitizeSlug(tt.input)
			if err != nil {
				t.Errorf("SanitizeSlug(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("SanitizeSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TODO: Uncomment and implement when go-sqlmock dependency is added
// func TestGenerateUnique(t *testing.T) {
// 	// Create a mock database
// 	db, mock, err := sqlmock.New()
// 	if err != nil {
// 		t.Fatalf("Failed to create mock database: %v", err)
// 	}
// 	defer db.Close()
//
// 	slugifier := NewSlugifier(db)
//
// 	t.Run("Unique slug on first try", func(t *testing.T) {
// 		// Mock that slug doesn't exist
// 		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM products WHERE slug = \$1`).
// 			WithArgs("hamam-zajil").
// 			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
//
// 		slug, err := slugifier.GenerateUnique(context.Background(), "حمام زاجل", "products", "slug")
// 		if err != nil {
// 			t.Errorf("GenerateUnique failed: %v", err)
// 		}
//
// 		expected := "hamam-zajil"
// 		if slug != expected {
// 			t.Errorf("GenerateUnique = %q, want %q", slug, expected)
// 		}
//
// 		if err := mock.ExpectationsWereMet(); err != nil {
// 			t.Errorf("Mock expectations not met: %v", err)
// 		}
// 	})
//
// 	t.Run("Generate unique with counter", func(t *testing.T) {
// 		// Mock that base slug exists
// 		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM products WHERE slug = \$1`).
// 			WithArgs("hamam-zajil").
// 			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
//
// 		// Mock that slug with counter doesn't exist
// 		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM products WHERE slug = \$1`).
// 			WithArgs("hamam-zajil-1").
// 			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
//
// 		slug, err := slugifier.GenerateUnique(context.Background(), "حمام زاجل", "products", "slug")
// 		if err != nil {
// 			t.Errorf("GenerateUnique failed: %v", err)
// 		}
//
// 		expected := "hamam-zajil-1"
// 		if slug != expected {
// 			t.Errorf("GenerateUnique = %q, want %q", slug, expected)
// 		}
//
// 		if err := mock.ExpectationsWereMet(); err != nil {
// 			t.Errorf("Mock expectations not met: %v", err)
// 		}
// 	})
// }

func TestBatchGenerate(t *testing.T) {
	inputs := []string{"حمام زاجل", "مستلزمات الحمام", "طعام الحمام"}
	config := DefaultConfig()

	results := BatchGenerate(inputs, config)

	expected := []string{"hamam-zajil", "mstlzmat-alhamam", "taam-alhamam"}

	if len(results) != len(expected) {
		t.Errorf("BatchGenerate returned %d results, want %d", len(results), len(expected))
		return
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("BatchGenerate[%d] = %q, want %q", i, result, expected[i])
		}
	}
}

func TestCleanupSeparators(t *testing.T) {
	tests := []struct {
		input     string
		separator string
		expected  string
	}{
		{"hello--world", "-", "hello-world"},
		{"hello---world", "-", "hello-world"},
		{"hello__world", "_", "hello_world"},
		{"hello-world", "-", "hello-world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanupSeparators(tt.input, tt.separator)
			if result != tt.expected {
				t.Errorf("cleanupSeparators(%q, %q) = %q, want %q", tt.input, tt.separator, result, tt.expected)
			}
		})
	}
}

func TestRemoveNonASCII(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello-world", "hello-world"},
		{"hello-世界", "hello-"},
		{"حمام-zajil", "-zajil"},
		{"123-abc", "123-abc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := removeNonASCII(tt.input)
			if result != tt.expected {
				t.Errorf("removeNonASCII(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestReplaceSpecialChars(t *testing.T) {
	tests := []struct {
		input     string
		separator string
		expected  string
	}{
		{"hello world", "-", "hello-world"},
		{"hello_world", "-", "hello-world"},
		{"hello.world", "-", "hello-world"},
		{"hello/world", "-", "hello-world"},
		{"hello@world#test", "-", "hello-world-test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := replaceSpecialChars(tt.input, tt.separator)
			if !strings.Contains(result, tt.separator) && tt.input != tt.expected {
				t.Errorf("replaceSpecialChars(%q, %q) = %q, expected to contain separator", tt.input, tt.separator, result)
			}
		})
	}
}
