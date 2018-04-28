package main

import (
	"fmt"
	"os"
	"time"

	//"github.com/google/uuid"
	"github.com/barnybug/gogsmmodem"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/tarm/serial"
)

type Message struct {
	ID        string `gorm:"primary_key,size:32"`
	Number    string `gorm:"size:32"`
	Body      string `gorm:"size:160"`
	Incoming  bool   `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

var modem *gogsmmodem.Modem

func Send(to, body string) error {
	if modem == nil {
		return fmt.Errorf("modem not initialized")
	}

	return modem.SendMessage(to, body)
}

func createAndDelete(db *gorm.DB, modem *gogsmmodem.Modem, msg *gogsmmodem.Message) error {
	message := Message{
		ID:       uuid.New().String(),
		Number:   msg.Telephone,
		Body:     msg.Body,
		Incoming: true,
	}
	db.Create(&message)

	//fmt.Printf("Message from %s: %s\n", msg.Telephone, msg.Body)
	return modem.DeleteMessage(msg.Index)
}

func Run(db *gorm.DB) error {
	port, portErr := serial.OpenPort(&serial.Config{Name: "/dev/ttyUSB1", Baud: 115200})
	if portErr != nil {
		panic(portErr)
	}

	modem, modemErr := gogsmmodem.NewModem(port, gogsmmodem.NewSerialModemConfig())
	if modemErr != nil {
		panic(modemErr)
	}

	errorChannel := make(chan error, 1)
	defer close(errorChannel)

	go func() {
		msgs, err := modem.ListMessages("ALL")
		if err != nil {
			errorChannel <- err
			return
		}

		for _, msg := range []gogsmmodem.Message(*msgs) {
			err := createAndDelete(db, modem, &msg)
			if err != nil {
				errorChannel <- err
			}
		}

		for {
			for packet := range modem.OOB {
				fmt.Printf("%#v\n", packet)
				switch p := packet.(type) {
				case gogsmmodem.MessageNotification:
					if msg, err := modem.GetMessage(p.Index); err != nil {
						errorChannel <- err
						return
					} else {
						msg.Index = p.Index
						err := createAndDelete(db, modem, msg)
						if err != nil {
							errorChannel <- err
						}
					}

				}
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()

	select {
	case err := <-errorChannel:
		return err
	}
}

func main() {
	pgUser := os.Getenv("PGUSER")
	pgPassword := os.Getenv("PGPASSWORD")
	pgDatabase := os.Getenv("PGDATABASE")
	pgConnectionString := fmt.Sprintf("postgresql://%v:%v@127.0.0.1/%v?sslmode=disable", pgUser, pgPassword, pgDatabase)
	fmt.Println(pgConnectionString)
	db, dbErr := gorm.Open("postgres", pgConnectionString)
	if dbErr != nil {
		panic(dbErr.Error())
	}
	defer db.Close()

	db.AutoMigrate(&Message{})

	runErr := Run(db)
	if runErr != nil {
		panic(runErr.Error())
	}
}
