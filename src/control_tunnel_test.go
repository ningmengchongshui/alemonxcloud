package cloud

import "testing"

func TestValidControlTarget(t *testing.T) {
	for _, value := range []string{"http://127.0.0.1:3000", "http://localhost:8080"} {
		if _, err := validControlTarget(value); err != nil {
			t.Fatalf("%s should be accepted: %v", value, err)
		}
	}
	for _, value := range []string{"https://127.0.0.1:3000", "http://10.0.0.1:80", "http://localhost", "http://localhost:3000/path"} {
		if _, err := validControlTarget(value); err == nil {
			t.Fatalf("%s should be rejected", value)
		}
	}
}

func TestControlCredentialHashStable(t *testing.T) {
	if controlHash("credential") != controlHash("credential") || controlHash("credential") == controlHash("other") {
		t.Fatal("credential hashes must be deterministic and distinct")
	}
}
