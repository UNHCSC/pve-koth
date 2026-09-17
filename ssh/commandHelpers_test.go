package ssh

import (
	"strings"
	"testing"
)

func TestLoadAndRunScriptUsesTempFileAndNonEmptyCheck(t *testing.T) {
	command := LoadAndRunScript("https://10.255.158.187/api/setup.sh", "token", map[string]any{
		"KOTH_PUBLIC_FOLDER": "https://10.255.158.187/public",
	})

	if strings.Contains(command, "| bash") || strings.Contains(command, "|bash") || strings.Contains(command, "-qO-") {
		t.Fatalf("expected command not to pipe downloader output to bash: %s", command)
	}

	for _, want := range []string{
		"tmp_script=$(mktemp)",
		"trap 'rm -f \"$tmp_script\"' EXIT",
		"-o \"$tmp_script\"",
		"-qO \"$tmp_script\"",
		"test -s \"$tmp_script\"",
		"bash \"$tmp_script\"",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected command to contain %q: %s", want, command)
		}
	}
}

func TestLoadAndRunScriptShellQuotesValues(t *testing.T) {
	command := LoadAndRunScript("https://host/path with spaces/setup's.sh", "tok'en value", map[string]any{
		"KOTH_PUBLIC_FOLDER": "https://host/public folder/it's here",
	})

	for _, want := range []string{
		"KOTH_PUBLIC_FOLDER='https://host/public folder/it'\"'\"'s here'",
		"'Cookie: Authorization=tok'\"'\"'en value'",
		"'https://host/path with spaces/setup'\"'\"'s.sh'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected command to contain %q: %s", want, command)
		}
	}
}
