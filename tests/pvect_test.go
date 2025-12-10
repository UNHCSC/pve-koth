package tests

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/ssh"
	"github.com/stretchr/testify/assert"
)

func TestBasicLifecycle(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	var (
		err error
		env *ProxmoxTestingEnvironment
	)

	if env, err = proxmoxEnvironmentSetup(t, false, false, []string{"koth-test-basic-lifecycle"}, false); err != nil {
		t.Fatalf("failed to setup proxmox testing environment: %v", err)
	}

	defer env.Cleanup(t)
}

func TestSSHBasicExecution(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	var (
		err       error
		env       *ProxmoxTestingEnvironment
		cmdStatus int
		output    string
	)

	if env, err = proxmoxEnvironmentSetup(t, true, false, []string{"koth-test-ssh-exec"}, false); err != nil {
		t.Fatalf("failed to setup proxmox testing environment: %v", err)
	}

	defer env.Cleanup(t)

	t.Logf("Connecting to %s (%s) via SSH...", env.containerHostnames[0], env.ips[0])
	if _, err = env.ConnectSSH(0); err != nil {
		t.Fatalf("failed to connect via SSH to container: %v", err)
	}

	t.Logf("SSH connection to %s (%s) established.", env.containerHostnames[0], env.ips[0])
	if cmdStatus, output, err = env.ExecuteOn(0, "hostname"); err != nil {
		t.Fatalf("failed to execute command via SSH: %v", err)
	}

	assert.Equal(t, 0, cmdStatus, "expected command to succeed")
	assert.Equal(t, env.containerHostnames[0], strings.TrimSpace(output), "expected hostname to match container hostname")
}

func TestSSHFetchExecution(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	var (
		err       error
		env       *ProxmoxTestingEnvironment
		cmdStatus int
		output    string
	)

	if env, err = proxmoxEnvironmentSetup(t, true, true, []string{"koth-test-ssh-fetch"}, false); err != nil {
		t.Fatalf("failed to setup proxmox testing environment: %v", err)
	}

	defer env.Cleanup(t)

	t.Logf("Connecting to %s (%s) via SSH...", env.containerHostnames[0], env.ips[0])
	if _, err = env.ConnectSSH(0); err != nil {
		t.Fatalf("failed to connect via SSH to container: %v", err)
	}

	t.Logf("SSH connection to %s (%s) established.", env.containerHostnames[0], env.ips[0])
	var commandEnvs map[string]any

	if commandEnvs, err = env.EnvsFor(0, map[string]any{
		"KOTH_TEST_ENV_VAR": "Hello from KOTH!",
	}); err != nil {
		t.Fatalf("failed to get environment variables for command execution: %v", err)
	}

	if cmdStatus, output, err = env.ExecuteOn(0, ssh.LoadAndRunScript(fmt.Sprintf("http://%s/koth-test-ssh-fetch.sh", net.JoinHostPort(ssh.MustLocalIP(), env.webPort)), env.reqToken(), commandEnvs)); err != nil {
		t.Fatalf("failed to execute command via SSH: %v", err)
	}

	assert.Equal(t, 123, cmdStatus, "expected command to exit with status 123")
	assert.Equal(t, commandEnvs["KOTH_TEST_ENV_VAR"], strings.TrimSpace(output), "expected output to match environment variable")
}

