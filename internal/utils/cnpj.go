package utils

import (
	"regexp"
	"strconv"
)

// CleanCNPJ removes all non-numeric characters from CNPJ
func CleanCNPJ(cnpj string) string {
	// Remove all non-numeric characters
	re := regexp.MustCompile(`\D`)
	return re.ReplaceAllString(cnpj, "")
}

// FormatCNPJ formats CNPJ with dots, slash and dash (XX.XXX.XXX/XXXX-XX)
func FormatCNPJ(cnpj string) string {
	cleaned := CleanCNPJ(cnpj)
	if len(cleaned) != 14 {
		return cnpj // Return original if invalid length
	}

	return cleaned[:2] + "." + cleaned[2:5] + "." + cleaned[5:8] + "/" + cleaned[8:12] + "-" + cleaned[12:14]
}

// IsValidCNPJ validates CNPJ using the official algorithm
func IsValidCNPJ(cnpj string) bool {
	cleaned := CleanCNPJ(cnpj)

	// Check length
	if len(cleaned) != 14 {
		return false
	}

	// Check if all digits are the same
	if isAllSameDigit(cleaned) {
		return false
	}

	// Convert to slice of integers
	digits := make([]int, 14)
	for i, char := range cleaned {
		digit, err := strconv.Atoi(string(char))
		if err != nil {
			return false
		}
		digits[i] = digit
	}

	// Validate first check digit
	if !isValidCheckDigit(digits[:12], digits[12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) {
		return false
	}

	// Validate second check digit
	if !isValidCheckDigit(digits[:13], digits[13], []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) {
		return false
	}

	return true
}

// isAllSameDigit checks if all digits in the string are the same
func isAllSameDigit(s string) bool {
	if len(s) == 0 {
		return false
	}

	first := s[0]
	for _, char := range s {
		if byte(char) != first {
			return false
		}
	}
	return true
}

// isValidCheckDigit validates a check digit using the given weights
func isValidCheckDigit(digits []int, checkDigit int, weights []int) bool {
	sum := 0
	for i, digit := range digits {
		sum += digit * weights[i]
	}

	remainder := sum % 11
	expectedDigit := 0
	if remainder >= 2 {
		expectedDigit = 11 - remainder
	}

	return expectedDigit == checkDigit
}

// GenerateRandomCNPJ generates a random valid CNPJ for testing purposes
func GenerateRandomCNPJ() string {
	// This is a simplified version for testing
	// In production, you might want a more sophisticated generator
	base := "11222333000100" // Base CNPJ

	// Calculate check digits
	digits := make([]int, 12)
	for i, char := range base[:12] {
		digits[i], _ = strconv.Atoi(string(char))
	}

	// Calculate first check digit
	firstCheck := calculateCheckDigit(digits, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})

	// Add first check digit and calculate second
	digits = append(digits, firstCheck)
	secondCheck := calculateCheckDigit(digits, []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})

	return base[:12] + strconv.Itoa(firstCheck) + strconv.Itoa(secondCheck)
}

// calculateCheckDigit calculates check digit using given weights
func calculateCheckDigit(digits []int, weights []int) int {
	sum := 0
	for i, digit := range digits {
		sum += digit * weights[i]
	}

	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

// ExtractCNPJFromText extracts CNPJ numbers from text
func ExtractCNPJFromText(text string) []string {
	// Pattern for formatted CNPJ: XX.XXX.XXX/XXXX-XX
	formattedPattern := regexp.MustCompile(`\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}`)

	// Pattern for unformatted CNPJ: 14 consecutive digits
	unformattedPattern := regexp.MustCompile(`\b\d{14}\b`)

	var cnpjs []string

	// Find formatted CNPJs
	formatted := formattedPattern.FindAllString(text, -1)
	for _, cnpj := range formatted {
		if IsValidCNPJ(cnpj) {
			cnpjs = append(cnpjs, CleanCNPJ(cnpj))
		}
	}

	// Find unformatted CNPJs
	unformatted := unformattedPattern.FindAllString(text, -1)
	for _, cnpj := range unformatted {
		if IsValidCNPJ(cnpj) {
			// Check if not already added (from formatted search)
			cleaned := CleanCNPJ(cnpj)
			found := false
			for _, existing := range cnpjs {
				if existing == cleaned {
					found = true
					break
				}
			}
			if !found {
				cnpjs = append(cnpjs, cleaned)
			}
		}
	}

	return cnpjs
}

// NormalizeCNPJ normalizes CNPJ by cleaning and validating
func NormalizeCNPJ(cnpj string) (string, bool) {
	cleaned := CleanCNPJ(cnpj)
	valid := IsValidCNPJ(cleaned)
	return cleaned, valid
}

// CNPJInfo holds information about a CNPJ
type CNPJInfo struct {
	Original  string `json:"original"`
	Cleaned   string `json:"cleaned"`
	Formatted string `json:"formatted"`
	Valid     bool   `json:"valid"`
}

// AnalyzeCNPJ analyzes a CNPJ string and returns detailed information
func AnalyzeCNPJ(cnpj string) CNPJInfo {
	cleaned := CleanCNPJ(cnpj)
	valid := IsValidCNPJ(cleaned)
	formatted := ""

	if valid {
		formatted = FormatCNPJ(cleaned)
	}

	return CNPJInfo{
		Original:  cnpj,
		Cleaned:   cleaned,
		Formatted: formatted,
		Valid:     valid,
	}
}

// GetCNPJType returns the type of CNPJ (MATRIZ or FILIAL)
func GetCNPJType(cnpj string) string {
	cleaned := CleanCNPJ(cnpj)
	if len(cleaned) != 14 {
		return "INVALID"
	}

	// The branch number is positions 8-11 (0-indexed)
	branchNumber := cleaned[8:12]

	if branchNumber == "0001" {
		return "MATRIZ"
	}
	return "FILIAL"
}

// GetCNPJRoot returns the root CNPJ (first 8 digits)
func GetCNPJRoot(cnpj string) string {
	cleaned := CleanCNPJ(cnpj)
	if len(cleaned) != 14 {
		return ""
	}

	return cleaned[:8]
}

// GetCNPJBranch returns the branch number (positions 8-11)
func GetCNPJBranch(cnpj string) string {
	cleaned := CleanCNPJ(cnpj)
	if len(cleaned) != 14 {
		return ""
	}

	return cleaned[8:12]
}

// AreSameCNPJRoot checks if two CNPJs belong to the same company (same root)
func AreSameCNPJRoot(cnpj1, cnpj2 string) bool {
	root1 := GetCNPJRoot(cnpj1)
	root2 := GetCNPJRoot(cnpj2)

	return root1 != "" && root2 != "" && root1 == root2
}
