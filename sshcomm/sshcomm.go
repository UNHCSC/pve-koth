package sshcomm

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHConnection struct {
	client   *ssh.Client
	session  *ssh.Session
	username string
	host     string
	port     int
	auth     ssh.AuthMethod
}

func (conn *SSHConnection) Close() {
	conn.session.Close()
	conn.client.Close()
}

func (conn *SSHConnection) Send(command string) error {
	return conn.session.Run(command)
}

func (conn *SSHConnection) SendWithOutput(command string) (int, string, error) {
	output, err := conn.session.CombinedOutput(command)

	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), string(output), nil
		}

		return -1, string(output), err
	}

	return 0, string(output), nil
}

func WithPrivateKey(key []byte) ssh.AuthMethod {
	var (
		signer ssh.Signer
		err    error
	)

	if signer, err = ssh.ParsePrivateKey(key); err != nil {
		panic(err)
	}

	return ssh.PublicKeys(signer)
}

func Connect(username, host string, port int, auth ssh.AuthMethod) (conn *SSHConnection, err error) {
	conn = &SSHConnection{
		username: username,
		host:     host,
		port:     port,
		auth:     auth,
	}

	if conn.client, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}); err != nil {
		return nil, err
	}

	if conn.session, err = conn.client.NewSession(); err != nil {
		conn.client.Close()
		return nil, err
	}

	return conn, nil
}

func (conn *SSHConnection) Reset() (err error) {
	conn.session.Close()

	conn.session, err = conn.client.NewSession()
	return
}
