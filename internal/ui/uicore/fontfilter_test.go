package uicore

import (
	"reflect"
	"testing"
)

func TestFilterFonts(t *testing.T) {
	names := []string{"", "Arial", "Avenir Next", "Georgia", "Helvetica Neue", "Noto Sans Georgian", "Times New Roman"}
	tests := []struct {
		q    string
		keep int
		want []int
	}{
		{"", -1, []int{0, 1, 2, 3, 4, 5, 6}},
		{"  ", 3, []int{0, 1, 2, 3, 4, 5, 6}},
		{"geo", -1, []int{3, 5}},
		{"GEORG", 6, []int{6, 3, 5}},
		{"new", -1, []int{6}},
		{"sans geo", -1, []int{5}},
		{"a", 2, []int{1, 2, 3, 4, 5, 6}},
		{"zzz", -1, nil},
		{"zzz", 4, []int{4}},
		{"zzz", 99, nil},
	}
	for _, tt := range tests {
		if got := FilterFonts(names, tt.q, tt.keep); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("FilterFonts(%q, %d) = %v, want %v", tt.q, tt.keep, got, tt.want)
		}
	}
}
