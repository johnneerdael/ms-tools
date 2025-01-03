package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	_ "embed"

	"github.com/johnneerdael/ms-tools/mshal"
)

type DumpROM struct {
	Filename string `arg help:"File to write dump to."`
}

type dumpCodeParams struct {
	addrMailbox int
	addrTemp    int
	addrTempLen int
	addrLoad    int
	addrHook    int
	valueHook   byte
}

func (d *DumpROM) Run(c *Context) error {
	var p dumpCodeParams

	devType := c.hal.GetDeviceType()
	if strings.Contains(devType, "MS2106") {
		p.addrMailbox = 0xCF10
		p.addrTemp = 0xCD00
		p.addrTempLen = 256
		p.addrLoad = 0xC4A0
		p.addrHook = 9
		p.valueHook = 0x96
	} else if strings.Contains(devType, "MS2107") {
		p.addrMailbox = 0xD000
		p.addrTemp = 0xD100
		p.addrTempLen = 256
		p.addrLoad = 0xC800
		p.addrHook = 8
		p.valueHook = 1
	} else if strings.Contains(devType, "MS2109") {
		p.addrMailbox = 0xCBF0
		p.addrTemp = 0xD300
		p.addrTempLen = 256
		p.addrLoad = 0xCC20
		p.addrHook = 4
		p.valueHook = 1 << 2
	} else if strings.Contains(devType, "MS2130") {
		p.addrMailbox = 0x7C00
		p.addrTemp = 0x7D00
		p.addrTempLen = 256
		p.addrHook = 0x7D00
		p.addrHook = 8
		p.valueHook = 1
	} else {
		return mshal.ErrorUnknownDevice
	}

	// Convert to original HAL type for compatibility
	origHal := (*mshal.HAL)(c.hal)
	code, err := d.work(origHal, p)
	if err != nil {
		return err
	}

	return os.WriteFile(d.Filename, code, 0644)
}

//go:embed asm/dumprom.bin
var dumpBlobBase []byte

func (d *DumpROM) work(ms *mshal.HAL, p dumpCodeParams) ([]byte, error) {
	dumpBlob := bytes.Replace(dumpBlobBase, []byte{0xDE, 0xAD}, []byte{byte(p.addrMailbox >> 8), byte(p.addrMailbox)}, -1)

	tmpBufLen := 1 + int(0xFF-byte(p.addrTemp))
	if tmpBufLen > p.addrTempLen {
		tmpBufLen = p.addrTempLen
	}
	if tmpBufLen > 255 {
		tmpBufLen = 255
	}

	config := ms.MemoryRegionGet(mshal.MemoryRegionUserConfig)
	configOld := make([]byte, config.GetLength())
	configNew := make([]byte, config.GetLength())

	/* Read orig hooks */
	if _, err := config.Access(false, 0, configOld); err != nil {
		return nil, err
	}

	/* Disable all userhooks */
	if _, err := config.Access(true, 0, configNew); err != nil {
		return nil, err
	}

	/* Disable reading */
	xdata := ms.MemoryRegionGet(mshal.MemoryRegionRAM)
	if err := mshal.WriteByte(xdata, p.addrMailbox, 0); err != nil {
		return nil, err
	}

	/* Read original code */
	orig := make([]byte, len(dumpBlob))
	_, err := xdata.Access(false, p.addrLoad, orig)
	if err != nil {
		return nil, nil
	}

	/* Ensure CPU left the affected area */
	time.Sleep(time.Second)

	/* Write new code */
	if p.addrLoad > 0 {
		if _, err := xdata.Access(true, p.addrLoad, dumpBlob); err != nil {
			return nil, err
		}
	}

	/* Enable USB/Periodic hook */
	if err := mshal.WriteByte(config, p.addrHook, p.valueHook); err != nil {
		return nil, err
	}

	buf := make([]byte, 65536)

	addr := 0
	lastAddr := addr + len(buf)
	index := 0

	for {
		config := []byte{byte(addr >> 8), byte(addr), byte(p.addrTemp >> 8), byte(p.addrTemp)}
		remaining := lastAddr - addr
		if remaining == 0 {
			break
		}
		if remaining > tmpBufLen {
			remaining = tmpBufLen
		}

		if _, err := xdata.Access(true, p.addrMailbox+1, config); err != nil {
			return nil, err
		}

		if err := mshal.WriteByte(xdata, p.addrMailbox, byte(remaining)); err != nil {
			return nil, err
		}

		timeout := time.Now().Add(500 * time.Millisecond)
		for {
			ack, err := mshal.ReadByte(xdata, p.addrMailbox)
			if err != nil {
				return nil, err
			}

			if ack != 0 {
				if time.Now().After(timeout) {
					return nil, mshal.ErrorPatchFailed
				}
			} else {
				break
			}

			time.Sleep(20 * time.Millisecond)
		}

		_, err = xdata.Access(false, p.addrTemp, buf[index:(index+remaining)])
		if err != nil {
			return nil, err
		}

		addr += remaining
		index += remaining
		fmt.Printf("Dumping code: %d bytes read.\n", addr)
	}

	/* Disable USB hook */
	if err := mshal.WriteByte(config, p.addrHook, 0); err != nil {
		return nil, err
	}

	/* Ensure CPU left code */
	time.Sleep(25 * time.Millisecond)

	/* Put original code back */
	if p.addrLoad > 0 {
		/* Remove overwritten code from dump */
		buf = bytes.ReplaceAll(buf, dumpBlob, orig)

		if _, err := xdata.Access(true, p.addrLoad, orig); err != nil {
			return nil, err
		}
	}

	/* Re-enable old hooks */
	_, err = config.Access(true, 0, configOld)
	return buf, err
}
