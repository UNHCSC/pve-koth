package sshcomm

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func MustLocalIP() string {
	var (
		output []byte
		err    error
	)

	if output, err = exec.Command("hostname", "-I").Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	var interfaces []net.Interface

	if interfaces, err = net.Interfaces(); err != nil {
		panic(err)
	}

	for _, iface := range interfaces {
		var addrs []net.Addr
		if addrs, err = iface.Addrs(); err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return strings.TrimSpace(ipNet.IP.String())
			}
		}
	}

	panic("no valid IP address found")
}

func PingHost(ipAddress string) bool {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", "2000", ipAddress)
	default:
		cmd = exec.Command("ping", "-c", "1", "-W", "2", ipAddress)
	}

	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

func WaitOnline(ipAddress string) error {
	start := time.Now()

	for time.Since(start) < 3*time.Minute {
		if PingHost(ipAddress) {
			return nil
		}

		time.Sleep(time.Second * 3)
	}

	return fmt.Errorf("host not available on %s within timeout", ipAddress)
}
