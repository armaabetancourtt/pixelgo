package main

import "testing"

func TestValidateTargetDefaultsToLoopback(t *testing.T) {
    for _, address := range []string{"http://localhost:8080/health", "http://127.0.0.1:8080/ready", "http://[::1]:8080/health"} {
        if err := validateTarget(address, false); err != nil { t.Fatalf("%s: %v", address, err) }
    }
}

func TestValidateTargetRequiresRemoteOptIn(t *testing.T) {
    if err := validateTarget("https://example.org/health", false); err == nil {
        t.Fatal("remote hosts must require explicit opt-in")
    }
    if err := validateTarget("https://example.org/health", true); err != nil { t.Fatal(err) }
    for _, address := range []string{"file:///etc/passwd", "http://user:pass@localhost:8080/health", "not a url"} {
        if err := validateTarget(address, true); err == nil { t.Fatalf("expected rejection for %s", address) }
    }
}
