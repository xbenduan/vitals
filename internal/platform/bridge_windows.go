//go:build windows && (amd64 || arm64)

package platform

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	shcore                     = syscall.NewLazyDLL("shcore.dll")
	getForegroundWindow        = user32.NewProc("GetForegroundWindow")
	getWindowThreadProcessID   = user32.NewProc("GetWindowThreadProcessId")
	getClassName               = user32.NewProc("GetClassNameW")
	getClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	isWindow                   = user32.NewProc("IsWindow")
	isIconic                   = user32.NewProc("IsIconic")
	showWindowAsync            = user32.NewProc("ShowWindowAsync")
	setForegroundWindow        = user32.NewProc("SetForegroundWindow")
	getAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	sendInput                  = user32.NewProc("SendInput")
	openProcess                = kernel32.NewProc("OpenProcess")
	closeHandle                = kernel32.NewProc("CloseHandle")
	queryFullProcessImageName  = kernel32.NewProc("QueryFullProcessImageNameW")
	getCursorPos               = user32.NewProc("GetCursorPos")
	monitorFromPoint           = user32.NewProc("MonitorFromPoint")
	getMonitorInfo             = user32.NewProc("GetMonitorInfoW")
	getDPIForMonitor           = shcore.NewProc("GetDpiForMonitor")
	setWindowPos               = user32.NewProc("SetWindowPos")
)

// Bridge provides clipboard observation, focus restoration, and paste events.
// Call its methods on MyGo's main thread after the Windows backend is initialized.
type Bridge struct{}

// NewBridge checks the native functions required for Windows integration.
func NewBridge() (*Bridge, error) {
	for _, proc := range []*syscall.LazyProc{
		getForegroundWindow, getWindowThreadProcessID, getClassName,
		getClipboardSequenceNumber, isWindow, isIconic, showWindowAsync,
		setForegroundWindow, getAsyncKeyState, sendInput, openProcess,
		closeHandle, queryFullProcessImageName, getCursorPos, monitorFromPoint,
		getMonitorInfo, setWindowPos,
	} {
		if err := proc.Find(); err != nil {
			return nil, err
		}
	}
	return &Bridge{}, nil
}

// Trusted reports that Windows needs no macOS-style Accessibility permission.
// Windows can still reject injected input into a more privileged process.
func (b *Bridge) Trusted() bool { return true }

func windowPID(window uintptr) int {
	var pid uint32
	getWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&pid)))
	return int(pid)
}

