package main

import (
	"bytes"
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
	ID        string    `gorm:"primary_key,size:32" json:"id"`
	Number    string    `gorm:"size:32" json:"number"`
	Body      string    `gorm:"size:160" json:"body"`
	Incoming  bool      `gorm:"index" json:"incoming"`
	Handled   bool      `gorm:"index" json:"-"`
	Time      time.Time `json:"time"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
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
		Time:     msg.Timestamp,
	}
	db.Create(&message)
	deleteErr := modem.DeleteMessage(msg.Index)
	if deleteErr != nil {
		return deleteErr
	}

	str, marshalErr := json.Marshal(&message)
	if marshalErr != nil {
		return marshalErr
	}

	res, err := http.Post("http://localhost", "application/json", bytes.NewBuffer(str))
	if err != nil {
		return err
	}

	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		message.Handled = true
		db.Update(&message)
	}

	return nil
}

func Run(db *gorm.DB, device string) error {
	port, portErr := serial.OpenPort(&serial.Config{Name: device, Baud: 115200})
	if portErr != nil {
		panic(portErr)
	}

	m, modemErr := gogsmmodem.NewModem(port, gogsmmodem.NewSerialModemConfig())
	if modemErr != nil {
		panic(modemErr)
	}
	modem = m

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

func Serve(db *gorm.DB, port string) error {
	http.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
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
			m.ID = uuid.New().String()
			m.Time = time.Now()

			db.Create(&m)

			err = modem.SendMessage(m.Number, m.Body)
			if err != nil {
				http.Error(w, "500 Failed to send.", http.StatusInternalServerError)
				return
			}

			m.Handled = true
			db.Save(&m)

			w.WriteHeader(http.StatusOK)
			str, _ := json.Marshal(m)
			w.Write([]byte(str))
		default:
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}
	})

	return http.ListenAndServe(":"+port, nil)
}

func main() {
	errorChannel := make(chan error)

	device := os.Getenv("DEVICE")
	port := os.Getenv("PORT")
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
		errorChannel <- Run(db, device)
	}()

	go func() {
		errorChannel <- Serve(db, port)
	}()

	select {
	case err := <-errorChannel:
		panic(err)
	}
}
