package gogsmmodem

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func isRead(str string) bool {
	return strings.Index(str, "<-") == 0
}

func isWrite(str string) bool {
	return strings.Index(str, "->") == 0
}

type MockSerialPort struct {
	replay        []string
	enqueuedReads chan string
	position      int
}

func NewMockSerialPort(replay []string) *MockSerialPort {
	self := &MockSerialPort{
		replay:        replay,
		enqueuedReads: make(chan string, 16),
		position:      0,
	}
	self.EnqueueReads()
	return self
}

func (self *MockSerialPort) Done() bool {
	return self.position >= len(self.replay)
}

func (self *MockSerialPort) Print() {
	for i, s := range self.replay {
		if i == self.position {
			fmt.Printf("* ")
		} else {
			fmt.Printf("  ")
		}
		fmt.Printf("%v\n", s)
	}
}

func (self *MockSerialPort) EnqueueReads() {
	// enqueue response(s) from replay
	readPosition := self.position
	//fmt.Printf("enqueing check %v\n", self.replay[readPosition])
	for {
		if readPosition >= len(self.replay) || isWrite(self.replay[readPosition]) {
			//fmt.Printf("skipping %v\n", readPosition)
			break
		}
		s := self.replay[readPosition]
		//fmt.Printf("enqueuing %v\n", s)
		self.enqueuedReads <- s
		readPosition++
	}
}

func (self *MockSerialPort) Read(b []byte) (int, error) {
	var result string
	select {
	case <-time.After(5 * time.Second):
		fmt.Printf("Read: timeout waiting for read\n")
		panic("fail")
	case r, ok := <-self.enqueuedReads:
		if !ok {
			return 0, io.EOF
		}
		result = r
		// something ready to read
	}
	//fmt.Printf("READ %v\n", result)

	if self.position >= len(self.replay) {
		fmt.Printf("Read: expected no more interactions, got: %#v\n", string(b))
		panic("fail")
	}

	expected := self.replay[self.position]
	if result != expected {
		fmt.Printf("Read: expected read and read mismatch: %v, %v\n", expected, result)
		panic("fail")
	}

	readData := []byte(expected[2:])
	self.position = self.position + 1

	copy(b, readData)
	return len(readData), nil
}

func (self *MockSerialPort) Write(b []byte) (int, error) {
	//fmt.Printf("WRITE %v\n", string(b))
	if self.position >= len(self.replay) {
		fmt.Printf("Write: expected no more interactions, got: %#v\n", string(b))
		panic("fail")
	}

	expected := self.replay[self.position]
	if isRead(expected) {
		fmt.Printf("Write: expected read %v, got write	: %#v\n", expected, string(b))
		panic("fail")
	}

	if expected[2:] != string(b) {
		fmt.Printf("Write: expected %#v got: %#v\n", expected, string(b))
		panic("fail")
	}
	self.position = self.position + 1
	self.EnqueueReads()

	return len(b), nil
}

func (self *MockSerialPort) Close() error {
	close(self.enqueuedReads)
	return nil
}
