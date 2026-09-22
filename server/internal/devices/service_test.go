package devices

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterEnforcesContractLimits(t *testing.T) {
	service := NewService(NewMemoryRepository())

	tests := []struct {
		name string
		in   RegisterInput
	}{
		{
			name: "empty name",
			in: RegisterInput{
				Name:     "",
				Platform: "ios",
			},
		},
		{
			name: "name longer than 120 characters",
			in: RegisterInput{
				Name:     strings.Repeat("a", 121),
				Platform: "android",
			},
		},
		{
			name: "unsupported platform",
			in: RegisterInput{
				Name:     "Device",
				Platform: "web",
			},
		},
		{
			name: "push token longer than 4096 characters",
			in: RegisterInput{
				Name:      "Device",
				Platform:  "ios",
				PushToken: strings.Repeat("t", 4097),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Register(context.Background(), test.in); err == nil {
				t.Fatal("expected invalid device")
			}
		})
	}
}

func TestRegisterCountsUnicodeCharactersNotBytes(t *testing.T) {
	service := NewService(NewMemoryRepository())

	device, err := service.Register(context.Background(), RegisterInput{
		Name:     strings.Repeat("é", 120),
		Platform: "ios",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device.Name == "" {
		t.Fatal("expected device name")
	}
}

func TestUpdatePushTokenAllowsClearAndRejectsOversize(t *testing.T) {
	service := NewService(NewMemoryRepository())
	ctx := context.Background()

	device, err := service.Register(ctx, RegisterInput{
		Name:      "Device",
		Platform:  "android",
		PushToken: "old-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.UpdatePushToken(ctx, device.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.PushToken != "" {
		t.Fatalf("expected cleared push token, got %q", updated.PushToken)
	}

	if _, err := service.UpdatePushToken(
		ctx,
		device.ID,
		strings.Repeat("t", 4097),
	); err == nil {
		t.Fatal("expected oversized push token to be rejected")
	}
}
