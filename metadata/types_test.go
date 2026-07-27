package metadata

import "testing"

func TestNormalizeOriginalLanguage(t *testing.T) {
	if NormalizeOriginalLanguage("  EN ") != "en" {
		t.Fatal(NormalizeOriginalLanguage("  EN "))
	}
}
