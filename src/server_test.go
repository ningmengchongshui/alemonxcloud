package cloud

import "testing"

func TestListenAddress(t *testing.T) {
	for input, expected := range map[string]string{
		"":               ":8082",
		"8082":           ":8082",
		" :8082 ":        ":8082",
		"127.0.0.1:8082": "127.0.0.1:8082",
		"[::1]:8082":     "[::1]:8082",
	} {
		if actual := listenAddress(input); actual != expected {
			t.Errorf("listenAddress(%q) = %q, want %q", input, actual, expected)
		}
	}
}
