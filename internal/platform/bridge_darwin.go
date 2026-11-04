//go:build darwin

package platform

import (
	"errors"
	"os"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Bridge provides clipboard observation, focus restoration, and paste events.
// Call its methods on MyGo's main thread after AppKit is initialized.
// No Objective-C object escapes a call.
type Bridge struct {
	trusted   func() bool
	createKey func(uintptr, uint16, bool) uintptr
	setFlags  func(uintptr, uint64)
	postToPID func(int32, uintptr)
	release   func(uintptr)
	keyState  func(int32, uint16) bool
	axApp     func(int32) uintptr
	axCopy    func(uintptr, uintptr, *uintptr) int32
	axTimeout func(uintptr, float32) int32
	cfString  func(uintptr, string, uint32) uintptr
	typeID    func(uintptr) uintptr
	stringID  func() uintptr
}

// NewBridge loads macOS frameworks and binds the native functions.
func NewBridge() (*Bridge, error) {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	ax, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	b := &Bridge{}
	purego.RegisterLibFunc(&b.trusted, ax, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&b.createKey, cg, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&b.setFlags, cg, "CGEventSetFlags")
	purego.RegisterLibFunc(&b.postToPID, cg, "CGEventPostToPid")
	purego.RegisterLibFunc(&b.keyState, cg, "CGEventSourceKeyState")
	purego.RegisterLibFunc(&b.release, cf, "CFRelease")
	purego.RegisterLibFunc(&b.axApp, ax, "AXUIElementCreateApplication")
	purego.RegisterLibFunc(&b.axCopy, ax, "AXUIElementCopyAttributeValue")
	purego.RegisterLibFunc(&b.axTimeout, ax, "AXUIElementSetMessagingTimeout")
	purego.RegisterLibFunc(&b.cfString, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&b.typeID, cf, "CFGetTypeID")
	purego.RegisterLibFunc(&b.stringID, cf, "CFStringGetTypeID")
	return b, nil
}

func nativeString(value objc.ID) string {
	if value == 0 {
		return ""
	}
	return objc.Send[string](value, objc.RegisterName("UTF8String"))
}

// Trusted reports whether the app has Accessibility permission.
func (b *Bridge) Trusted() bool {
	return b.trusted()
}

// Frontmost returns the active external app, or a zero target for this process.
func (b *Bridge) Frontmost() Target {
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	app := workspace.Send(objc.RegisterName("frontmostApplication"))
	pid := int(app.Send(objc.RegisterName("processIdentifier")))
	if pid == 0 || pid == os.Getpid() {
		return Target{}
	}
	return Target{PID: pid, Name: nativeString(app.Send(objc.RegisterName("localizedName")))}
}

// ChangeCount returns the system pasteboard revision used to detect new copies.
func (b *Bridge) ChangeCount() int {
	board := objc.ID(objc.GetClass("NSPasteboard")).Send(objc.RegisterName("generalPasteboard"))
	return int(board.Send(objc.RegisterName("changeCount")))
}

// Activate asks macOS to bring the target forward; focus may settle asynchronously.
func (b *Bridge) Activate(target Target) bool {
	if target.PID <= 0 || target.PID == os.Getpid() {
		return false
	}
	app := objc.ID(objc.GetClass("NSRunningApplication")).Send(objc.RegisterName("runningApplicationWithProcessIdentifier:"), int32(target.PID))
	return app != 0 && objc.Send[bool](app, objc.RegisterName("activateWithOptions:"), uint64(2))
}

// ModifiersReleased reports whether Command, Shift, Option, and Control are all up.
func (b *Bridge) ModifiersReleased() bool {
	// Wait for both sides of Command, Shift, Option and Control, so opening
	// via a held shortcut cannot turn the synthetic Cmd+V into Cmd+Shift+V.
	for _, key := range []uint16{54, 55, 56, 60, 58, 61, 59, 62} {
		if b.keyState(1, key) {
			return false
		}
	}
	return true
}

// Paste sends Command+V to the target after checking permission and current focus.
// Success means the events were sent, not that the target accepted the content.
func (b *Bridge) Paste(target Target) error {
	if !b.trusted() {
		return errors.New("请在系统设置中允许 Vitals 使用「辅助功能」，以便自动粘贴")
	}
	if !b.Frontmost().Matches(target) {
		return errors.New("目标应用没有获得焦点，内容已复制，可使用 ⌘V 粘贴")
	}
	return b.commandKey(target, 9)
}

// SelectedText reads the focused accessibility element without touching the clipboard.
// Supported is true even for an empty selection, so callers must not copy a whole
// editor line when Accessibility explicitly reports that nothing is selected.
func (b *Bridge) SelectedText(target Target) (text string, supported bool, err error) {
	if !b.Trusted() {
		return "", false, errors.New("请在系统设置中允许 Vitals 使用「辅助功能」，以便读取选中文字")
	}
	if !b.Frontmost().Matches(target) {
		return "", false, errors.New("原应用已失去焦点，请重新选择文字后使用快捷键")
	}
	app := b.axApp(int32(target.PID))
	if app == 0 {
		return "", false, nil
	}
	defer b.release(app)
	b.axTimeout(app, 0.2)
	copyAttribute := func(element uintptr, name string) uintptr {
		key := b.cfString(0, name, 0x08000100) // kCFStringEncodingUTF8
		if key == 0 {
			return 0
		}
		defer b.release(key)
		var value uintptr
		if b.axCopy(element, key, &value) != 0 {
			return 0
		}
		return value
	}
	focused := copyAttribute(app, "AXFocusedUIElement")
	if focused == 0 {
		return "", false, nil
	}
	defer b.release(focused)
	b.axTimeout(focused, 0.2)
	value := copyAttribute(focused, "AXSelectedText")
	if value == 0 {
		return "", false, nil
	}
	defer b.release(value)
	if b.typeID(value) != b.stringID() {
		return "", false, nil
	}
	return nativeString(objc.ID(value)), true, nil
}

// Copy asks the still-focused source app to copy its selection after modifiers settle.
func (b *Bridge) Copy(target Target) error {
	if !b.Trusted() {
		return errors.New("请允许 Vitals 使用「辅助功能」以读取选中文字")
	}
	if !b.Frontmost().Matches(target) || !b.ModifiersReleased() {
		return errors.New("原应用或按键状态已改变，请重新划词后使用快捷键")
	}
	return b.commandKey(target, 8)
}

func (b *Bridge) commandKey(target Target, key uint16) error {
	down, up := b.createKey(0, key, true), b.createKey(0, key, false)
	if down == 0 || up == 0 {
		if down != 0 {
			b.release(down)
		}
		if up != 0 {
			b.release(up)
		}
		return errors.New("无法创建按键事件")
	}
	defer b.release(down)
	defer b.release(up)
	b.setFlags(down, 1<<20)
	b.setFlags(up, 1<<20)
	b.postToPID(int32(target.PID), down)
	b.postToPID(int32(target.PID), up)
	return nil
}