func TestSSHReset(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	var (
		err       error
		env       *ProxmoxTestingEnvironment
		cmdStatus int
		output    string
	)

	if env, err = proxmoxEnvironmentSetup(t, true, true, []string{"koth-test-ssh-fetch"}, false); err != nil {
		t.Fatalf("failed to setup proxmox testing environment: %v", err)
	}

	defer env.Cleanup(t)

	t.Logf("Connecting to %s (%s) via SSH...", env.containerHostnames[0], env.ips[0])
	if _, err = env.ConnectSSH(0); err != nil {
		t.Fatalf("failed to connect via SSH to container: %v", err)
	}

	t.Logf("SSH connection to %s (%s) established.", env.containerHostnames[0], env.ips[0])
	if cmdStatus, output, err = env.ExecuteOn(0, "hostname"); err != nil {
		t.Fatalf("failed to execute command via SSH: %v", err)
	}

	assert.Equal(t, 0, cmdStatus, "expected command to succeed")
	assert.Equal(t, env.containerHostnames[0], strings.TrimSpace(output), "expected hostname to match container hostname")

	if err = env.ResetSSH(0); err != nil {
		t.Fatalf("failed to reset SSH session: %v", err)
	}

	var commandEnvs map[string]any

	if commandEnvs, err = env.EnvsFor(0, map[string]any{
		"KOTH_TEST_ENV_VAR": "Hello from KOTH!",
	}); err != nil {
		t.Fatalf("failed to get environment variables for command execution: %v", err)
	}

	if cmdStatus, output, err = env.ExecuteOn(0, ssh.LoadAndRunScript(fmt.Sprintf("http://%s/koth-test-ssh-fetch.sh", net.JoinHostPort(ssh.MustLocalIP(), env.webPort)), env.reqToken(), commandEnvs)); err != nil {
		t.Fatalf("failed to execute command via SSH: %v", err)
	}

	assert.Equal(t, 123, cmdStatus, "expected command to exit with status 123")
	assert.Equal(t, commandEnvs["KOTH_TEST_ENV_VAR"], strings.TrimSpace(output), "expected output to match environment variable")
}

func TestSSHEnvs(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	var (
		err       error
		env       *ProxmoxTestingEnvironment
		cmdStatus int
		output    string
	)

	if env, err = proxmoxEnvironmentSetup(t, true, true, []string{"koth-test-envs"}, false); err != nil {
		t.Fatalf("failed to setup proxmox testing environment: %v", err)
	}

	defer env.Cleanup(t)

	t.Logf("Connecting to %s (%s) via SSH...", env.containerHostnames[0], env.ips[0])
	if _, err = env.ConnectSSH(0); err != nil {
		t.Fatalf("failed to connect via SSH to container: %v", err)
	}

	t.Logf("SSH connection to %s (%s) established.", env.containerHostnames[0], env.ips[0])
	var (
		commandEnvs      map[string]any
		artifactContents string
	)

	if commandEnvs, err = env.EnvsFor(0); err != nil {
		t.Fatalf("failed to get environment variables for command execution: %v", err)
	}

	if artifactContents, err = readPublicFileContents("artifact.txt"); err != nil {
		t.Fatalf("failed to read artifact contents: %v", err)
	}

	if cmdStatus, output, err = env.ExecuteOn(0, ssh.LoadAndRunScript(fmt.Sprintf("http://%s/koth-test-envs.sh", net.JoinHostPort(ssh.MustLocalIP(), env.webPort)), env.reqToken(), commandEnvs)); err != nil {
		t.Fatalf("failed to execute command via SSH: %v", err)
	}

	assert.Equal(t, 0, cmdStatus, "expected command to succeed")
	assert.Equal(t, strings.TrimSpace(artifactContents), strings.TrimSpace(output), "expected output to match artifact contents")
}

func TestContainerBulkOperations(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	var (
		err       error
		env       *ProxmoxTestingEnvironment
		cmdStatus int
		output    string
	)

	if env, err = proxmoxEnvironmentSetup(t, true, false, genHostnamesHelper("koth-test-bulk-", 32), true); err != nil {
		t.Fatalf("failed to setup proxmox testing environment: %v", err)
	}

	defer env.Cleanup(t)

	if err = env.SSHAll(); err != nil {
		t.Fatalf("failed to establish SSH connections to all containers: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(len(env.containerHostnames))

	for i := range env.containerHostnames {
		go func(idx int) {
			defer wg.Done()

			t.Logf("Connecting to %s (%s) via SSH...", env.containerHostnames[idx], env.ips[idx])
			if _, err = env.ConnectSSH(idx); err != nil {
				t.Errorf("failed to connect via SSH to container %s: %v", env.containerHostnames[idx], err)
				return
			}

			t.Logf("SSH connection to %s (%s) established.", env.containerHostnames[idx], env.ips[idx])
			if cmdStatus, output, err = env.ExecuteOn(idx, "hostname"); err != nil {
				t.Errorf("failed to execute command via SSH on container %s: %v", env.containerHostnames[idx], err)
				return
			}

			assert.Equal(t, 0, cmdStatus, "expected command to succeed")
			assert.Equal(t, env.containerHostnames[idx], strings.TrimSpace(output), "expected hostname to match container hostname")
		}(i)
	}
}
