package sshx

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
)

type Executor struct {
	addr     string
	user     string
	password string
}

func NewExecutor(addr, user, password string) *Executor {
	return &Executor{addr: addr, user: user, password: password}
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
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

	outBytes, _ := io.ReadAll(stdout)
	errBytes, _ := io.ReadAll(stderr)

	exitCode := 0
	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
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
