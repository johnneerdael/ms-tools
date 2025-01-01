module github.com/johnneerdael/ms-tools/cli

go 1.21

require (
	github.com/alecthomas/kong v0.8.1
	github.com/fatih/color v1.18.0
	github.com/inancgumus/screen v0.0.0-20190314163918-06e984b86ed3
	github.com/karalabe/usb v0.0.2
	github.com/sigurn/crc16 v0.0.0-20240131213347-83fcde1e29d1
	golang.org/x/sys v0.28.0
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/term v0.27.0 // indirect
)

replace github.com/johnneerdael/ms-tools => ../
