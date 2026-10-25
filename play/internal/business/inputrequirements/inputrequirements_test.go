package inputrequirements

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDevice(t *testing.T) {
	type test struct {
		raw string

		expected   Device
		expectedOk bool
	}

	tests := map[string]func(t *testing.T) test{
		"desktop":   func(t *testing.T) test { return test{raw: "desktop", expected: DeviceDesktop, expectedOk: true} },
		"mobile":    func(t *testing.T) test { return test{raw: "mobile", expected: DeviceMobile, expectedOk: true} },
		"missing":   func(t *testing.T) test { return test{raw: ""} },
		"unknown":   func(t *testing.T) test { return test{raw: "tablet"} },
		"uppercase": func(t *testing.T) test { return test{raw: "DESKTOP"} },
		"both_is_not_a_device": func(t *testing.T) test {
			return test{raw: "both"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			device, ok := ParseDevice(tc.raw)

			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, device)
		})
	}
}

func TestDevicesAllows(t *testing.T) {
	type test struct {
		devices Devices
		device  Device

		expectedAllowed bool
		expectedKnown   bool
	}

	tests := map[string]func(t *testing.T) test{
		"desktop_on_desktop": func(t *testing.T) test {
			return test{devices: DevicesDesktop, device: DeviceDesktop, expectedAllowed: true, expectedKnown: true}
		},
		"mobile_on_desktop": func(t *testing.T) test {
			return test{devices: DevicesDesktop, device: DeviceMobile, expectedKnown: true}
		},
		"desktop_on_both": func(t *testing.T) test {
			return test{devices: DevicesBoth, device: DeviceDesktop, expectedAllowed: true, expectedKnown: true}
		},
		"mobile_on_both": func(t *testing.T) test {
			return test{devices: DevicesBoth, device: DeviceMobile, expectedAllowed: true, expectedKnown: true}
		},
		"mobile_on_mobile": func(t *testing.T) test {
			return test{devices: DevicesMobile, device: DeviceMobile, expectedAllowed: true, expectedKnown: true}
		},
		"desktop_on_mobile": func(t *testing.T) test {
			return test{devices: DevicesMobile, device: DeviceDesktop, expectedKnown: true}
		},
		"unknown_device_on_both": func(t *testing.T) test {
			return test{devices: DevicesBoth, device: Device("tablet"), expectedKnown: true}
		},
		"missing_device_on_both": func(t *testing.T) test {
			return test{devices: DevicesBoth, device: Device(""), expectedKnown: true}
		},
		"unknown_devices": func(t *testing.T) test {
			return test{devices: Devices("tv"), device: DeviceDesktop}
		},
		"empty_devices": func(t *testing.T) test {
			return test{devices: Devices(""), device: DeviceDesktop}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			allowed, known := tc.devices.Allows(tc.device)

			require.Equal(t, tc.expectedAllowed, allowed)
			require.Equal(t, tc.expectedKnown, known)
		})
	}
}

func TestDevicesFor(t *testing.T) {
	type test struct {
		keyboard, hover, press bool

		expected Devices
	}

	tests := map[string]func(t *testing.T) test{
		"nothing":            func(t *testing.T) test { return test{expected: DevicesBoth} },
		"press_only":         func(t *testing.T) test { return test{press: true, expected: DevicesBoth} },
		"hover_only":         func(t *testing.T) test { return test{hover: true, expected: DevicesBoth} },
		"hover_and_press":    func(t *testing.T) test { return test{hover: true, press: true, expected: DevicesDesktop} },
		"keyboard_only":      func(t *testing.T) test { return test{keyboard: true, expected: DevicesDesktop} },
		"keyboard_and_press": func(t *testing.T) test { return test{keyboard: true, press: true, expected: DevicesDesktop} },
		"keyboard_and_hover": func(t *testing.T) test { return test{keyboard: true, hover: true, expected: DevicesDesktop} },
		"every_interaction": func(t *testing.T) test {
			return test{keyboard: true, hover: true, press: true, expected: DevicesDesktop}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, DevicesFor(tc.keyboard, tc.hover, tc.press))
		})
	}
}

func TestRequirementsValidate(t *testing.T) {
	type test struct {
		requirements Requirements

		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"press_only_on_both": func(t *testing.T) test {
			return test{requirements: Requirements{UsesPointerPress: true, Devices: DevicesBoth}}
		},
		"keyboard_on_desktop": func(t *testing.T) test {
			return test{requirements: Requirements{UsesKeyboard: true, Devices: DevicesDesktop}}
		},
		"hover_and_press_on_desktop": func(t *testing.T) test {
			return test{requirements: Requirements{UsesPointerHover: true, UsesPointerPress: true, Devices: DevicesDesktop}}
		},
		"keyboard_claimed_on_both": func(t *testing.T) test {
			return test{requirements: Requirements{UsesKeyboard: true, Devices: DevicesBoth}, expectErr: true}
		},
		"press_only_restricted_to_desktop": func(t *testing.T) test {
			return test{requirements: Requirements{UsesPointerPress: true, Devices: DevicesDesktop}, expectErr: true}
		},
		"mobile_is_never_derived": func(t *testing.T) test {
			return test{requirements: Requirements{UsesPointerPress: true, Devices: DevicesMobile}, expectErr: true}
		},
		"unknown_devices": func(t *testing.T) test {
			return test{requirements: Requirements{Devices: Devices("tv")}, expectErr: true}
		},
		"missing_devices": func(t *testing.T) test {
			return test{requirements: Requirements{}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			err := tc.requirements.Validate()

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
