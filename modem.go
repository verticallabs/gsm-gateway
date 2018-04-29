package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/barnybug/gogsmmodem"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
)

func saveAndDelete(db *gorm.DB, modem *gogsmmodem.Modem, msg *gogsmmodem.Message, notificationUrl string) error {
	message := Message{
		ID:       uuid.New().String(),
		Number:   msg.Telephone,
		Body:     msg.Body,
		Incoming: true,
		Time:     readTimeAsUTC(msg.Timestamp, time.Now().Location()),
	}

	// store message
	db.Create(&message)
	deleteErr := modem.DeleteMessage(msg.Index)
	if deleteErr != nil {
		return deleteErr
	}

	str, marshalErr := json.Marshal(&message)
	if marshalErr != nil {
		return marshalErr
	}

	// post result
	res, err := http.Post(notificationUrl, "application/json", bytes.NewBuffer(str))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		// update handled
		message.Handled = true
		db.Save(&message)
	}

	return nil
}

func listenOnModem(db *gorm.DB, modem *gogsmmodem.Modem, notificationUrl string) chan error {
	errorChannel := make(chan error, 1)

	go func() {
		// retrieve old messages
		msgs, err := modem.ListMessages("ALL")
		if err != nil {
			errorChannel <- err
			return
		}

		for _, msg := range []gogsmmodem.Message(*msgs) {
			err := saveAndDelete(db, modem, &msg, notificationUrl)
			if err != nil {
				errorChannel <- err
			}
		}

		for {
			for packet := range modem.OOB {
				switch p := packet.(type) {
				case gogsmmodem.MessageNotification:
					msg, err := modem.GetMessage(p.Index)
					if err != nil {
						errorChannel <- err
						continue
					}
					log.Printf("Received message %v: %v\n", msg.Telephone, msg.Body)

					saveErr := saveAndDelete(db, modem, msg, notificationUrl)
					if saveErr != nil {
						errorChannel <- saveErr
						continue
					}
				}
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()

	return errorChannel
}
