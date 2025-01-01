//go:build !puregohid
// +build !puregohid

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BertoldVdb/ms-tools/gohid"
	"github.com/karalabe/usb"
)

// hidDeviceWrapper wraps usb.Device to implement gohid.HIDDevice
type hidDeviceWrapper struct {
	dev usb.Device
}

func (d *hidDeviceWrapper) GetFeatureReport(b []byte) (int, error) {
	// Add report ID 0 as per USB HID spec
	if len(b) < 1 {
		return 0, errors.New("buffer too small")
	}
	return d.dev.Read(b)
}

func (d *hidDeviceWrapper) SendFeatureReport(b []byte) (int, error) {
	// Add report ID 0 as per USB HID spec
	if len(b) < 1 {
		return 0, errors.New("buffer too small")
	}
	return d.dev.Write(b)
}

func (d *hidDeviceWrapper) Close() error {
	return d.dev.Close()
}

func tryEnumerate(vid uint16, pid uint16) ([]usb.DeviceInfo, error) {
	var lastErr error
	// Try multiple times as USB enumeration can be flaky
	for attempts := 0; attempts < 3; attempts++ {
		if attempts > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		// First try HID enumeration
		devices, err := usb.Enumerate(vid, pid)
		if err == nil && len(devices) > 0 {
			log.Printf("Found %d devices using HID enumeration", len(devices))
			return devices, nil
		}
		lastErr = err

		// Then try raw USB enumeration
		rawDevices, err := usb.EnumerateRaw(vid, pid)
		if err == nil && len(rawDevices) > 0 {
			log.Printf("Found %d devices using raw USB enumeration", len(rawDevices))
			return rawDevices, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return nil, lastErr
}

func SearchDevice(foundHandler func(info usb.DeviceInfo) error) error {
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

		// Try to match HID interface
		if info.Interface == 4 || info.Usage == 0x0001 {
			if err := foundHandler(info); err != nil {
				if err.Error() == "Done" {
					return nil
				}
				log.Printf("Handler error: %v", err)
				return err
			}
		} else {
			log.Printf("Skipping non-HID interface/usage")
		}
	}
	return nil
}

func OpenDevice() (gohid.HIDDevice, error) {
	var device usb.Device
	err := SearchDevice(func(info usb.DeviceInfo) error {
		log.Printf("Attempting to open device: %s (interface %d)", info.Path, info.Interface)
		// Try multiple times as device opening can be flaky
		for attempts := 0; attempts < 3; attempts++ {
			if attempts > 0 {
				time.Sleep(100 * time.Millisecond)
			}
			dev, err := info.Open()
			if err == nil {
				device = dev
				log.Printf("Successfully opened device: %s", info.Path)
				return errors.New("Done")
			}
			log.Printf("Failed to open device %s (attempt %d): %v", info.Path, attempts+1, err)
		}
		return fmt.Errorf("failed to open device after multiple attempts")
	})
	if device != nil {
		return &hidDeviceWrapper{dev: device}, nil
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
