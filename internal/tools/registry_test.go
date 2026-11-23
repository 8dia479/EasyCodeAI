package tools

import (
	"strings"
	"testing"
)

func TestLimitedBufferCapsOutput(t *testing.T) {
	buffer := &limitedBuffer{max: 5}
	if n, err := buffer.Write([]byte("12345678")); err != nil || n != 8 {
		t.Fatalf("Write() = (%d, %v), want (8, nil)", n, err)
	}
	if got, want := buffer.String(), "12345\n[output truncated at 5 bytes]"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestLimitedBufferDoesNotMarkShortOutputAsTruncated(t *testing.T) {
	buffer := &limitedBuffer{max: 10}
	_, _ = buffer.Write([]byte("short"))
	if got := buffer.String(); strings.Contains(got, "truncated") {
		t.Fatalf("unexpected truncation marker: %q", got)
	}
}


func TestSafeGoEnvExcludesNIMCredentials(t *testing.T) {
	t.Setenv("NIM_API_KEY", "do-not-pass-this")
	t.Setenv("NIM_MODEL", "private-model")
	env := strings.Join(safeGoEnv(), "\n")
	if strings.Contains(env, "NIM_API_KEY=") || strings.Contains(env, "NIM_MODEL=") {
		t.Fatalf("NIM credentials leaked into project check environment: %q", env)
	}
	if !strings.Contains(env, "PATH=") {
		t.Fatal("safe project environment is missing PATH")
	}
}
