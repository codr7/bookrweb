package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
)

type Record = map[string]interface{}
type List = []interface{}

type Request struct {
	id string
	fields Record
	response Record
	mutex sync.Mutex
	done *sync.Cond
}

func NewRequest(id string) *Request {
	r := new(Request)
	r.id = id
	r.fields = make(Record)
	r.done = sync.NewCond(&r.mutex)
	return r
}

func (self *Request) Write(out io.Writer) error {
	fmt.Fprintln(out, self.id)
	fmt.Fprintln(out, "")
	return nil
}

func (self *Request) Wait() Record {
	self.mutex.Lock()

	for self.response == nil {
		self.done.Wait()
	}

	return self.response
}

func ReadLine(in *bufio.Reader) (string, error) {
	s, err := in.ReadString('\n')
	return strings.TrimRight(s, "\r\n"), err
}

func ReadRecord(in *bufio.Reader, end string) (Record, error) {
	out := make(Record)

	for {
		var f string
		var v interface{}
		var err error
		
		if f, err = ReadLine(in); err != nil {
			return nil, err
		}

		if f == end {
			break
		}

		if v, err = ReadValue(in); err != nil {
			return nil, err
		}

		out[f] = v
	}
	
	return out, nil
}

func ReadList(in *bufio.Reader) (List, error) {
	var out []interface{}

	for {
		v, err := ReadValue(in)
		
		if err != nil {
			return nil, err
		}
		
		if v == "]" {
			break
		}
	}
	
	return out, nil
}

func ReadValue(in *bufio.Reader) (interface{}, error) {
	s, err := ReadLine(in)

	if err != nil {
		return nil, err
	}

	switch s {
	case "{":
		return ReadRecord(in, "}")
	case "[":
		return ReadList(in)
	default:
		break
	}
	
	return s, nil
}

func ReadResponse(in *bufio.Reader) (Record, error) {
	return ReadRecord(in, "")
}

type Server struct {
	queue chan *Request
	active sync.WaitGroup
}

func NewServer() *Server {
	s := new(Server)
	s.queue = make(chan *Request)
	return s
}

func (self *Server) Start() error {
	cmd := exec.Command("bookr.exe")
	
	var _in io.ReadCloser
	var out io.WriteCloser
	var err error
	
	if out, err = cmd.StdinPipe(); err != nil {
		return fmt.Errorf("Failed opening STDIN: %v", err)
	}

	if _in, err = cmd.StdoutPipe(); err != nil {
		return fmt.Errorf("Failed opening STDOUT: %v", err)
	}
	
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("Failed starting: %v", err)
	}

	in := bufio.NewReader(_in)
	self.active.Add(1)
	
	go func() {
		for {
			req, ok := <-self.queue

			if !ok {
				break
			}

			req.mutex.Lock()
			
			if err := req.Write(out); err != nil {
				log.Fatalf("Failed writing request: %v", err)
			}
			
			res, err := ReadResponse(in)

			if err != nil {
				log.Fatal(err)
			}

			req.response = res
			req.mutex.Unlock()
			req.done.Signal()
		}
		
		fmt.Fprintln(out, "quit\n")
		_in.Close()
		out.Close()
		cmd.Wait()
		self.active.Done()
	}()
	
	return nil
}

func (self *Server) Send(req *Request) *Request{
	self.queue<- req
	return req
}

func (self *Server) Stop() {
	close(self.queue)
	self.active.Wait()
}

func main() {
	log.Println("Starting backend...")
	backend := NewServer()

	if err := backend.Start(); err != nil {
		log.Fatalf("Failed starting backend: %v", err)
	}

	req := backend.Send(NewRequest("resources"))
	req.Wait()

	log.Println("Stopping backend...")
	backend.Stop()
}
