package gogsmmodem

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const BODY_PROMPT = "> "
const END_BODY = "\x1a"
const ESC = "\x1b"

type Modem struct {
	OOB          chan Packet
	port         io.ReadWriteCloser
	rx           chan Packet
	tx           chan string
	ready        chan bool
	initComplete bool
	config       *ModemConfig
}

type ModemConfig struct {
	startupTimeout time.Duration
	readTimeout    time.Duration
	debug          bool
}

func NewSerialModemConfig() *ModemConfig {
	return &ModemConfig{
		startupTimeout: 500 * time.Millisecond,
		readTimeout:    4 * time.Second,
		debug:          false,
	}
}

func NewModem(port io.ReadWriteCloser, config *ModemConfig) (*Modem, error) {
	if config.debug {
		port = LogReadWriteCloser{port}
	}

	oob := make(chan Packet, 16)
	rx := make(chan Packet)
	tx := make(chan string)
	ready := make(chan bool)

	modem := &Modem{
		OOB:    oob,
		config: config,
		port:   port,
		rx:     rx,
		tx:     tx,
		ready:  ready,
	}

	// run send/receive goroutine
	go modem.listen()

	if err := modem.init(); err != nil {
		return nil, err
	}
	return modem, nil
}

func (self *Modem) Close() error {
	close(self.OOB)
	close(self.rx)
	close(self.tx)
	close(self.ready)
	return self.port.Close()
}

// Commands

// GetMessage by index n from memory.
func (self *Modem) GetMessage(n int) (*Message, error) {
	packet, err := self.send(formatCommand("+CMGR", n))
	if err != nil {
		return nil, err
	}
	if msg, ok := packet.(Message); ok {
		msg.Index = n
		return &msg, nil
	}
	return nil, errors.New("Message not found")
}

// ListMessages stored in memory. Filter should be "ALL", "REC UNREAD", "REC READ", etc.
func (self *Modem) ListMessages(filter string) (*MessageList, error) {
	packet, err := self.send(formatCommand("+CMGL", filter))
	if err != nil {
		return nil, err
	}
	res := MessageList{}
	if _, ok := packet.(OK); ok {
		// empty response
		return &res, nil
	}

	for {
		if msg, ok := packet.(Message); ok {
			res = append(res, msg)
			if msg.Last {
				break
			}
		} else {
			return nil, errors.New("Unexpected error")
		}

		packet = <-self.rx
	}
	return &res, nil
}

func (self *Modem) SupportedStorageAreas() (*StorageAreas, error) {
	packet, err := self.send(formatCommand("+CPMS", "?"))
	if err != nil {
		return nil, err
	}
	if msg, ok := packet.(StorageAreas); ok {
		return &msg, nil
	}
	return nil, errors.New("Unexpected response type")
}

func (self *Modem) DeleteMessage(n int) error {
	_, err := self.send(formatCommand("+CMGD", n))
	return err
}

func (self *Modem) SendMessage(telephone, body string) error {
	enc := gsmEncode(body)
	_, err := self.sendBody("+CMGS", enc, telephone)
	return err
}

func lineChannel(r io.Reader) chan string {
	ret := make(chan string)
	go func() {
		buffer := bufio.NewReader(r)
		for {
			all := []byte{}
			line := ""
			for {
				b, e := buffer.ReadByte()
				if e == io.EOF {
					return
				}
				if e != nil {
					panic(e.Error())
				}

				all = append(all, b)
				line = string(all)
				if b == 10 || line == BODY_PROMPT {
					break
				}
			}

			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				continue
			}
			ret <- line
		}
	}()
	return ret
}

var reQuestion = regexp.MustCompile(`AT(\+[A-Z]+)`)

func parsePacket(status string, header string, body string) Packet {
	if header == "" && (status == "OK" || status == "ERROR") {
		if status == "OK" {
			return OK{}
		} else {
			return ERROR{}
		}
	}

	ls := strings.SplitN(header, ":", 2)
	if len(ls) != 2 {
		return UnknownPacket{header, []interface{}{}}
	}
	uargs := strings.TrimSpace(ls[1])
	args := unquotes(uargs)
	switch ls[0] {
	case "+ZUSIMR":
		// message storage unset nag, ignore
		return nil
	case "+ZPASR":
		return ServiceStatus{args[0].(string)}
	case "+ZDONR":
		return NetworkStatus{args[0].(string)}
	case "+CMTI":
		return MessageNotification{args[0].(string), args[1].(int)}
	case "+CSCA":
		return SMSCAddress{args}
	case "+CMGR":
		return Message{
			Status:    args[0].(string),
			Telephone: args[1].(string),
			Timestamp: parseTime(args[3].(string)),
			Body:      body,
		}
	case "+CMGL":
		return Message{
			Index:     args[0].(int),
			Status:    args[1].(string),
			Telephone: args[2].(string),
			Timestamp: parseTime(args[4].(string)),
			Body:      body,
			Last:      status != "",
		}
	case "+CPMS":
		s := uargs
		if strings.HasPrefix(s, "(") {
			// query response
			// ("A","B","C"),("A","B","C"),("A","B","C")
			s = strings.TrimPrefix(s, "(")
			s = strings.TrimSuffix(s, ")")
			areas := strings.SplitN(s, "),(", 3)
			return StorageAreas{
				stringsUnquotes(areas[0]),
				stringsUnquotes(areas[1]),
				stringsUnquotes(areas[2]),
			}
		} else {
			// set response
			// 0,100,0,100,0,100
			// get ints
			var iargs []int
			for _, arg := range args {
				if iarg, ok := arg.(int); ok {
					iargs = append(iargs, iarg)
				}
			}
			if len(iargs) != 6 {
				break
			}

			return StorageInfo{
				iargs[0], iargs[1], iargs[2], iargs[3], iargs[4], iargs[5],
			}
		}
	case "":
		if status == "OK" {
			return OK{}
		} else {
			return ERROR{}
		}
	}
	return UnknownPacket{ls[0], args}
}

