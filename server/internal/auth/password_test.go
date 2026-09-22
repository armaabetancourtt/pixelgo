package auth

import "testing"

func TestPasswordHashUsesRandomSaltAndVerifies(t *testing.T) {
	first, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected unique salted password hashes")
	}

	ok, err := VerifyPassword(first, "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected valid password")
	}

	ok, err = VerifyPassword(first, "wrong-password-value")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected wrong password to be rejected")
	}
}
