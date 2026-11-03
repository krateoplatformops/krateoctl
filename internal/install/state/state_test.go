package state

import (
	"reflect"
	"testing"
)

func TestMergeFinalizers(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		desired  []string
		want     []string
	}{
		{
			name:     "preserves existing order and appends new finalizers once",
			existing: []string{"alpha", InstallationFinalizer},
			desired:  []string{InstallationFinalizer, "beta"},
			want:     []string{"alpha", InstallationFinalizer, "beta"},
		},
		{
			name:    "returns desired finalizers when none exist",
			desired: []string{InstallationFinalizer, "beta"},
			want:    []string{InstallationFinalizer, "beta"},
		},
		{
			name:     "keeps existing entries when desired is empty",
			existing: []string{"alpha"},
			want:     []string{"alpha"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			merged := mergeFinalizers(tc.existing, tc.desired)
			if !reflect.DeepEqual(merged, tc.want) {
				t.Fatalf("mergeFinalizers(%v, %v) = %v, want %v", tc.existing, tc.desired, merged, tc.want)
			}
		})
	}
}
