package koth

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/luthermonson/go-proxmox"
)

var powerShellEncodedCharacter = regexp.MustCompile(`(?i)_x([0-9a-f]{4})_`)

func waitForWindowsReady(api *proxmoxAPI.ProxmoxAPI, vm *proxmox.VirtualMachine, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	command := []string{
		"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-ItemProperty -LiteralPath "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State").ImageState`,
	}
	for time.Now().Before(deadline) {
		if err := api.WaitForVirtualMachineAgent(vm, 2*time.Minute, 10*time.Second); err == nil {
			result, execErr := api.ExecuteVirtualMachineCommand(vm, command, "", time.Minute)
			if execErr == nil && result.ExitCode == 0 && strings.Contains(result.Stdout, "IMAGE_STATE_COMPLETE") {
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("Windows OOBE did not complete within %s", timeout)
}

func bootstrapWindowsVM(api *proxmoxAPI.ProxmoxAPI, vm *proxmox.VirtualMachine, plan *guestPlan) error {
	if err := waitForWindowsReady(api, vm, 30*time.Minute); err != nil {
		return err
	}
	script := fmt.Sprintf(`$ErrorActionPreference = "Stop"
$adapter = Get-NetAdapter | Where-Object { $_.HardwareInterface -and $_.Status -ne "Disabled" } | Sort-Object ifIndex | Select-Object -First 1
if ($null -eq $adapter) { throw "No usable network adapter found" }
Get-NetIPAddress -InterfaceIndex $adapter.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.PrefixOrigin -ne "WellKnown" } | Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue
Get-NetRoute -InterfaceIndex $adapter.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.DestinationPrefix -eq "0.0.0.0/0" } | Remove-NetRoute -Confirm:$false -ErrorAction SilentlyContinue
New-NetIPAddress -InterfaceIndex $adapter.ifIndex -IPAddress %s -PrefixLength %d -DefaultGateway %s | Out-Null
Set-DnsClientServerAddress -InterfaceIndex $adapter.ifIndex -ServerAddresses %s
if ($env:COMPUTERNAME -ne %s) { Rename-Computer -NewName %s -Force }
`, powershellQuote(plan.ipAddress), config.Config.Network.ContainerCIDR, powershellQuote(config.Config.Network.ContainerGateway), powershellQuote(config.Config.Network.ContainerNameserver), powershellQuote(plan.hostname), powershellQuote(plan.hostname))

	result, err := api.ExecuteVirtualMachineCommand(vm, powershellCommand(script), "", 3*time.Minute)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("Windows network bootstrap exited %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	if err = api.RebootVirtualMachine(vm); err != nil {
		return err
	}
	return waitForWindowsReady(api, vm, 15*time.Minute)
}

func powershellCommand(script string) []string {
	runes := utf16.Encode([]rune(script))
	bytes := make([]byte, len(runes)*2)
	for i, value := range runes {
		bytes[i*2] = byte(value)
		bytes[i*2+1] = byte(value >> 8)
	}
	return []string{"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(bytes)}
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func readablePowerShellOutput(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
	if value == "" || !strings.Contains(value, "#< CLIXML") {
		return value
	}

	xmlStart := strings.Index(value, "<Objs")
	if xmlStart < 0 {
		return normalizePowerShellText(strings.TrimSpace(strings.TrimPrefix(value, "#< CLIXML")))
	}
	decoder := xml.NewDecoder(strings.NewReader(value[xmlStart:]))
	var messages []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return normalizePowerShellText(strings.TrimSpace(strings.TrimPrefix(value, "#< CLIXML")))
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "S" {
			continue
		}
		isError := false
		for _, attr := range start.Attr {
			if attr.Name.Local == "S" && strings.EqualFold(attr.Value, "Error") {
				isError = true
				break
			}
		}
		if !isError {
			continue
		}
		var message string
		if err = decoder.DecodeElement(&message, &start); err == nil {
			message = normalizePowerShellText(message)
			if message != "" {
				messages = append(messages, message)
			}
		}
	}
	if len(messages) == 0 {
		return normalizePowerShellText(strings.TrimSpace(strings.TrimPrefix(value, "#< CLIXML")))
	}
	return strings.Join(messages, "\n")
}

func normalizePowerShellText(value string) string {
	value = powerShellEncodedCharacter.ReplaceAllStringFunc(value, func(encoded string) string {
		code, err := strconv.ParseInt(encoded[2:6], 16, 32)
		if err != nil {
			return encoded
		}
		return string(rune(code))
	})
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func windowsComputerName(teamNumber int, guestName string) string {
	name := fmt.Sprintf("KOTH-T%d-%s", teamNumber, strings.ToUpper(sanitizeContainerName(guestName)))
	if len(name) > 15 {
		name = name[:15]
	}
	return strings.TrimRight(name, "-")
}

func guestResourceName(hostnamePrefix string, teamNumber int, guestName string) string {
	return fmt.Sprintf("%s-team-%d-%s", hostnamePrefix, teamNumber, guestName)
}

func powershellScriptInvocation(scriptURL, token string, envs map[string]any) string {
	var keys []string
	for key := range envs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString("$ErrorActionPreference = 'Stop'\n")
	builder.WriteString("[System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }\n")
	for _, key := range keys {
		fmt.Fprintf(&builder, "$env:%s = %s\n", key, powershellQuote(fmt.Sprint(envs[key])))
	}
	builder.WriteString("$scriptPath = Join-Path $env:TEMP ('pve-koth-' + [guid]::NewGuid().ToString() + '.ps1')\n")
	fmt.Fprintf(&builder, "$scriptUri = [uri]%s\n", powershellQuote(scriptURL))
	builder.WriteString("$webSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession\n")
	fmt.Fprintf(&builder, "$webSession.Cookies.Add((New-Object System.Net.Cookie('Authorization', %s, '/', $scriptUri.Host)))\n", powershellQuote(token))
	builder.WriteString("Invoke-WebRequest -UseBasicParsing -Uri $scriptUri -WebSession $webSession -OutFile $scriptPath\n")
	builder.WriteString("try {\n")
	builder.WriteString("  & $scriptPath\n")
	builder.WriteString("} finally { Remove-Item -LiteralPath $scriptPath -Force -ErrorAction SilentlyContinue }\n")
	return builder.String()
}
