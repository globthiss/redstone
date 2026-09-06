//go:build windows

package iconfix

import (
	"fmt"
	"syscall"
	"unsafe"
)

func CreateLaunchShortcut(lnkPath, javaBinary, args, workDir, iconPath string) error {
	if err := coInitialize(); err != nil {
		return err
	}
	defer coUninitialize()

	shellLink, err := createShellLinkInstance()
	if err != nil {
		return err
	}
	defer shellLink.Release()

	if err := shellLink.SetPath(javaBinary); err != nil {
		return err
	}
	if err := shellLink.SetArguments(args); err != nil {
		return err
	}
	if err := shellLink.SetWorkingDirectory(workDir); err != nil {
		return err
	}
	if iconPath != "" {
		if err := shellLink.SetIconLocation(iconPath, 0); err != nil {
			return err
		}
	}

	persistFile, err := shellLink.QueryInterfacePersistFile()
	if err != nil {
		return err
	}
	defer persistFile.Release()

	return persistFile.Save(lnkPath)
}

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitialize     = ole32.NewProc("CoInitialize")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

var (
	clsidShellLink = syscall.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIShellLinkW = syscall.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIPersistFile = syscall.GUID{Data1: 0x0000010b, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

func coInitialize() error {
	r, _, _ := procCoInitialize.Call(0)
	if r != 0 && r != 1 { // S_OK=0, S_FALSE=1 (уже инициализировано) - оба приемлемы
		return fmt.Errorf("CoInitialize failed: 0x%x", r)
	}
	return nil
}

func coUninitialize() { procCoUninitialize.Call() }

type iShellLinkW struct{ vtbl *iShellLinkWVtbl }
type iShellLinkWVtbl struct {
	QueryInterface, AddRef, Release                                      uintptr
	GetPath, GetIDList, SetIDList, GetDescription, SetDescription        uintptr
	GetWorkingDirectory, SetWorkingDirectory, GetArguments, SetArguments uintptr
	GetHotkey, SetHotkey, GetShowCmd, SetShowCmd                         uintptr
	GetIconLocation, SetIconLocation, SetRelativePath, Resolve, SetPath  uintptr
}

func createShellLinkInstance() (*iShellLinkW, error) {
	var obj *iShellLinkW
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, /*CLSCTX_INPROC_SERVER*/
		uintptr(unsafe.Pointer(&iidIShellLinkW)), uintptr(unsafe.Pointer(&obj)),
	)
	if r != 0 {
		return nil, fmt.Errorf("CoCreateInstance(IShellLinkW) failed: 0x%x", r)
	}
	return obj, nil
}

func (s *iShellLinkW) call(fn uintptr, a ...uintptr) uintptr {
	args := append([]uintptr{uintptr(unsafe.Pointer(s))}, a...)
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

func (s *iShellLinkW) Release() { s.call(s.vtbl.Release) }

func (s *iShellLinkW) SetPath(path string) error {
	p, _ := syscall.UTF16PtrFromString(path)
	if r := s.call(s.vtbl.SetPath, uintptr(unsafe.Pointer(p))); r != 0 {
		return fmt.Errorf("SetPath failed: 0x%x", r)
	}
	return nil
}

func (s *iShellLinkW) SetArguments(args string) error {
	p, _ := syscall.UTF16PtrFromString(args)
	if r := s.call(s.vtbl.SetArguments, uintptr(unsafe.Pointer(p))); r != 0 {
		return fmt.Errorf("SetArguments failed: 0x%x", r)
	}
	return nil
}

func (s *iShellLinkW) SetWorkingDirectory(dir string) error {
	p, _ := syscall.UTF16PtrFromString(dir)
	if r := s.call(s.vtbl.SetWorkingDirectory, uintptr(unsafe.Pointer(p))); r != 0 {
		return fmt.Errorf("SetWorkingDirectory failed: 0x%x", r)
	}
	return nil
}

func (s *iShellLinkW) SetIconLocation(path string, index int32) error {
	p, _ := syscall.UTF16PtrFromString(path)
	if r := s.call(s.vtbl.SetIconLocation, uintptr(unsafe.Pointer(p)), uintptr(index)); r != 0 {
		return fmt.Errorf("SetIconLocation failed: 0x%x", r)
	}
	return nil
}

type iPersistFile struct{ vtbl *iPersistFileVtbl }
type iPersistFileVtbl struct {
	QueryInterface, AddRef, Release                uintptr
	GetClassID                                     uintptr
	IsDirty, Load, Save, SaveCompleted, GetCurFile uintptr
}

func (s *iShellLinkW) QueryInterfacePersistFile() (*iPersistFile, error) {
	var obj *iPersistFile
	r := s.call(s.vtbl.QueryInterface, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&obj)))
	if r != 0 {
		return nil, fmt.Errorf("QueryInterface(IPersistFile) failed: 0x%x", r)
	}
	return obj, nil
}

func (p *iPersistFile) Release() {
	syscall.SyscallN(p.vtbl.Release, uintptr(unsafe.Pointer(p)))
}

func (p *iPersistFile) Save(path string) error {
	wp, _ := syscall.UTF16PtrFromString(path)
	r, _, _ := syscall.SyscallN(p.vtbl.Save, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(wp)), 1)
	if r != 0 {
		return fmt.Errorf("IPersistFile.Save failed: 0x%x", r)
	}
	return nil
}
