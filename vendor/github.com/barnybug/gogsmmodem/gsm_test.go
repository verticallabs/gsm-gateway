package gogsmmodem

import (
	"fmt"
	"io"
	"log"
	"reflect"
	"testing"
	"time"

	"github.com/tarm/serial"
)

func appendLists(ls ...[]string) []string {
	size := 0
	for _, l := range ls {
		size += len(l)
	}
	ret := make([]string, size)
	off := ret
	for _, l := range ls {
		copy(off, l)
		off = off[len(l):]
	}
	return ret
}

var initReplay = []string{
	"->ATZ\r\n",
	"<-\r\nOK\r\n",
	"->ATE0\r\n",
	"<-ATE0\n",
	"<-\r\nOK\r\n",
	"->AT+CPMS=\"SM\",\"SM\",\"SM\"\r\n",
	"<-\r\n+CPMS: 50,50,50,50,50,50\r\nOK\n\n",
	"->AT+CMGF=1\r\n",
	"<-\r\nOK\r\n",
	"->AT+CSCA?\r\n",
	"<-\r\n+CSCA: \"+447802092035\",145\r\nOK\r\n",
	"->AT+CSCA=\"+447802092035\",145\r\n",
	"<-\r\nOK\r\n",
}

func TestNormalInit(t *testing.T) {
	replay := appendLists(initReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	modem.Close()
	//fmt.Printf("%v %v\n", mock.position, len(mock.replay))
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var hangingPromptReplay = []string{
	"->ATZ\r\n",
	"->\x1b",
	"<-\r\nOK\r\n",
}

func TestHangingPromptInit(t *testing.T) {
	replay := appendLists(hangingPromptReplay, initReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	modem.Close()
	//fmt.Printf("%v %v\n", mock.position, len(mock.replay))
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

func assertOOBCommands(t *testing.T, modem *Modem, commands []Packet) {
	for i := range modem.OOB {
		if len(commands) == 0 {
			t.Errorf("Unexpected extra command: %#v", i)
			break
		}
		head := commands[0]
		if !reflect.DeepEqual(i, head) {
			t.Errorf("Expected: %#v, got: %#v", head, i)
		}
		commands = commands[1:]
	}
	if len(commands) > 0 {
		t.Errorf("Expected: %d more commands", len(commands))
	}

}

var oobReplay = []string{
	"<-\r\n+ZUSIMR:2\r\n",
	"<-\r\n+ZPASR: \"No Service\"\r\n",
	"<-\r\n+ZDONR: \"O2-UK\",234,10,\"CS_PS\",\"ROAM_OFF\"\r\n",
	"<-\r\n+ZPASR: \"EDGE\"\r\n",
	"<-\r\n+ZPASR: \"UMTS\"\r\n",
	"<-\r\nDODGY\r\n",
	"<-\r\n+ZZZ: \"A\"\r\n",
}

var oobCommands = []Packet{
	ServiceStatus{"No Service"},
	NetworkStatus{"O2-UK"},
	ServiceStatus{"EDGE"},
	ServiceStatus{"UMTS"},
	UnknownPacket{"DODGY", []interface{}{}},
	UnknownPacket{"+ZZZ", []interface{}{"A"}},
}

func TestOOB(t *testing.T) {
	replay := appendLists(oobReplay, initReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	modem.Close()
	assertOOBCommands(t, modem, oobCommands)
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var receivedReplay = []string{
	"<-\r\n+CMTI: \"SM\",5\r\n",
}

var receivedCommands = []Packet{
	MessageNotification{"SM", 5},
}

func TestIncoming(t *testing.T) {
	replay := appendLists(initReplay, receivedReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	modem.Close()
	assertOOBCommands(t, modem, receivedCommands)
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var messageReplay = []string{
	"->AT+CMGR=1\r\n",
	"<-\r\n+CMGR: \"REC UNREAD\",\"+441234567890\",,\"14/02/01,15:07:43+00\"\r\nHi\r\n\r\nOK\r\n",
}

func TestGetMessage(t *testing.T) {
	replay := appendLists(initReplay, messageReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, _ := modem.GetMessage(1)
	expected := Message{0, "REC UNREAD", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Hi", false}
	if *msg != expected {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}

	modem.Close()
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var missingMessageReplay = []string{
	"->AT+CMGR=1\r\n",
	"<-\r\nOK\r\n",
}

func TestGetMissingMessage(t *testing.T) {
	replay := appendLists(initReplay, missingMessageReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	_, err = modem.GetMessage(1)
	if fmt.Sprint(err) != "Message not found" {
		t.Errorf("Expected error: %#v, got %#v", err, err)
	}

	modem.Close()
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var sendMessageReplay = []string{
	"->AT+CMGS=\"441234567890\"\r\n",
	"<-> \r\n",
	"->Body\x00\x1a",
	"<-\r\nOK\r\n",
}

func TestSendMessage(t *testing.T) {
	replay := appendLists(initReplay, sendMessageReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	err = modem.SendMessage("441234567890", "Body@")
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	modem.Close()
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var listMessagesReplay = []string{
	"->AT+CMGL=\"ALL\"\r\n",
	"<-\r\n+CMGL: 0,\"REC UNREAD\",\"+441234567890\",,\"14/02/01,15:07:43+00\"\r\nHi\r\n+CMGL: 1,\"REC READ\",\"+441234567890\",,\"14/02/01,15:07:43+00\"\r\nOla\r\n+CMGL: 2,\"REC UNREAD\",\"+44123456",
	"<-7890\",,\"14/02/01,15:07:43+00\"\r\nJa\r\n\r\nOK\r\n",
}

func TestListMessages(t *testing.T) {
	replay := appendLists(initReplay, listMessagesReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, _ := modem.ListMessages("ALL")
	expected := MessageList{
		Message{0, "REC UNREAD", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Hi", false},
		Message{1, "REC READ", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Ola", false},
		Message{2, "REC UNREAD", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Ja", true},
	}
	if len(*msg) != len(expected) {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}
	for i, m := range *msg {
		if m != expected[i] {
			t.Errorf("Expected: %#v, got %#v", expected, msg)
		}
	}

	modem.Close()
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var listMessagesEmptyReplay = []string{
	"->AT+CMGL=\"ALL\"\r\n",
	"<-\r\nOK\r\n",
}

func TestListMessagesEmpty(t *testing.T) {
	replay := appendLists(initReplay, listMessagesEmptyReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, err := modem.ListMessages("ALL")
	log.Println("ERROR", err)
	expected := MessageList{}
	if len(*msg) != len(expected) {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}

	modem.Close()
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}

var storageAreasReplay = []string{
	"->AT+CPMS=?\r\n",
	"<-\r\n+CPMS: (\"ME\",\"MT\",\"SM\",\"SR\"),(\"ME\",\"MT\",\"SM\",\"SR\"),(\"ME\",\"MT\",\"SM\",\"SR\")\r\n\r\nOK\r\n",
}

func TestSupportedStorageAreas(t *testing.T) {

	replay := appendLists(initReplay, storageAreasReplay)
	mock := NewMockSerialPort(replay)
	modem, err := Open(&serial.Config{}, false, func(config *serial.Config) (io.ReadWriteCloser, error) {
		return mock, nil
	})

	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, _ := modem.SupportedStorageAreas()
	expected := StorageAreas{
		[]string{"ME", "MT", "SM", "SR"},
		[]string{"ME", "MT", "SM", "SR"},
		[]string{"ME", "MT", "SM", "SR"},
	}
	if fmt.Sprint(*msg) != fmt.Sprint(expected) {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}

	modem.Close()
	if !mock.Done() {
		t.Errorf("Incomplete replay: remaining %v", mock.replay[mock.position])
	}

}
