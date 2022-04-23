package run

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Command _
type Command struct {
	command        []string
	alreadyStarted bool

	timeout *time.Duration

	stdOutPipe io.ReadCloser
	stdErrPipe io.ReadCloser
	stdInPipe  io.WriteCloser

	done     chan struct{}
	stdout   chan string
	stderr   chan string
	failures chan error
}

// New _
func New(command []string) *Command {
	return &Command{
		command: command,
	}
}

// ErrDirty _
var ErrDirty = errors.New("Runner was already started")

// StreamOutput _
func (i *Command) StreamOutput() (done chan struct{}, stdout chan string, stderr chan string, failures chan error) {
	return i.done, i.stdout, i.stderr, i.failures
}

// SetTimeout _
func (i *Command) SetTimeout(timeout time.Duration) {
	i.timeout = &timeout
}

// Wait _
func (i *Command) Wait() error {
	return i.wait(true, false)
}

// wait _
func (i *Command) wait(collectFailures, collectStderr bool) error {
	errs := make([]error, 0)

	if collectFailures {
		go func() {
			for err := range i.failures {
				errs = append(errs, err)
			}
		}()
	}

	if collectStderr {
		go func() {
			for err := range i.stderr {
				errs = append(errs, errors.New(err))
			}
		}()
	}

	<-i.done

	if len(errs) == 1 {
		return errs[0]
	}

	if len(errs) > 1 {
		errorsMessages := make([]string, 0)

		for _, err := range errs {
			errorsMessages = append(errorsMessages, err.Error())
		}

		return errors.New(strings.Join(errorsMessages, "; "))
	}

	return nil
}

// Run _
func (i *Command) Run(ctx context.Context) (err error) {

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
		var cmdContext context.Context
		var kill context.CancelFunc

		if i.timeout != nil {
			cmdContext, kill = context.WithTimeout(ctx, *i.timeout)
		} else {
			cmdContext, kill = context.WithCancel(ctx)
		}

		fail := buildFailFunc(kill, i.failures)

		proc := exec.CommandContext(cmdContext, i.command[0], i.command[1:len(i.command)]...)

		i.stdOutPipe, err = proc.StdoutPipe()

		if err != nil {
			fail(err, "Failed to get stderr")
			return
		}

		i.stdErrPipe, err = proc.StderrPipe()

		if err != nil {
			fail(err, "Failed to get stderr")
			return
		}

		i.stdInPipe, err = proc.StdinPipe()

		if err != nil {
			fail(err, "Stdin not available")
			return
		}

		err = proc.Start()

		if err != nil {
			fail(err, "Failed to run command")
			return
		}

		go scanLines(i.stdOutPipe, i.stdout)
		go scanLines(i.stdErrPipe, i.stderr)

		waitForFinish(proc, i.failures, i.done)

		close(i.stdout)
		close(i.stderr)
		close(i.failures)
	}()

	return nil
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

		if err != nil {
			pipe.Close()
			return
		}

		out <- line
	}
}

func buildFailFunc(kill func(), failures chan error) func(err error, description string) error {
	return func(err error, description string) error {
		failure := errors.Wrap(err, description)
		failures <- failure
		kill()
		return failure
	}
}

// Quote _
func Quote(str string) string {
	return "\"" + str + "\""
}
