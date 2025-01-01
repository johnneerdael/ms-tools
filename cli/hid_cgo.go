//go:build !puregohid
// +build !puregohid

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/johnneerdael/ms-tools/gohid"
	"github.com/karalabe/usb"
)

// hidDeviceWrapper wraps usb.Device to implement gohid.HIDDevice
type hidDeviceWrapper struct {
	dev              usb.Device
	ms2130spiEnabled int
}

func (d *hidDeviceWrapper) GetFeatureReport(b []byte) (int, error) {
	if len(b) < 1 {
		return 0, errors.New("buffer too small")
	}
	return d.dev.Read(b)
}

func (d *hidDeviceWrapper) SendFeatureReport(b []byte) (int, error) {
	if len(b) < 1 {
		return 0, errors.New("buffer too small")
	}
	return d.dev.Write(b)
}

func (d *hidDeviceWrapper) Close() error {
	return d.dev.Close()
}

func (d *hidDeviceWrapper) ms2130enableSPI(enable bool) error {
	value := byte(0x00)
	if enable {
		if d.ms2130spiEnabled == 1 {
			return nil
		}

		/* Configure GPIO */
		output := byte(1<<2 | 1<<3 | 1<<4)
		input := byte(1 << 5)

		// Configure GPIO pins
		var out [8]byte
		out[0] = 0xb5 // GPIO command
		out[1] = output
		out[2] = 0
		out[3] = output
		out[4] = input
		if _, err := d.SendFeatureReport(out[:]); err != nil {
			return err
		}

		value = byte(0x10)
	} else {
		if d.ms2130spiEnabled == 0 {
			return nil
		}
	}

	/* Configure pin mux */
	var out [8]byte
	out[0] = 0xb5 // RAM write command
	out[1] = 0xf0 // High byte of address
	out[2] = 0x1f // Low byte of address
	out[3] = value
	if _, err := d.SendFeatureReport(out[:]); err != nil {
		return err
	}

	if enable {
		d.ms2130spiEnabled = 1
	} else {
		d.ms2130spiEnabled = 0
	}

	return nil
}

func (d *hidDeviceWrapper) readFlashPage(page uint16, offset uint8, buf []byte) (int, error) {
	if err := d.ms2130enableSPI(true); err != nil {
		return 0, err
	}

	/* Read from flash to buffer: f701aaaaaabbbb00 (aaaaaa=addr, bbbb=len to read)
	 * Read from buffer to host: f700000000aaaa00 (aaaa=offset) */

	var out [8]byte
	out[0] = 0xf7
	out[1] = 0x01
	binary.BigEndian.PutUint16(out[2:], page)
	binary.BigEndian.PutUint16(out[5:], 256)

	if _, err := d.SendFeatureReport(out[:]); err != nil {
		return 0, err
	}

	out[0] = 0xf7
	out[1] = 0x00
	binary.BigEndian.PutUint16(out[5:], uint16(offset))

	in := make([]byte, 8)
	_, err := d.GetFeatureReport(in)
	if err != nil {
		return 0, err
	}

	maxLen := 0x100 - int(offset)
	if len(buf) > maxLen {
		buf = buf[:maxLen]
	}

	return copy(buf, in[1:]), nil
}

func tryEnumerate(vid uint16, pid uint16) ([]usb.DeviceInfo, error) {
	var lastErr error
	for attempts := 0; attempts < 3; attempts++ {
		if attempts > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		// Try raw USB enumeration first since this is a video device
		rawDevices, err := usb.EnumerateRaw(vid, pid)
		if err == nil && len(rawDevices) > 0 {
			log.Printf("Found %d raw USB devices", len(rawDevices))
			return rawDevices, nil
		}
		if err != nil {
			lastErr = err
			log.Printf("Raw enumeration error: %v", err)
		}

		// Then try HID enumeration as fallback
		devices, err := usb.EnumerateHid(vid, pid)
		if err == nil && len(devices) > 0 {
			log.Printf("Found %d HID devices", len(devices))
			return devices, nil
		}
		if err != nil {
			lastErr = err
			log.Printf("HID enumeration error: %v", err)
		}
	}
	return nil, lastErr
}

