//go:build !puregohid
// +build !puregohid

package main

import (
	"fmt"
	"os"
	"errors"

	"github.com/BertoldVdb/ms-tools/gohid"
	"github.com/karalabe/usb"
)

// hidDeviceWrapper wraps usb.Device to implement gohid.HIDDevice
type hidDeviceWrapper struct {
	dev usb.Device
}

func (d *hidDeviceWrapper) GetFeatureReport(b []byte) (int, error) {
	return d.dev.Read(b)
}

func (d *hidDeviceWrapper) SendFeatureReport(b []byte) (int, error) {
	return d.dev.Write(b)
}

func (d *hidDeviceWrapper) Close() error {
	return d.dev.Close()
}

func SearchDevice(foundHandler func(info usb.DeviceInfo) error) error {
	// Enumerate both HID and raw USB devices
	devices, err := usb.Enumerate(uint16(CLI.VID), uint16(CLI.PID))
	if err != nil {
		return err
	}
	if len(devices) == 0 && CLI.VID2 != 0 {
		devices, err = usb.Enumerate(uint16(CLI.VID2), uint16(CLI.PID))
		if err != nil {
			return err
		}
	}

	for _, info := range devices {
		if CLI.Serial != "" && info.Serial != CLI.Serial {
			continue
		}
		if CLI.RawPath != "" && info.Path != CLI.RawPath {
			continue
		}

		if err := foundHandler(info); err != nil {
			if err.Error() == "Done" {
				return nil
			}
			return err
		}
	}
	return nil
}

func OpenDevice() (gohid.HIDDevice, error) {
	var device usb.Device
	err := SearchDevice(func(info usb.DeviceInfo) error {
		dev, err := info.Open()
		if err == nil {
			device = dev
			return errors.New("Done")
		}
		return err
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
		fmt.Printf("%s: ID %04x:%04x %s %s\n",
			info.Path, info.VendorID, info.ProductID, info.Manufacturer, info.Product)
		fmt.Println("Device Information:")
		fmt.Printf("\tPath         %s\n", info.Path)
		fmt.Printf("\tVendorID     %04x\n", info.VendorID)
		fmt.Printf("\tProductID    %04x\n", info.ProductID)
		fmt.Printf("\tSerial       %s\n", info.Serial)
		fmt.Printf("\tRelease      %x.%x\n", info.Release>>8, info.Release&0xff)
		fmt.Printf("\tManufacturer %s\n", info.Manufacturer)
		fmt.Printf("\tProduct      %s\n", info.Product)
		fmt.Printf("\tUsagePage    %#x\n", info.UsagePage)
		fmt.Printf("\tUsage        %#x\n", info.Usage)
		fmt.Printf("\tInterface    %d\n", info.Interface)
		fmt.Println()

		return nil
	})
}
