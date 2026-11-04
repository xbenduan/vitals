//go:build darwin

package platform

import (
	"os"
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestSelectionNativeBindings(t *testing.T) {
	bridge, err := NewBridge()
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the actual ABI with synthetic text; never read the current
	// frontmost app, selection or clipboard in the default test suite.
	text := "Selected text · 选中文字"
	value := bridge.cfString(0, text, 0x08000100)
	if value == 0 {
		t.Fatal("CFString creation failed")
	}
	defer bridge.release(value)
	if bridge.typeID(value) != bridge.stringID() || nativeString(objc.ID(value)) != text {
		t.Fatal("native selection string conversion failed")
	}
	app := bridge.axApp(int32(os.Getpid()))
	if app == 0 {
		t.Fatal("AXUIElement application creation failed")
	}
	defer bridge.release(app)
	if err := bridge.axTimeout(app, 0.2); err != 0 {
		t.Fatalf("AXUIElement timeout setup failed: %d", err)
	}
}
