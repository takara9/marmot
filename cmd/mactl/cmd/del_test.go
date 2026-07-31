package cmd

import (
	"reflect"
	"testing"
)

func TestParseCommaSeparatedNames(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{
			name:  "issue 603 sample with commas and spaces",
			input: []string{"biz,", "rest1,", "rest2,", "rest3,", "db"},
			want:  []string{"biz", "rest1", "rest2", "rest3", "db"},
		},
		{
			name:  "single arg with comma list",
			input: []string{"biz,rest1,rest2,rest3,db"},
			want:  []string{"biz", "rest1", "rest2", "rest3", "db"},
		},
		{
			name:    "empty values only",
			input:   []string{" , ", ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommaSeparatedNames(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parsed names mismatch: got=%v want=%v", got, tt.want)
			}
		})
	}
}
