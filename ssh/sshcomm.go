package ssh

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHConnection struct {
	client   *ssh.Client
	session  *ssh.Session
	username string
	host     string
	port     int
}

// If you've run a command and want to run another, you need to reset the session
func (conn *SSHConnection) Reset() (err error) {
	if err = conn.session.Close(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "eof") {
		return
	}

	conn.session, err = conn.client.NewSession()
	if err != nil {
		err = fmt.Errorf("failed to create new SSH session: %v", err)
	}

	return
}

func (conn *SSHConnection) Close() (err error) {
	if err = conn.session.Close(); err != nil {
		return
	}

	err = conn.client.Close()
	return
}

func (conn *SSHConnection) Send(command string) (err error) {
	err = conn.session.Run(command)
	return
}

func (conn *SSHConnection) SendWithOutput(command string) (status int, output []byte, err error) {
	if output, err = conn.session.CombinedOutput(command); err != nil {
		var (
			exitErr *ssh.ExitError
			ok      bool
		)

		if exitErr, ok = err.(*ssh.ExitError); ok {
			status = exitErr.ExitStatus()
			err = nil
			return
		}

		status = -1
		return
	}

	status = 0
	return
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

func WithPassword(password string) ssh.AuthMethod {
	return ssh.Password(password)
}

func WithKeyboardInteractivePassword(password string) ssh.AuthMethod {
	return ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		var answers = make([]string, len(questions))
		for i := range answers {
			answers[i] = password
		}
		return answers, nil
	})
}

func Connect(username, host string, port int, authMethods ...ssh.AuthMethod) (conn *SSHConnection, err error) {
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods provided")
	}

	conn = &SSHConnection{
		username: username,
		host:     host,
		port:     port,
	}

	if conn.client, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
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

func ConnectOnceReadyWithRetry(username, host string, port int, retries int, authMethods ...ssh.AuthMethod) (conn *SSHConnection, err error) {
	if err = WaitOnline(host); err != nil {
		return
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods provided")
	}

	for i := range retries {
		err = nil
		if conn, err = Connect(username, host, port, authMethods...); err == nil {
			return
		}

		time.Sleep((time.Duration(i) + 1) * time.Second)
	}

	return
}
