//go:build !windows

package service

import (
	"os"
	"os/exec"
)

type jobObject struct{}

func createJobObject() (jobObject, error) {
	return jobObject{}, nil
}

func (j *jobObject) Assign(proc *os.Process) error {
	return nil
}

func (j *jobObject) Close() error {
	return nil
}

func setProcAttr(cmd *exec.Cmd) {}
