package scoop

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObject 封装 Windows Job Object。
// scoop 是 cmd 脚本，会派生 powershell 等子进程，直接 Kill 只能终止直接子进程；
// 把子进程加入 Job Object 后，整棵进程树可以一起终止
type jobObject struct {
	handle windows.Handle
}

// newJobObject 创建带 KILL_ON_JOB_CLOSE 标志的 Job Object：
// 本程序退出（含异常退出）时，其中未结束的子进程树会被系统一并终止
func newJobObject() (*jobObject, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}

	return &jobObject{handle: h}, nil
}

// assign 把进程加入 Job Object，其后派生的子进程会自动继承
func (j *jobObject) assign(p *os.Process) error {
	// AssignProcessToJobObject 要求句柄具有 PROCESS_SET_QUOTA 与 PROCESS_TERMINATE 权限
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)

	return windows.AssignProcessToJobObject(j.handle, h)
}

// terminate 终止 Job Object 中的整棵进程树
func (j *jobObject) terminate() error {
	return windows.TerminateJobObject(j.handle, 1)
}

// close 关闭 Job Object 句柄。KILL_ON_JOB_CLOSE 保证句柄关闭时树内进程被终止
func (j *jobObject) close() error {
	return windows.CloseHandle(j.handle)
}