// Frontmost returns the active external window, ignoring our panel and shell UI.
// Ignoring the taskbar preserves the previous paste destination on tray clicks.
func (b *Bridge) Frontmost() Target {
	window, _, _ := getForegroundWindow.Call()
	pid := windowPID(window)
	if window == 0 || pid == 0 || pid == os.Getpid() {
		return Target{}
	}
	var class [256]uint16
	getClassName.Call(window, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	switch syscall.UTF16ToString(class[:]) {
	case "Shell_TrayWnd", "Shell_SecondaryTrayWnd", "NotifyIconOverflowWindow",
		"TopLevelWindowForOverflowXamlIsland", "Progman", "WorkerW":
		return Target{}
	}
	return Target{PID: pid, Window: window, Name: processName(pid)}
}

func processName(pid int) string {
	const queryLimitedInformation = 0x1000
	process, _, _ := openProcess.Call(queryLimitedInformation, 0, uintptr(pid))
	if process == 0 {
		return ""
	}
	defer closeHandle.Call(process)
	var path [32768]uint16
	size := uint32(len(path))
	ok, _, _ := queryFullProcessImageName.Call(process, 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return ""
	}
	name := filepath.Base(syscall.UTF16ToString(path[:size]))
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// ChangeCount returns the clipboard sequence number used to detect new copies.
func (b *Bridge) ChangeCount() int {
	count, _, _ := getClipboardSequenceNumber.Call()
	return int(count)
}

// Activate restores the original window if it still belongs to the target process.
// Windows foreground restrictions may prevent activation; callers must check focus.
func (b *Bridge) Activate(target Target) bool {
	if target.PID <= 0 || target.PID == os.Getpid() || target.Window == 0 {
		return false
	}
	valid, _, _ := isWindow.Call(target.Window)
	if valid == 0 || windowPID(target.Window) != target.PID {
		return false
	}
	if minimized, _, _ := isIconic.Call(target.Window); minimized != 0 {
		showWindowAsync.Call(target.Window, 9) // SW_RESTORE
	}
	ok, _, _ := setForegroundWindow.Call(target.Window)
	front, _, _ := getForegroundWindow.Call()
	return ok != 0 || front == target.Window
}

// ModifiersReleased reports whether Control, Shift, Alt, and both Windows keys are up.
func (b *Bridge) ModifiersReleased() bool {
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C} {
		if state, _, _ := getAsyncKeyState.Call(key); state&0x8000 != 0 {
			return false
		}
	}
	return true
}

// keyboardEvent matches the 40-byte INPUT union layout on Windows amd64/arm64.
type keyboardEvent struct {
	kind      uint32
	_         uint32
	key       uint16
	scan      uint16
	flags     uint32
	time      uint32
	_         uint32
	extraInfo uintptr
	_         [8]byte
}

// Paste sends Ctrl+V only after verifying the exact destination window and modifiers.
// Success confirms event delivery, not that the destination accepted the content.
func (b *Bridge) Paste(target Target) error {
	if !b.Frontmost().Matches(target) || !b.ModifiersReleased() {
		return errors.New("目标窗口或按键状态已改变，内容已复制，可使用 Ctrl+V 粘贴")
	}
	const keyUp = 0x0002
	events := [...]keyboardEvent{
		{kind: 1, key: 0x11}, // VK_CONTROL down
		{kind: 1, key: 0x56}, // V down
		{kind: 1, key: 0x56, flags: keyUp},
		{kind: 1, key: 0x11, flags: keyUp},
	}
	sent, _, _ := sendInput.Call(uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), unsafe.Sizeof(events[0]))
	if sent != uintptr(len(events)) {
		if sent > 0 {
			// Release any partially inserted keys so a failed paste cannot leave
			// Control held down. UIPI failures normally insert zero events.
			sendInput.Call(2, uintptr(unsafe.Pointer(&events[2])), unsafe.Sizeof(events[0]))
		}
		return errors.New("Windows 未允许自动粘贴，内容已复制，可使用 Ctrl+V 粘贴")
	}
	return nil
}

type screenPoint struct{ x, y int32 }
type screenRect struct{ left, top, right, bottom int32 }
type monitorInfo struct {
	size    uint32
	monitor screenRect
	work    screenRect
	flags   uint32
}

// PlaceWindow opens a panel near the cursor using the destination monitor's DPI.
// Width and height are logical units. Native physical coordinates avoid mixed-DPI
// monitor overlap; the caller must temporarily allow programmatic window movement.
func (b *Bridge) PlaceWindow(window uintptr, width, height float64) error {
	var cursor screenPoint
	if ok, _, err := getCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); ok == 0 {
		return fmt.Errorf("读取鼠标位置：%w", err)
	}
	// POINT is passed by value as one 64-bit argument on both supported ABIs.
	point := uint64(uint32(cursor.x)) | uint64(uint32(cursor.y))<<32
	monitor, _, _ := monitorFromPoint.Call(uintptr(point), 2) // MONITOR_DEFAULTTONEAREST
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, err := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return fmt.Errorf("读取屏幕工作区：%w", err)
	}
	scale := 1.0
	if getDPIForMonitor.Find() == nil {
		var x, y uint32
		if hr, _, _ := getDPIForMonitor.Call(monitor, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); hr == 0 && x > 0 {
			scale = float64(x) / 96
		}
	}
	work := info.work
	rect := PanelBounds(float64(cursor.x), float64(cursor.y), width*scale, height*scale, 12*scale, Bounds{
		X: float64(work.left), Y: float64(work.top),
		Width: float64(work.right - work.left), Height: float64(work.bottom - work.top),
	})
	// Crossing a DPI boundary can synchronously apply WM_DPICHANGED's suggested
	// rectangle. Reapply the physical bounds after that transition has settled.
	for range 2 {
		ok, _, err := setWindowPos.Call(window, 0,
			uintptr(int32(math.Round(rect.X))), uintptr(int32(math.Round(rect.Y))),
			uintptr(int32(math.Round(rect.Width))), uintptr(int32(math.Round(rect.Height))),
			0x0004|0x0010) // SWP_NOZORDER | SWP_NOACTIVATE
		if ok == 0 {
			return fmt.Errorf("定位面板：%w", err)
		}
	}
	return nil
}
