// Package inputrequirements defines the interactions a game version uses, the
// device classes a player's client can be, and which of them can play a
// version: a desktop has a keyboard and a mouse; a touch device has no
// keyboard and imitates the pointer for only one of hover or press.
package inputrequirements

import "fmt"

// Device is the class a player's client classifies itself as.
type Device string

const (
	DeviceDesktop Device = "desktop"
	DeviceMobile  Device = "mobile"
)

// ParseDevice returns the Device raw names; ok is false for any other value.
func ParseDevice(raw string) (device Device, ok bool) {
	switch Device(raw) {
	case DeviceDesktop, DeviceMobile:
		return Device(raw), true
	default:
		return "", false
	}
}

// Devices are the device classes a game version can be played on.
type Devices string

const (
	DevicesDesktop Devices = "desktop"

	// DevicesMobile is a valid value for games only touch devices can play;
	// no interaction makes a game need it yet.
	DevicesMobile Devices = "mobile"

	DevicesBoth Devices = "both"
)

// Allows reports whether device may play a version d allows; known is false
// when d is none of the Devices values.
func (d Devices) Allows(device Device) (allowed bool, known bool) {
	switch d {
	case DevicesDesktop:
		return device == DeviceDesktop, true
	case DevicesMobile:
		return device == DeviceMobile, true
	case DevicesBoth:
		return device == DeviceDesktop || device == DeviceMobile, true
	default:
		return false, false
	}
}

// Requirements are the interactions a game version uses: keyboard keys
// (letters, digits, arrows and space), moving the pointer without pressing
// (hover), and pressing or dragging with the primary button or a finger.
// Devices is derived from them by DevicesFor and stored for queries.
type Requirements struct {
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          Devices
}

// DevicesFor returns the devices that can play a version using these
// interactions: both when it needs no keyboard and at most one of hover or
// press, desktop otherwise.
func DevicesFor(usesKeyboard, usesPointerHover, usesPointerPress bool) Devices {
	if !usesKeyboard && !(usesPointerHover && usesPointerPress) {
		return DevicesBoth
	}
	return DevicesDesktop
}

// Validate returns an error unless r.Devices is what DevicesFor derives from
// r's interactions. Whoever writes Requirements validates them first.
func (r Requirements) Validate() error {
	expected := DevicesFor(r.UsesKeyboard, r.UsesPointerHover, r.UsesPointerPress)
	if r.Devices != expected {
		return fmt.Errorf("devices %q do not match the interactions, which give %q", r.Devices, expected)
	}
	return nil
}
