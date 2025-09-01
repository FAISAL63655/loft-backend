package catalog

import (
	"testing"
)

func TestProductsListRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		req         ProductsListRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid request with all filters",
			req: ProductsListRequest{
				TypeStr:   "pigeon",
				StatusStr: "available",
				Q:         "test search",
				PriceMin:  100.0,
				PriceMax:  500.0,
				Page:      1,
				Limit:     20,
				SortStr:   "newest",
			},
			expectError: false,
		},
		{
			name: "Valid request with minimal data",
			req: ProductsListRequest{
				Page:  1,
				Limit: 10,
			},
			expectError: false,
		},
		{
			name: "Invalid type filter",
			req: ProductsListRequest{
				TypeStr: "invalid_type",
				Page:    1,
				Limit:   20,
			},
			expectError: true,
			errorMsg:    "type must be 'pigeon' or 'supply'",
		},
		{
			name: "Invalid status filter",
			req: ProductsListRequest{
				StatusStr: "invalid_status",
				Page:      1,
				Limit:     20,
			},
			expectError: true,
			errorMsg:    "invalid status value",
		},
		{
			name: "Invalid price range - negative min",
			req: ProductsListRequest{
				PriceMin: -100.0,
				Page:     1,
				Limit:    20,
			},
			expectError: true,
			errorMsg:    "price_min must be non-negative",
		},
		{
			name: "Invalid price range - min > max",
			req: ProductsListRequest{
				PriceMin: 500.0,
				PriceMax: 100.0,
				Page:     1,
				Limit:    20,
			},
			expectError: true,
			errorMsg:    "price_min cannot be greater than price_max",
		},
		{
			name: "Invalid sort parameter",
			req: ProductsListRequest{
				SortStr: "invalid_sort",
				Page:    1,
				Limit:   20,
			},
			expectError: true,
			errorMsg:    "sort must be one of: newest, oldest, price_asc, price_desc",
		},
		{
			name: "Auto-correct page and limit",
			req: ProductsListRequest{
				Page:  0,   // Should be corrected to 1
				Limit: 200, // Should be corrected to 100 (max)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("Expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Check auto-corrections
				if tt.req.Page <= 0 && tt.req.Page != 1 {
					t.Errorf("Page should be auto-corrected to 1, got %d", tt.req.Page)
				}
				if tt.req.Limit > 100 && tt.req.Limit != 100 {
					t.Errorf("Limit should be auto-corrected to 100, got %d", tt.req.Limit)
				}
				if tt.req.Limit <= 0 && tt.req.Limit != 20 {
					t.Errorf("Limit should be auto-corrected to 20, got %d", tt.req.Limit)
				}
			}
		})
	}
}

func TestProductsListRequestHelperMethods(t *testing.T) {
	req := ProductsListRequest{
		TypeStr:   "pigeon",
		StatusStr: "available",
		Q:         "test search",
		PriceMin:  100.0,
		PriceMax:  500.0,
		SortStr:   "price_asc",
	}

	// Test GetType
	typePtr := req.GetType()
	if typePtr == nil || *typePtr != "pigeon" {
		t.Errorf("GetType() should return 'pigeon', got %v", typePtr)
	}

	// Test GetStatus
	statusPtr := req.GetStatus()
	if statusPtr == nil || *statusPtr != "available" {
		t.Errorf("GetStatus() should return 'available', got %v", statusPtr)
	}

	// Test GetQ
	qPtr := req.GetQ()
	if qPtr == nil || *qPtr != "test search" {
		t.Errorf("GetQ() should return 'test search', got %v", qPtr)
	}

	// Test GetPriceMin
	priceMinPtr := req.GetPriceMin()
	if priceMinPtr == nil || *priceMinPtr != 100.0 {
		t.Errorf("GetPriceMin() should return 100.0, got %v", priceMinPtr)
	}

	// Test GetPriceMax
	priceMaxPtr := req.GetPriceMax()
	if priceMaxPtr == nil || *priceMaxPtr != 500.0 {
		t.Errorf("GetPriceMax() should return 500.0, got %v", priceMaxPtr)
	}

	// Test GetSort
	sortPtr := req.GetSort()
	if sortPtr == nil || *sortPtr != "price_asc" {
		t.Errorf("GetSort() should return 'price_asc', got %v", sortPtr)
	}
}

func TestProductsListRequestEmptyValues(t *testing.T) {
	req := ProductsListRequest{
		TypeStr:   "",
		StatusStr: "",
		Q:         "",
		PriceMin:  0.0,
		PriceMax:  0.0,
		SortStr:   "",
	}

	// Test that empty values return nil
	if req.GetType() != nil {
		t.Error("GetType() should return nil for empty string")
	}

	if req.GetStatus() != nil {
		t.Error("GetStatus() should return nil for empty string")
	}

	if req.GetQ() != nil {
		t.Error("GetQ() should return nil for empty string")
	}

	if req.GetPriceMin() != nil {
		t.Error("GetPriceMin() should return nil for zero value")
	}

	if req.GetPriceMax() != nil {
		t.Error("GetPriceMax() should return nil for zero value")
	}

	if req.GetSort() != nil {
		t.Error("GetSort() should return nil for empty string")
	}
}
