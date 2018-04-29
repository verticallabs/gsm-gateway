package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/barnybug/gogsmmodem"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/tarm/serial"
)

func main() {
	device := os.Getenv("DEVICE")
	port := os.Getenv("PORT")
	notificationUrl := os.Getenv("NOTIFICATION_URL")

	pgHost := os.Getenv("PGHOST")
	pgUser := os.Getenv("PGUSER")
	pgPassword := os.Getenv("PGPASSWORD")
	pgDatabase := os.Getenv("PGDATABASE")

	pgConnectionString := fmt.Sprintf("postgresql://%v:%v@%v/%v?sslmode=disable", pgUser, pgPassword, pgHost, pgDatabase)

	log.Printf("Initializing gateway with time zone %v", time.Now().Location().String())

	// set up db
	log.Printf("Connecting to db at %v", pgConnectionString)
	db, dbErr := gorm.Open("postgres", pgConnectionString)
	if dbErr != nil {
		panic(dbErr.Error())
	}
	defer db.Close()
	db.AutoMigrate(&Message{})

	// set up modem
	serialPort, portErr := serial.OpenPort(&serial.Config{Name: device, Baud: 115200})
	if portErr != nil {
		panic(portErr)
	}
	modem, modemErr := gogsmmodem.NewModem(serialPort, gogsmmodem.NewSerialModemConfig())
	if modemErr != nil {
		panic(modemErr)
	}
	defer modem.Close()

	modemError := listenOnModem(db, modem, notificationUrl)
	defer close(modemError)

	httpError := listenOnHTTP(db, modem, port)
	defer close(httpError)

	for {
		select {
		case err := <-modemError:
			log.Println(err.Error())
		case err := <-httpError:
			log.Println(err.Error())
		}
	}
}
