package ssh

import "testing"

func TestFirstUsableIPv4PrefersNonLoopbackIPv4(t *testing.T) {
	input := "127.0.0.1 ::1 10.255.158.187 fd0e:f5b4:57c7::1 fd0e:f5b4:57c7::2"

	if got := firstUsableIPv4(input); got != "10.255.158.187" {
		t.Fatalf("expected first usable IPv4, got %q", got)
	}
}

func TestFirstUsableIPv4ReturnsEmptyWithoutUsableIPv4(t *testing.T) {
	input := "127.0.0.1 ::1 fd0e:f5b4:57c7::1"

	if got := firstUsableIPv4(input); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}
