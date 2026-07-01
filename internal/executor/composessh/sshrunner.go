package composessh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshRunner struct {
	addr   string
	config *ssh.ClientConfig

	mu     sync.Mutex
	client *ssh.Client // dialed lazily, reused across Run calls
}

// NewSSHRunner builds a Runner that executes commands over SSH using a private key.
// knownHosts must be the contents of a known_hosts file; empty is rejected.
func NewSSHRunner(addr, user, privateKey, knownHosts string) (Runner, error) {
	if knownHosts == "" {
		return nil, errors.New("known_hosts required (no silent host-key TOFU)")
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
	client, err := r.dial(ctx)
	if err != nil {
		return "", err
	}
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = sess.Close() }() //nolint:gosec // best-effort close; the command's own error is what matters
	if stdin != nil {
		sess.Stdin = bytes.NewReader(stdin)
	}

	// CombinedOutput has no context awareness, so a hung remote command (e.g. a stuck
	// `docker compose pull`) would block forever. Run it on a goroutine and close the
	// session on cancellation, which unblocks the RPC with an error.
	type outcome struct {
		out []byte
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := sess.CombinedOutput(cmd)
		done <- outcome{out, err}
	}()

	select {
	case <-ctx.Done():
		_ = sess.Close() //nolint:gosec // best-effort close to interrupt the blocked command
		<-done           // let the goroutine observe the closed session and exit (no leak)
		return "", ctx.Err()
	case res := <-done:
		return string(res.out), res.err
	}
}

// dial returns a connected SSH client, dialing once and caching it so a deploy's
// several commands (env write, login, pull, up) share one TCP+SSH handshake. The
// client lives for the process lifetime; gantry is a short-lived CLI, so it is
// closed by process exit rather than an explicit Close.
func (r *sshRunner) dial(ctx context.Context) (*ssh.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", withDefaultPort(r.addr))
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, withDefaultPort(r.addr), r.config)
	if err != nil {
		return nil, err
	}
	r.client = ssh.NewClient(c, chans, reqs)
	return r.client, nil
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
	defer func() { _ = os.Remove(f.Name()) }() //nolint:gosec // best-effort cleanup of a temp file
	if _, err := f.WriteString(contents); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return knownhosts.New(f.Name())
}
