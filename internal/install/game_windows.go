package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if errors.Is(err, errorInvalidParameter) {
		return false, nil
	}
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false, err
	}
	return code == 259, nil
}

func findRunningClient(dir string) (int, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	root = strings.ToLower(root) + string(os.PathSeparator)
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(snap)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err = syscall.Process32First(snap, &entry); err == nil; err = syscall.Process32Next(snap, &entry) {
		if int(entry.ProcessID) == os.Getpid() {
			continue
		}
		h, e := syscall.OpenProcess(processQueryLimitedInformation, false, entry.ProcessID)
		if e != nil {
			continue
		}
		var buffer [32768]uint16
		size := uint32(len(buffer))
		e = queryFullProcessImageName(h, &buffer[0], &size)
		syscall.CloseHandle(h)
		if e == nil && strings.HasPrefix(strings.ToLower(syscall.UTF16ToString(buffer[:size])), root) {
			return int(entry.ProcessID), nil
		}
	}
	if errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return 0, nil
	}
	return 0, err
}

const processQueryLimitedInformation = 0x1000
const errorInvalidParameter = syscall.Errno(87)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var queryImageName = kernel32.NewProc("QueryFullProcessImageNameW")

func queryFullProcessImageName(handle syscall.Handle, buffer *uint16, size *uint32) error {
	ok, _, err := queryImageName.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(buffer)), uintptr(unsafe.Pointer(size)))
	if ok == 0 {
		return err
	}
	return nil
}
