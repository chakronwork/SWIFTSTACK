//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type jobObject struct {
	handle windows.Handle
}

func createJobObject() (jobObject, error) {
	hJob, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return jobObject{}, fmt.Errorf("CreateJobObject failed: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}

	_, err = windows.SetInformationJobObject(
		hJob,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(hJob)
		return jobObject{}, fmt.Errorf("SetInformationJobObject failed: %w", err)
	}

	return jobObject{handle: hJob}, nil
}

func (j *jobObject) Assign(proc *os.Process) error {
	if proc == nil || j.handle == 0 {
		return nil
	}

	hProc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(proc.Pid))
	if err != nil {
		return fmt.Errorf("OpenProcess failed for PID %d: %w", proc.Pid, err)
	}
	defer windows.CloseHandle(hProc)

	err = windows.AssignProcessToJobObject(j.handle, hProc)
	if err != nil {
		return fmt.Errorf("AssignProcessToJobObject failed for PID %d: %w", proc.Pid, err)
	}

	return nil
}

func (j *jobObject) Close() error {
	if j.handle != 0 {
		err := windows.CloseHandle(j.handle)
		j.handle = 0
		return err
	}
	return nil
}

func setProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// CREATE_NEW_PROCESS_GROUP prevents console Ctrl+C events from affecting child workers directly
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}
