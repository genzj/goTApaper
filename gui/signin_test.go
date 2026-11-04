package gui

import (
	"strings"
	"testing"
)

func TestSignInScriptChecksTheOriginFirst(t *testing.T) {
	script, err := signInScript("http://127.0.0.1:8080/ui/", "secret")
	if err != nil {
		t.Fatalf("signInScript: %v", err)
	}

	guard := `if (window.location.origin !== "http://127.0.0.1:8080") return;`
	if !strings.Contains(script, guard) {
		t.Fatalf("the script does not check the origin:\n%s", script)
	}
	// The token must only appear after the guard, so a page from any other
	// origin returns before it is ever assigned.
	if strings.Index(script, `"secret"`) < strings.Index(script, guard) {
		t.Fatal("the token appears before the origin check")
	}
}

func TestSignInScriptEscapesItsValues(t *testing.T) {
	script, err := signInScript("http://127.0.0.1:8080/ui/", `"); alert(1); ("`)
	if err != nil {
		t.Fatalf("signInScript: %v", err)
	}
	if strings.Contains(script, `"); alert(1); ("`) {
		t.Fatal("a value broke out of its string literal")
	}
}
