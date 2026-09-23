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
