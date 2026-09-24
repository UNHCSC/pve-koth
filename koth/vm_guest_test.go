package koth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWindowsComputerName(t *testing.T) {
	name := windowsComputerName(12, "windows-workstation")
	assert.LessOrEqual(t, len(name), 15)
	assert.True(t, strings.HasPrefix(name, "KOTH-T12-"))
}

func TestGuestResourceNameUsesCompetitionPrefix(t *testing.T) {
	assert.Equal(t, "koth-teamact0924-team-5-windows", guestResourceName("koth-teamact0924", 5, "windows"))
}

func TestPowerShellScriptInvocationEscapesEnvironment(t *testing.T) {
	script := powershellScriptInvocation("https://koth.local/script.ps1", "token'value", map[string]any{
		"KOTH_HOSTNAME": "host'value",
	})
	assert.Contains(t, script, "$env:KOTH_HOSTNAME = 'host''value'")
	assert.Contains(t, script, "System.Net.Cookie('Authorization', 'token''value'")
	assert.Contains(t, script, "ServerCertificateValidationCallback")
	assert.Contains(t, script, "& $scriptPath")
	assert.NotContains(t, script, "& powershell.exe")
}

func TestReadablePowerShellOutputDecodesCLIXMLErrors(t *testing.T) {
	raw := "#< CLIXML\r\n<Objs xmlns=\"http://schemas.microsoft.com/powershell/2004/04\"><S S=\"Error\">Install failed_x000D__x000A_At C:\\setup.ps1:42 char:3</S></Objs>"

	assert.Equal(t, "Install failed\nAt C:\\setup.ps1:42 char:3", readablePowerShellOutput(raw))
}

func TestReadablePowerShellOutputLeavesPlainTextReadable(t *testing.T) {
	assert.Equal(t, "plain failure", readablePowerShellOutput("  plain failure\r\n"))
}
