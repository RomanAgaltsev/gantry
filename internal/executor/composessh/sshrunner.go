package composessh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshRunner struct {
	addr   string
	config *ssh.ClientConfig
}

// NewSSHRunner builds a Runner that executes commands over SSH using a private key.
// knownHosts must be the contents of a known_hosts file; empty is rejected.
func NewSSHRunner(addr, user, privateKey, knownHosts string) (Runner, error) {
	if knownHosts == "" {
		return nil, fmt.Errorf("known_hosts required (no silent host-key TOFU)")
	}
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}
	hostKeyCB, err := knownHostsCallback(knownHosts)
	if err != nil {
		return nil, err
	}
	return &sshRunner{
		addr: addr,
		config: &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: hostKeyCB,
		},
	}, nil
}

func (r *sshRunner) Run(ctx context.Context, cmd string, stdin []byte) (string, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", withDefaultPort(r.addr))
	if err != nil {
		return "", err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, withDefaultPort(r.addr), r.config)
	if err != nil {
		return "", err
	}
	client := ssh.NewClient(c, chans, reqs)
	defer func() { _ = client.Close() }()

	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close() }()
	if stdin != nil {
		sess.Stdin = bytes.NewReader(stdin)
	}
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

func withDefaultPort(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, "22")
	}
	return addr
}

func knownHostsCallback(contents string) (ssh.HostKeyCallback, error) {
	f, err := os.CreateTemp("", "gantry-known-hosts-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(contents); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return knownhosts.New(f.Name())
}
