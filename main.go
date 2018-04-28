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

func Run(db *gorm.DB) error {
	conf := serial.Config{Name: "/dev/serial0", Baud: 115200}
	m, openErr := gogsmmodem.OpenSerial(&conf, true)
	if openErr != nil {
		return openErr
	}
	modem = m

	errorChannel := make(chan error, 1)
	defer close(errorChannel)

	go func() {
		for {
			for packet := range modem.OOB {
				fmt.Printf("%#v\n", packet)
				switch p := packet.(type) {
				case gogsmmodem.MessageNotification:
					fmt.Println("Message notification:", p)
					msg, err := modem.GetMessage(p.Index)
					if err != nil {
						errorChannel <- err
					}

					message := Message{
						ID:       uuid.New().String(),
						Number:   msg.Telephone,
						Body:     msg.Body,
						Incoming: true,
					}
					db.Create(&message)

					// handleErr := gmg.onReceive(
					// 	Contact(msg.Telephone),
					// 	msg.Body,
					// 	msg.Timestamp,
					// )
					// if handleErr != nil {
					// 	errorChannel <- handleErr
					// }

					fmt.Printf("Message from %s: %s\n", msg.Telephone, msg.Body)
					modem.DeleteMessage(p.Index)

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
