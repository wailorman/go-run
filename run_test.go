package run

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func Test__Streamed(t *testing.T) {
	assert := assert.New(t)

	command := []string{"bash", "-c", "echo", "1"}

	runner := New(command)

	// done, stdout, stderr, failures := runner.Run()
	err := runner.Run(context.Background())

	assert.Nil(err, "Runner initialization error")

	done, stdout, stderr, failures := runner.StreamOutput()

	for {
		select {
		case line := <-stdout:
			if line != "1" && line != "\n" {
				assert.Error(fmt.Errorf("unexpected line received: %s", line), "stdout")
			}
		case <-stderr:
			assert.Error(errors.New("Received stderr"), "")
		case failure := <-failures:
			assert.Nil(failure, "failure")
			return
		case <-done:
			return
		case <-time.After(3 * time.Second):
			assert.Error(errors.New("Timeout"), "")
		}
	}
}
