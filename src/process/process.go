package process

import (
	"bufio"
	"context"
	"os/exec"
)

type LogLine struct {
	Text     string
	IsStderr bool
}

type Handle struct {
	Cmd     *exec.Cmd
	Logs    <-chan LogLine
	cleanup func()
	done    chan struct{}
	stop    chan struct{}
	waitErr error
}

type Options struct {
	JavaBinary string
	Args       []string
	WorkDir    string
	OnExit     func(exitCode int, err error)
	Cleanup    func()
}

func Start(ctx context.Context, o Options) (*Handle, error) {
	cmd := exec.CommandContext(ctx, o.JavaBinary, o.Args...)
	cmd.Dir = o.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	logs := make(chan LogLine, 256)
	stop := make(chan struct{})

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	pumpDone := make(chan struct{}, 2)
	pump := func(scanner *bufio.Scanner, isErr bool) {
		defer func() { pumpDone <- struct{}{} }()
		for scanner.Scan() {
			select {
			case logs <- LogLine{Text: scanner.Text(), IsStderr: isErr}:
			default:
			}
		}
	}

	go pump(bufio.NewScanner(stdout), false)
	go pump(bufio.NewScanner(stderr), true)

	h := &Handle{Cmd: cmd, Logs: logs, cleanup: o.Cleanup, done: make(chan struct{}), stop: stop}

	go func() {
		err := cmd.Wait()
		h.waitErr = err

		close(stop)
		<-pumpDone
		<-pumpDone
		close(logs)

		if h.cleanup != nil {
			h.cleanup()
		}
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		if o.OnExit != nil {
			o.OnExit(exitCode, err)
		}
		close(h.done)
	}()

	return h, nil
}

func (h *Handle) Wait() error {
	<-h.done
	return h.waitErr
}

func (h *Handle) Kill() error {
	if h.Cmd.Process == nil {
		return nil
	}
	return h.Cmd.Process.Kill()
}
