package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/barnybug/gogsmmodem"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
)

func CreateIncomingMessageHandler(db *gorm.DB, modem *gogsmmodem.Modem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			// read struct
			decoder := json.NewDecoder(r.Body)
			var m Message
			err := decoder.Decode(&m)
			defer r.Body.Close()
			if err != nil {
				http.Error(w, "400 Bad request.", http.StatusBadRequest)
				return
			}

			// save to db
			m.Incoming = false
			m.ID = uuid.New().String()
			m.Time = time.Now().UTC()
			db.Create(&m)

			// send
			err = modem.SendMessage(m.Number, m.Body)
			if err != nil {
				http.Error(w, "500 Failed to send.", http.StatusInternalServerError)
				return
			}

			// mark handled and update in db
			m.Handled = true
			db.Save(&m)

			// respond to http request
			w.WriteHeader(http.StatusOK)
			str, _ := json.Marshal(m)
			w.Write([]byte(str))
		default:
			http.Error(w, "404 not found.", http.StatusNotFound)
			return
		}
	}
}

func ListenOnHTTP(db *gorm.DB, modem *gogsmmodem.Modem, port string) chan error {
	errorChannel := make(chan error, 1)

	http.HandleFunc("/api/messages", CreateIncomingMessageHandler(db, modem))

	for {
		err := http.ListenAndServe(":"+port, nil)
		errorChannel <- err
	}
}
