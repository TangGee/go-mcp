package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/MegaGrindStone/go-mcp"
)

func TestMustString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    mcp.MustString
		wantErr bool
	}{
		{
			name:    "string input",
			input:   `"test123"`,
			want:    mcp.MustString("test123"),
			wantErr: false,
		},
		{
			name:    "integer input",
			input:   `42`,
			want:    mcp.MustString("42"),
			wantErr: false,
		},
		{
			name:    "float input",
			input:   `42.0`,
			want:    mcp.MustString("42"),
			wantErr: false,
		},
		{
			name:    "invalid type",
			input:   `{"key": "value"}`,
			want:    mcp.MustString(""),
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `invalid`,
			want:    mcp.MustString(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got mcp.MustString
			err := json.Unmarshal([]byte(tt.input), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("MustString.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("MustString.UnmarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMustString_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   mcp.MustString
		want    string
		wantErr bool
	}{
		{
			name:    "string value",
			input:   mcp.MustString("test123"),
			want:    `"test123"`,
			wantErr: false,
		},
		{
			name:    "numeric string",
			input:   mcp.MustString("42"),
			want:    `"42"`,
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   mcp.MustString(""),
			want:    `""`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MustString.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("MustString.MarshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestMustString_RoundTrip(t *testing.T) {
	original := mcp.MustString("test123")

	// Marshal
	marshaled, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var unmarshaled mcp.MustString
	err = json.Unmarshal(marshaled, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Compare
	if original != unmarshaled {
		t.Errorf("Round trip failed: got %v, want %v", unmarshaled, original)
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		name     string
		level    mcp.LogLevel
		expected string
	}{
		{
			name:     "Debug level",
			level:    mcp.LogLevelDebug,
			expected: "debug",
		},
		{
			name:     "Info level",
			level:    mcp.LogLevelInfo,
			expected: "info",
		},
		{
			name:     "Notice level",
			level:    mcp.LogLevelNotice,
			expected: "notice",
		},
		{
			name:     "Warning level",
			level:    mcp.LogLevelWarning,
			expected: "warning",
		},
		{
			name:     "Error level",
			level:    mcp.LogLevelError,
			expected: "error",
		},
		{
			name:     "Critical level",
			level:    mcp.LogLevelCritical,
			expected: "critical",
		},
		{
			name:     "Alert level",
			level:    mcp.LogLevelAlert,
			expected: "alert",
		},
		{
			name:     "Emergency level",
			level:    mcp.LogLevelEmergency,
			expected: "emergency",
		},
		{
			name:     "Unknown level",
			level:    mcp.LogLevel(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
