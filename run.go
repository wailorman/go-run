package run

import (
	"bufio"
	"io"
	"os/exec"

	"github.com/pkg/errors"
)

// Instance _
type Instance struct {
	command        []string
	alreadyStarted bool

	stdOutPipe io.ReadCloser
	stdErrPipe io.ReadCloser
	stdInPipe  io.WriteCloser

	cancel chan struct{}

	done     chan struct{}
	stdout   chan string
	stderr   chan string
	failures chan error
}

// New _
func New(command []string) *Instance {
	return &Instance{
		command: command,
	}
}

// ErrDirty _
var ErrDirty = errors.New("Runner was already started")

// Cancel _
func (i *Instance) Cancel() {
	close(i.cancel)
}

// StreamOutput _
func (i *Instance) StreamOutput() (done chan struct{}, stdout chan string, stderr chan string, failures chan error) {
	return i.done, i.stdout, i.stderr, i.failures
}

// Run _
func (i *Instance) Run() (err error) {

	if i.alreadyStarted {
		return ErrDirty
	}

	i.alreadyStarted = true

	i.done = make(chan struct{})
	i.stdout = make(chan string)
	i.stderr = make(chan string)
	i.failures = make(chan error)

	go func() {
		var err error

		proc := exec.Command(i.command[0], i.command[1:len(i.command)]...)

		i.stdOutPipe, err = proc.StdoutPipe()

		if err != nil {
			i.failures <- errors.Wrap(err, "Failed to get stderr")
			close(i.done)
			return
		}

		i.stdErrPipe, err = proc.StderrPipe()

		if err != nil {
			i.failures <- errors.Wrap(err, "Failed to get stderr")
			close(i.done)
			return
		}

		i.stdInPipe, err = proc.StdinPipe()

		if err != nil {
			i.failures <- errors.Wrap(err, "Stdin not available")
			close(i.done)
			return
		}

		err = proc.Start()

		if err != nil {
			i.failures <- errors.Wrap(err, "Failed to run command")
			close(i.done)
			return
		}

		go cancelListener(i.cancel, i.stdInPipe)
		go waitForFinish(proc, i.failures, i.done)

		go scanLines(i.stdOutPipe, i.stdout)
		go scanLines(i.stdErrPipe, i.stderr)
	}()

	return nil
}

func cancelListener(cancel chan struct{}, stdInPipe io.WriteCloser) {
	if cancel == nil {
		return
	}

	for {
		select {
		case <-cancel:
			stdInPipe.Write([]byte("q\n"))
			return
		}
	}
}

// Waiter _
type Waiter interface {
	Wait() error
}

// PipeCloser _
type PipeCloser interface {
	Close() error
}

func waitForFinish(proc Waiter, failures chan error, done chan struct{}) {
	err := proc.Wait()

	if err != nil {
		failures <- errors.Wrap(err, "Failed to finish process")
	}

	close(done)
}

func scanLines(pipe io.ReadCloser, out chan string) {
	reader := bufio.NewReader(pipe)

	for {
		line, err := reader.ReadString('\n')
		out <- line

		if err != nil {
			pipe.Close()
			return
		}
	}
}
