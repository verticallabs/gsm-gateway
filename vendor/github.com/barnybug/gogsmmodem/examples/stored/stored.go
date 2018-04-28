// Example of retrieving stored messages.
package main

import (
	"fmt"

	"github.com/barnybug/gogsmmodem"
	"github.com/tarm/serial"
)

func main() {
	port, portErr := serial.OpenPort(&serial.Config{Name: "/dev/ttyUSB1", Baud: 115200})
	if portErr != nil {
		panic(portErr)
	}

	modem, modemErr := gogsmmodem.NewModem(port, gogsmmodem.NewSerialModemConfig())
	if modemErr != nil {
		panic(modemErr)
	}

	li, _ := modem.ListMessages("ALL")
	fmt.Printf("%d stored messages\n", len(*li))
	for _, msg := range *li {
		fmt.Println(msg.Index, msg.Status, msg.Body)
	}
}
