package workerexe

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// killOnCloseJob — объект задания, который при закрытии дескриптора завершает все свои процессы.
func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job) // задание не настроилось — закрыть пустое
		return 0, err
	}
	return job, nil
}

// BindSelfToJob — эта программа и все её потомки (uv, Python, yue-server) в одном задании: программа вышла
// («Выход» в трее, «Прервать» установку, сбой) — потомки завершаются, а не держат видеопамять и папку.
func BindSelfToJob() error {
	job, err := killOnCloseJob()
	if err != nil {
		return err
	}
	// дескриптор не закрывается: его закроет система при выходе программы — это и есть сигнал
	return windows.AssignProcessToJobObject(job, windows.CurrentProcess())
}

// KillTreeOnClose — процесс p со всеми потомками в своём задании; возвращённая функция их завершает (перезапуск
// воркера из трея: Python и запущенный им yue-server останавливаются вместе).
func KillTreeOnClose(p *os.Process) (func(), error) {
	job, err := killOnCloseJob()
	if err != nil {
		return func() { _ = p.Kill() }, err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return func() { _ = p.Kill() }, err
	}
	defer func() { _ = windows.CloseHandle(h) }() // дескриптор процесса нужен только для привязки к заданию
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return func() { _ = p.Kill() }, err
	}
	return func() { _ = windows.CloseHandle(job) }, nil
}

// HideConsole — дочерний процесс без своего консольного окна: программа воркера сама без консоли, и иначе каждый
// запуск uv/python/nvidia-smi мелькал бы чёрным окном.
func HideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func FreeBytes(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, nil, nil); err != nil {
		return 0, err
	}
	return free, nil
}

func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &windows.Overlapped{})
}

func lockBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }
