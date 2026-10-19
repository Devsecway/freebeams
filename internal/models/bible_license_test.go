package models

import "testing"

func TestIsPublicDomainBible(t *testing.T) {
	tests := []struct {
		abbrev, name string
		want         bool
	}{
		{"KJV", "King James Version", true},
		{"kjv", "", true},
		{"ASV", "", true},
		{" asv ", "", true},
		{"", "American  Standard Version", true},
		{"WYC", "Wycliffe Bible", true},
		{"X", "Douay-Rheims 1899 American Edition", true},
		{"HCSB", "Holman Christian Standard Bible", false},
		{"NIV", "New International Version", false},
		{"NKJV", "New King James Version", false},
		{"MSG", "The Message", false},
		{"", "", false},
	}
	for _, tt := range tests {
		if got := IsPublicDomainBible(tt.abbrev, tt.name); got != tt.want {
			t.Errorf("IsPublicDomainBible(%q, %q) = %v, want %v", tt.abbrev, tt.name, got, tt.want)
		}
	}
}
