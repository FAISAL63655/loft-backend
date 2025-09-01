// Package money_sar provides SAR currency calculations and VAT utilities
// Note: Named "money_sar" to distinguish from "moyasar" (payment provider)
package moneysar

import "math"

// RoundHalfUp rounds value to the specified decimals using HALF-UP mode
func RoundHalfUp(value float64, decimals int) float64 {
	mult := math.Pow(10, float64(decimals))
	if value >= 0 {
		return math.Floor(value*mult+0.5) / mult
	}
	return -math.Floor(-value*mult+0.5) / mult
}

// GrossFromNet returns gross from net using given vat rate (e.g., 0.15)
func GrossFromNet(net, vatRate float64) float64 {
	return RoundHalfUp(net*(1+vatRate), 2)
}
