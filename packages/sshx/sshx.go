package sshx

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Executor struct {
	addr            string
	user            string
	password        string
	hostKeyCallback ssh.HostKeyCallback
}

func NewExecutor(addr, user, password string) *Executor {
	return &Executor{addr: addr, user: user, password: password, hostKeyCallback: ssh.InsecureIgnoreHostKey()}
}

func NewExecutorWithKnownHosts(addr, user, password, knownHostsPath string) (*Executor, error) {
	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, err
	}
	return &Executor{addr: addr, user: user, password: password, hostKeyCallback: callback}, nil
}

type Result struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	Error      string
	DurationMs int64
}

func (e *Executor) Run(ctx context.Context, command string) Result {
	start := time.Now()
	config := &ssh.ClientConfig{
		User: e.user,
		Auth: []ssh.AuthMethod{
			ssh.Password(e.password),
		},
		HostKeyCallback: e.hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", e.addr, config)
	if err != nil {
		return Result{
			Error:      fmt.Sprintf("ssh dial failed: %v", err),
			ExitCode:   -1,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return Result{
			Error:      fmt.Sprintf("ssh session failed: %v", err),
			ExitCode:   -1,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	defer session.Close()

	var stdout, stderr io.Reader
	stdout, err = session.StdoutPipe()
	if err != nil {
		return Result{Error: fmt.Sprintf("stdout pipe: %v", err), ExitCode: -1, DurationMs: time.Since(start).Milliseconds()}
	}
	stderr, err = session.StderrPipe()
	if err != nil {
		return Result{Error: fmt.Sprintf("stderr pipe: %v", err), ExitCode: -1, DurationMs: time.Since(start).Milliseconds()}
	}

	if err := session.Start(command); err != nil {
		return Result{
			Error:      fmt.Sprintf("command start failed: %v", err),
			ExitCode:   -1,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	outBytes, _ := io.ReadAll(io.LimitReader(stdout, 2<<20))
	errBytes, _ := io.ReadAll(io.LimitReader(stderr, 2<<20))

	exitCode := 0
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		_ = session.Close()
		return Result{Stdout: string(outBytes), Stderr: string(errBytes), Error: ctx.Err().Error(), ExitCode: -1, DurationMs: time.Since(start).Milliseconds()}
	}
	if waitErr != nil {
		if exitErr, ok := waitErr.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			exitCode = -1
		}
	}

	return Result{
		Stdout:     string(outBytes),
		Stderr:     string(errBytes),
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}
}
