package lib

import (
	"math"
	"strconv"
	"strings"
)

// ParseMoney converts a decimal string ("12.50", "1 234,50") into integer
// cents. Money is never kept as float64 inside business calculations; only the
// final formatted string is stored, which keeps totals exact to the cent.
func ParseMoney(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	if s == "" {
		return 0
	}
	// Accept both "12,50" and "12.50".
	if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 100))
}

// FormatMoney renders integer cents as a fixed 2-decimal string.
func FormatMoney(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := cents / 100
	frac := cents % 100
	out := strconv.FormatInt(whole, 10) + "." + pad2(frac)
	if neg {
		return "-" + out
	}
	return out
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// ParseRate converts a VAT percentage string ("20", "20.00") to float64.
func ParseRate(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// LineTotalCents computes quantity * unit price including VAT, rounded to the
// nearest cent. The unit price is in cents.
func LineTotalCents(quantity int, unitPriceCents int64, vatRate float64) int64 {
	base := float64(quantity) * float64(unitPriceCents)
	return int64(math.Round(base * (1 + vatRate/100)))
}
