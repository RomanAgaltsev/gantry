package composessh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// startTestSSHServer starts an in-process SSH server on 127.0.0.1:0 that runs each
// exec request through handle, authorizing only the given client public key. It returns
// the listen address and the host's known_hosts line.
//
// It is implemented with golang.org/x/crypto/ssh directly (already a dependency) rather
// than pulling in a separate SSH-server library, so go.mod stays unchanged.
func startTestSSHServer(tb testing.TB, authorized ssh.PublicKey, handle func(cmd string, stdin []byte) (string, int)) (addr, knownHosts string) {
	tb.Helper()
	hostSigner, hostPub := genHostSigner(tb)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed (test cleanup)
			}
			go serveSSHConn(conn, hostSigner, authorized, handle)
		}
	}()

	kh := knownhosts.Line([]string{ln.Addr().String()}, hostPub)
	return ln.Addr().String(), kh
}

func serveSSHConn(conn net.Conn, hostSigner ssh.Signer, authorized ssh.PublicKey, handle func(cmd string, stdin []byte) (string, int)) {
	defer func() { _ = conn.Close() }()
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytesEqual(key.Marshal(), authorized.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown public key")
		},
	}
	cfg.AddHostKey(hostSigner)

	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "only session channels")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		serveSession(ch, chReqs, handle)
	}
}

func serveSession(ch ssh.Channel, reqs <-chan *ssh.Request, handle func(cmd string, stdin []byte) (string, int)) {
	defer func() { _ = ch.Close() }()

	var cmd string
	for req := range reqs {
		if req.WantReply {
			_ = req.Reply(req.Type == "exec", nil)
		}
		if req.Type != "exec" {
			continue
		}
		var msg struct{ Command string }
		if err := ssh.Unmarshal(req.Payload, &msg); err != nil {
			return
		}
		cmd = msg.Command
		break
	}
	if cmd == "" {
		return
	}

	// The x/crypto/ssh client sends EOF on the stdin stream even when Stdin is nil,
	// so ReadAll returns once the client has flushed (or closed) its input.
	stdin, _ := io.ReadAll(ch)
	out, code := handle(cmd, stdin)

	_, _ = ch.Write([]byte(out))

	status := make([]byte, 4)
	binary.BigEndian.PutUint32(status, uint32(code))
	_, _ = ch.SendRequest("exit-status", false, status)
}

func TestSSHRunner_ExecAndStdin(t *testing.T) {
	clientKey, clientPub := genClientKey(t)
	addr, kh := startTestSSHServer(t, clientPub, func(cmd string, stdin []byte) (string, int) {
		if cmd == "cat" {
			return string(stdin), 0
		}
		return "ran:" + cmd, 0
	})
	r, err := NewSSHRunner(addr, "tester", clientKey, kh)
	require.NoError(t, err)

	out, err := r.Run(context.Background(), "echo hi", nil)
	require.NoError(t, err)
	require.Equal(t, "ran:echo hi", out)

	out, err = r.Run(context.Background(), "cat", []byte("payload"))
	require.NoError(t, err)
	require.Equal(t, "payload", out)
}

func TestSSHRunner_ContextCancellationUnblocks(t *testing.T) {
	clientKey, clientPub := genClientKey(t)
	addr, kh := startTestSSHServer(t, clientPub, func(string, []byte) (string, int) {
		time.Sleep(2 * time.Second) // model a wedged remote command
		return "", 0
	})
	r, err := NewSSHRunner(addr, "tester", clientKey, kh)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = r.Run(ctx, "sleep", nil)
	require.Error(t, err)                           // ctx cancelled
	require.Less(t, time.Since(start), time.Second) // unblocked promptly, not after 2s
}

// TestSSHRunner_CloseReleasesConnection asserts the C3 fix: after Close, a subsequent Run redials.
func TestSSHRunner_CloseReleasesConnection(t *testing.T) {
	clientKey, clientPub := genClientKey(t)
	addr, kh := startTestSSHServer(t, clientPub, func(string, []byte) (string, int) { return "ok", 0 })
	r, err := NewSSHRunner(addr, "tester", clientKey, kh)
	require.NoError(t, err)

	_, err = r.Run(context.Background(), "x", nil)
	require.NoError(t, err)

	closer, ok := r.(Closer)
	require.True(t, ok, "runner must implement Closer")
	require.NoError(t, closer.Close())

	_, err = r.Run(context.Background(), "y", nil) // redials after Close
	require.NoError(t, err)
}

// genHostSigner generates an ephemeral ed25519 host key, returning the signer and its
// public key (in SSH wire form, for known_hosts).
func genHostSigner(tb testing.TB) (ssh.Signer, ssh.PublicKey) {
	tb.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(tb, err)
	return signer, signer.PublicKey()
}

// genClientKey generates an ed25519 keypair for a connecting client, returning the
// PEM-encoded private key and the matching SSH public key for server-side authorization.
func genClientKey(tb testing.TB) (privKeyPEM string, pub ssh.PublicKey) {
	tb.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(tb, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(tb, err)
	return string(pem.EncodeToMemory(block)), signer.PublicKey()
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
