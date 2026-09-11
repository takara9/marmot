package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
)

// startTestSSHCommandServer starts a minimal in-process SSH server that accepts any
// public key and, for each "exec" request, either:
//   - immediately exits 0 (for any command not equal to hangCmd), or
//   - never responds (to simulate a remote command that hangs forever, e.g. apt-get
//     stuck on a dpkg lock).
//
// It returns the listener address and a teardown func.
func startTestSSHCommandServer(hangCmd string, clientPub ssh.PublicKey) (string, func()) {
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	signer, err := ssh.NewSignerFromKey(hostKey)
	Expect(err).NotTo(HaveOccurred())

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) != string(clientPub.Marshal()) {
				return nil, fmt.Errorf("unknown public key")
			}
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())

	stop := make(chan struct{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleTestSSHConn(conn, config, hangCmd, stop)
		}
	}()

	return listener.Addr().String(), func() {
		close(stop)
		_ = listener.Close()
	}
}

func handleTestSSHConn(conn net.Conn, config *ssh.ServerConfig, hangCmd string, stop <-chan struct{}) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer func() { _ = sshConn.Close() }()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer func() { _ = channel.Close() }()
			for req := range requests {
				if req.Type != "exec" {
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
					continue
				}
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
				// payload encodes the command as a length-prefixed string; for this
				// test we only need to distinguish the hang command from any other.
				cmd := string(req.Payload)
				if len(cmd) >= 4 {
					cmd = cmd[4:]
				}
				if cmd == hangCmd {
					// 意図的に応答しない(リモートコマンドがハングする状況を再現する)。
					<-stop
					return
				}
				_, _ = channel.Write([]byte("ok\n"))
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				return
			}
		}()
	}
}

var _ = Describe("kubernetesEngineNodeSSHRunner timeout", func() {
	var (
		addr     string
		teardown func()
		client   *ssh.Client
	)

	AfterEach(func() {
		if client != nil {
			_ = client.Close()
		}
		if teardown != nil {
			teardown()
		}
	})

	connect := func(hangCmd string) *ssh.Client {
		clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		clientSigner, err := ssh.NewSignerFromKey(clientKey)
		Expect(err).NotTo(HaveOccurred())

		addr, teardown = startTestSSHCommandServer(hangCmd, clientSigner.PublicKey())
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		Expect(err).NotTo(HaveOccurred())
		clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
			User:            "root",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         2 * time.Second,
		})
		Expect(err).NotTo(HaveOccurred())
		return ssh.NewClient(clientConn, chans, reqs)
	}

	It("returns a timeout error instead of blocking forever when the remote command hangs", func() {
		oldTimeout := kubernetesEngineNodeSSHCommandTimeout
		kubernetesEngineNodeSSHCommandTimeout = 300 * time.Millisecond
		defer func() { kubernetesEngineNodeSSHCommandTimeout = oldTimeout }()

		client = connect("hang-forever")
		runner := &kubernetesEngineNodeSSHRunner{client: client, resourceID: "test"}

		start := time.Now()
		err := runner.run("hang-forever", nil)
		elapsed := time.Since(start)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("timed out"))
		Expect(elapsed).To(BeNumerically("<", 2*time.Second))
	})

	It("still succeeds normally for commands that return promptly", func() {
		oldTimeout := kubernetesEngineNodeSSHCommandTimeout
		kubernetesEngineNodeSSHCommandTimeout = 5 * time.Second
		defer func() { kubernetesEngineNodeSSHCommandTimeout = oldTimeout }()

		client = connect("hang-forever")
		runner := &kubernetesEngineNodeSSHRunner{client: client, resourceID: "test"}

		Expect(runner.run("echo hello", nil)).To(Succeed())
	})
})
