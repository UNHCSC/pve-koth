package tests

import (
	"fmt"
	"testing"

	"github.com/UNHCSC/pve-koth/sshcomm"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/ssh"
)

func helperSpinUpWebServer() {
	var app = fiber.New(fiber.Config{})

	app.Get("/test_script", func(c *fiber.Ctx) error {
		var authCookie = c.Cookies("Authorization", "")
		if authCookie != "test_token" {
			return c.SendString("echo 'Unauthorized'; exit 1")
		}

		return c.SendString("echo Hello, World!")
	})

	app.Get("/test_script_env", func(c *fiber.Ctx) error {
		var authCookie = c.Cookies("Authorization", "")
		if authCookie != "test_token" {
			return c.SendString("echo 'Unauthorized'; exit 1")
		}

		return c.SendString("echo $KOTH_VAR")
	})

	app.Listen(":8080")
}

func expectCommand(t *testing.T, conn *sshcomm.SSHConnection, cmd string, expectedExitCode int, expectedOutput string) {
	var (
		exitCode int
		output   string
		err      error
	)

	if exitCode, output, err = conn.SendWithOutput(cmd); err != nil {
		t.Fatalf("failed to send command via SSH: %v\n", err)
		return
	}

	if exitCode != expectedExitCode {
		t.Errorf("expected exit code %d, got %d\n", expectedExitCode, exitCode)
	}

	if output != expectedOutput {
		t.Errorf("expected output '%s', got '%s'\n", expectedOutput, output)
	}
}

func setup(t *testing.T) *sshcomm.SSHConnection {
	var (
		conn *sshcomm.SSHConnection
		err  error
	)

	if conn, err = sshcomm.Connect("root", "10.224.0.1", 22, ssh.Password("password")); err != nil {
		t.Fatalf("failed to connect via SSH: %v\n", err)
		return nil
	}

	go helperSpinUpWebServer()

	return conn
}

func TestSSHHelloWorld(t *testing.T) {
	var conn *sshcomm.SSHConnection = setup(t)
	defer conn.Close()

	expectCommand(t, conn, "echo Hello, World!", 0, "Hello, World!\n")
}

func TestSSHExitCode(t *testing.T) {
	var conn *sshcomm.SSHConnection = setup(t)
	defer conn.Close()

	expectCommand(t, conn, "exit 42", 42, "")
}

func TestSSHLoadAndRunScriptGood(t *testing.T) {
	var conn *sshcomm.SSHConnection = setup(t)
	defer conn.Close()

	expectCommand(t, conn, sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/test_script", sshcomm.MustLocalIP(), 8080), "test_token", map[string]any{}), 0, "Hello, World!\n")
}

func TestSSHLoadAndRunScriptBadAuth(t *testing.T) {
	var conn *sshcomm.SSHConnection = setup(t)
	defer conn.Close()

	expectCommand(t, conn, sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/test_script", sshcomm.MustLocalIP(), 8080), "wrong_token", map[string]any{}), 1, "Unauthorized\n")
}

func TestSSHLoadAndRunScriptWithEnv(t *testing.T) {
	var conn *sshcomm.SSHConnection = setup(t)
	defer conn.Close()

	expectCommand(t, conn, sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/test_script_env", sshcomm.MustLocalIP(), 8080), "test_token", map[string]any{
		"KOTH_VAR": "TestValue",
	}), 0, "TestValue\n")
}