func SearchDevice(foundHandler func(info usb.DeviceInfo) error) error {
	if !usb.Supported() {
		return fmt.Errorf("USB support not enabled on this platform")
	}

	log.Printf("Searching for devices with VID:PID %04x:%04x", CLI.VID, CLI.PID)

	// Try primary VID/PID
	devices, err := tryEnumerate(uint16(CLI.VID), uint16(CLI.PID))
	if err != nil {
		log.Printf("Error enumerating devices: %v", err)
	}

	// If no devices found and we have an alternate VID, try that
	if (err != nil || len(devices) == 0) && CLI.VID2 != 0 {
		log.Printf("Trying alternate VID:PID %04x:%04x", CLI.VID2, CLI.PID)
		devices, err = tryEnumerate(uint16(CLI.VID2), uint16(CLI.PID))
		if err != nil {
			log.Printf("Error enumerating devices with alternate VID: %v", err)
		}
	}

	if len(devices) == 0 {
		log.Printf("No devices found")
		return os.ErrNotExist
	}

	for _, info := range devices {
		log.Printf("Found device: VID=%04x PID=%04x Path=%s Serial=%s Interface=%d Usage=%04x",
			info.VendorID, info.ProductID, info.Path, info.Serial, info.Interface, info.Usage)

		if CLI.Serial != "" && info.Serial != CLI.Serial {
			log.Printf("Skipping device: serial number mismatch (want %s)", CLI.Serial)
			continue
		}
		if CLI.RawPath != "" && info.Path != CLI.RawPath {
			log.Printf("Skipping device: path mismatch (want %s)", CLI.RawPath)
			continue
		}

		// Try to open the device
		if err := foundHandler(info); err != nil {
			if err.Error() == "Done" {
				return nil
			}
			log.Printf("Handler error: %v", err)
			return err
		}
	}
	return nil
}

func OpenDevice() (gohid.HIDDevice, error) {
	var device *hidDeviceWrapper
	err := SearchDevice(func(info usb.DeviceInfo) error {
		log.Printf("Attempting to open device: %s (interface %d)", info.Path, info.Interface)
		// Try multiple times as device opening can be flaky
		for attempts := 0; attempts < 3; attempts++ {
			if attempts > 0 {
				time.Sleep(100 * time.Millisecond)
			}
			dev, err := info.Open()
			if err == nil {
				device = &hidDeviceWrapper{
					dev:              dev,
					ms2130spiEnabled: -1,
				}
				log.Printf("Successfully opened device: %s", info.Path)
				return errors.New("Done")
			}
			log.Printf("Failed to open device %s (attempt %d): %v", info.Path, attempts+1, err)
		}
		return fmt.Errorf("failed to open device after multiple attempts")
	})
	if device != nil {
		return device, nil
	}
	if err == nil {
		err = os.ErrNotExist
	}
	return nil, err
}

type ListHIDCmd struct {
}

func (l *ListHIDCmd) Run(c *Context) error {
	return SearchDevice(func(info usb.DeviceInfo) error {
		fmt.Printf("%s: ID %04x:%04x %s %s (Interface %d)\n",
			info.Path, info.VendorID, info.ProductID, info.Manufacturer, info.Product, info.Interface)
		fmt.Println("Device Information:")
		fmt.Printf("\tPath         %s\n", info.Path)
		fmt.Printf("\tVendorID     %04x\n", info.VendorID)
		fmt.Printf("\tProductID    %04x\n", info.ProductID)
		fmt.Printf("\tSerial       %s\n", info.Serial)
		fmt.Printf("\tRelease      %x.%x\n", info.Release>>8, info.Release&0xff)
		fmt.Printf("\tManufacturer %s\n", info.Manufacturer)
		fmt.Printf("\tProduct      %s\n", info.Product)
		fmt.Printf("\tInterface    %d\n", info.Interface)
		fmt.Printf("\tUsage        %04x\n", info.Usage)
		fmt.Println()

		return nil
	})
}
