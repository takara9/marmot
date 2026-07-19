package virt

import "testing"

func TestCephSecretValueFromFileContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "extracts key from keyring format",
			content: "[client.takara]\n\tkey = AQCrPFxq4uOpMhAAjDKNPj5OYDt80fJBIG4zgQ==\n",
			want: "AQCrPFxq4uOpMhAAjDKNPj5OYDt80fJBIG4zgQ==",
		},
		{
			name: "returns raw key when file contains key only",
			content: "AQCrPFxq4uOpMhAAjDKNPj5OYDt80fJBIG4zgQ==\n",
			want: "AQCrPFxq4uOpMhAAjDKNPj5OYDt80fJBIG4zgQ==",
		},
		{
			name: "trims surrounding spaces for raw key",
			content: "  AQCrPFxq4uOpMhAAjDKNPj5OYDt80fJBIG4zgQ==  \n",
			want: "AQCrPFxq4uOpMhAAjDKNPj5OYDt80fJBIG4zgQ==",
		},
		{
			name: "returns empty for empty content",
			content: " \n\t ",
			want: "",
		},
		{
			name: "returns empty for multiline content without key field",
			content: "[client.takara]\nmon host = 10.0.0.1\n",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cephSecretValueFromFileContent([]byte(tt.content))
			if got != tt.want {
				t.Fatalf("cephSecretValueFromFileContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCephSecretValueBytesFromFileContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []byte
		wantErr bool
	}{
		{
			name:    "decodes key from keyring format",
			content: "[client.takara]\n\tkey = dGVzdA==\n",
			want:    []byte("test"),
		},
		{
			name:    "decodes raw base64 key",
			content: "dGVzdA==\n",
			want:    []byte("test"),
		},
		{
			name:    "returns error for invalid base64",
			content: "[client.takara]\n\tkey = not-base64\n",
			wantErr: true,
		},
		{
			name:    "returns error for empty content",
			content: " \n\t ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cephSecretValueBytesFromFileContent([]byte(tt.content))
			if tt.wantErr {
				if err == nil {
					t.Fatal("cephSecretValueBytesFromFileContent() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("cephSecretValueBytesFromFileContent() unexpected error: %v", err)
			}
			if string(got) != string(tt.want) {
				t.Fatalf("cephSecretValueBytesFromFileContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
