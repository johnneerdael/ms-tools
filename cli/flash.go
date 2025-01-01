package main

import (
	"fmt"
)

type FlashMemoryRegion struct {
	dev *hidDeviceWrapper
}

func (r *FlashMemoryRegion) GetLength() int {
	return 0x10000 // 64KB
}

func (r *FlashMemoryRegion) Access(write bool, addr int, buf []byte) (int, error) {
	if addr >= r.GetLength() {
		return 0, fmt.Errorf("address out of range")
	}

	if write {
		return 0, fmt.Errorf("flash write not implemented yet")
	}

	// Read in pages
	total := 0
	for len(buf) > 0 {
		page := uint16(addr >> 8)
		offset := uint8(addr & 0xff)

		n, err := r.dev.readFlashPage(page, offset, buf)
		if err != nil {
			return total, err
		}
		if n == 0 {
			break
		}

		total += n
		addr += n
		buf = buf[n:]
	}

	return total, nil
}

func (r *FlashMemoryRegion) GetParent() (interface{}, int) {
	return nil, 0
}

func (r *FlashMemoryRegion) GetName() string {
	return "FLASH"
}

func (r *FlashMemoryRegion) GetAlignment() int {
	return 1
}
