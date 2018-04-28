package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

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
	portName := os.Getenv("PORT")
	port, portErr := serial.OpenPort(&serial.Config{Name: portName, Baud: 115200})
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

func Serve(db *gorm.DB) error {
	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			decoder := json.NewDecoder(r.Body)
			var m Message
			err := decoder.Decode(&m)
			defer r.Body.Close()
			if err != nil {
				http.Error(w, "400 Bad request.", http.StatusBadRequest)
				return
			}
			m.Incoming = false
			m.ID = ""

			db.Create(&m)
			w.WriteHeader(http.StatusOK)
			str, _ := json.Marshal(m)
			w.Write([]byte(str))
		default:
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}
	})

	return http.ListenAndServe(":80", nil)
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

	go func() {
		err := Run(db)
		if err != nil {
			panic(err.Error())
		}
	}()

	go func() {
		err := Serve(db)
		if err != nil {
			panic(err.Error())
		}
	}()
}
