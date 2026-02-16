package log

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  LogLevel
	}{
		{name: "debug lower", input: "debug", want: LevelDebug},
		{name: "info upper", input: "INFO", want: LevelInfo},
		{name: "warn mixed", input: "WaRn", want: LevelWarn},
		{name: "error", input: "error", want: LevelError},
		{name: "fatal", input: "fatal", want: LevelFatal},
		{name: "trim spaces", input: "  debug  ", want: LevelDebug},
		{name: "unknown fallback", input: "verbose", want: LevelInfo},
		{name: "empty fallback", input: "", want: LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseLevel(tt.input); got != tt.want {
				t.Fatalf("ParseLevel(%q)=%v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
