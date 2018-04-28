package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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

func convertTime(local time.Time) time.Time {
	location := time.Now().Location()
	str := local.Format("06/01/02,15:04:05")
	t, _ := time.ParseInLocation("06/01/02,15:04:05", str, location)
	return t
}

func createAndDelete(db *gorm.DB, modem *gogsmmodem.Modem, msg *gogsmmodem.Message, notificationUrl string) error {
	message := Message{
		ID:       uuid.New().String(),
		Number:   msg.Telephone,
		Body:     msg.Body,
		Incoming: true,
		Time:     convertTime(msg.Timestamp),
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

	res, err := http.Post(notificationUrl, "application/json", bytes.NewBuffer(str))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		message.Handled = true
		db.Save(&message)
	}

	return nil
}

func ListenOnModem(db *gorm.DB, modem *gogsmmodem.Modem, notificationUrl string) chan error {
	errorChannel := make(chan error, 1)

	go func() {
		msgs, err := modem.ListMessages("ALL")
		if err != nil {
			errorChannel <- err
			return
		}

		for _, msg := range []gogsmmodem.Message(*msgs) {
			err := createAndDelete(db, modem, &msg, notificationUrl)
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
						err := createAndDelete(db, modem, msg, notificationUrl)
						if err != nil {
							errorChannel <- err
						}
					}

				}
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()

	return errorChannel
}

func ListenOnHTTP(db *gorm.DB, modem *gogsmmodem.Modem, port string) chan error {
	errorChannel := make(chan error, 1)

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
			m.Time = time.Now().UTC()

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

	for {
		err := http.ListenAndServe(":"+port, nil)
		errorChannel <- err
	}
}

func main() {
	device := os.Getenv("DEVICE")
	port := os.Getenv("PORT")
	notificationUrl := os.Getenv("NOTIFICATION_URL")

	pgUser := os.Getenv("PGUSER")
	pgPassword := os.Getenv("PGPASSWORD")
	pgDatabase := os.Getenv("PGDATABASE")

	pgConnectionString := fmt.Sprintf("postgresql://%v:%v@127.0.0.1/%v?sslmode=disable", pgUser, pgPassword, pgDatabase)

	// set up db
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

	go func() {
		modemError := ListenOnModem(db, modem, notificationUrl)
		defer close(modemError)
		select {
		case err := <-modemError:
			log.Println(err.Error())
		}
	}()

	go func() {
		httpError := ListenOnHTTP(db, modem, port)
		defer close(httpError)
		select {
		case err := <-httpError:
			log.Println(err.Error())
		}
	}()

	select {}
}