func (self *Modem) listen() {
	in := lineChannel(self.port)
	var echo, last, header, body string
	for {
		select {
		case line := <-in:
			if line == echo {
				continue // ignore echo of command
			} else if last != "" && startsWith(line, last) {
				if header != "" {
					// first of multiple responses (eg CMGL)
					packet := parsePacket("", header, body)
					self.rx <- packet
				}
				header = line
				body = ""
			} else if line == "OK" || line == "ERROR" {
				packet := parsePacket(line, header, body)
				self.rx <- packet
				header = ""
				body = ""
			} else if header != "" {
				// the body following a header
				body += line
			} else if line == BODY_PROMPT {
				// raw mode for body
				self.rx <- BodyPrompt{}
			} else {
				// OOB packet
				p := parsePacket("OK", line, "")
				if p != nil {
					self.OOB <- p
				}
			}
		case line, more := <-self.tx:
			if !more {
				return
			}
			m := reQuestion.FindStringSubmatch(line)
			if len(m) > 0 {
				last = m[1]
			}
			echo = strings.TrimRight(line, "\r\n")
			self.port.Write([]byte(line))

		case <-time.After(self.config.startupTimeout):
			if !self.initComplete {
				self.ready <- true
			}
		}
	}
}

func formatCommand(cmd string, args ...interface{}) string {
	line := "AT" + cmd
	if len(args) > 0 {
		line += "=" + quotes(args)
	}
	line += "\r\n"
	return line
}

func (self *Modem) sendBody(cmd string, body string, args ...interface{}) (Packet, error) {
	commandResponse, commandErr := self.send(formatCommand(cmd, args...))
	if commandErr != nil {
		return nil, commandErr
	}

	_, isBodyPrompt := commandResponse.(BodyPrompt)
	if !isBodyPrompt {
		return commandResponse, errors.New(fmt.Sprintf("Expected body prompt, got %v", reflect.TypeOf(commandResponse)))
	}

	return self.send(body + END_BODY)
}

func (self *Modem) send(cmd string) (Packet, error) {
	self.tx <- cmd

	select {
	case response := <-self.rx:
		if _, e := response.(ERROR); e {
			return response, errors.New("Response was ERROR")
		}
		return response, nil
	case <-time.After(self.config.readTimeout):
		return nil, fmt.Errorf("Timed out waiting for response to %v", cmd)
	}
}

func (self *Modem) init() error {
	// wait for any outstanding reads to take place
	select {
	case r := <-self.ready:
		self.initComplete = true
		if !r {
			return fmt.Errorf("Not ready")
		}
	}

	// reset
	for i := 0; ; i++ {
		_, err := self.send(formatCommand("Z"))
		if err == nil { // successfully reset modem
			log.Println("Reset")
			break
		} else if i > 3 {
			return fmt.Errorf("Could not reset modem")
		}
		// send an escape character in case of hanging body
		log.Println("No answer to ATZ, sending escape")
		self.send(ESC)
	}

	// turn off echo
	if _, err := self.send(formatCommand("E0")); err != nil {
		return err
	}
	log.Println("Echo off")

	// use combined storage (MT)
	msg, err := self.send(formatCommand("+CPMS", "SM", "SM", "SM"))
	if err != nil {
		return err
	}
	sinfo := msg.(StorageInfo)
	log.Printf("Set SMS Storage: %d/%d used\n", sinfo.UsedSpace1, sinfo.MaxSpace1)

	// set SMS text mode - easiest to implement. Ignore response which is
	// often a benign error.
	self.send(formatCommand("+CMGF", 1))

	log.Println("Set SMS text mode")
	// get SMSC
	// the modem complains if SMSC hasn't been set, but stores it correctly, so
	// query for stored value, then send a set from the query response.
	r, err := self.send(formatCommand("+CSCA?"))
	if err != nil {
		return err
	}
	smsc := r.(SMSCAddress)
	log.Println("Got SMSC:", smsc.Args)
	r, err = self.send(formatCommand("+CSCA", smsc.Args...))
	if err != nil {
		return err
	}
	log.Println("Set SMSC to:", smsc.Args)
	return nil
}
