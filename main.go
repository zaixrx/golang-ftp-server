package main

import (
	"fmt"
	"net"
	"log"
	"strings"
	"os/exec"
	"os"
	"time"
	"slices"
)

const (
	Port = "8080"
)

type MessageType int

const (
	ConnectedMessage MessageType = iota
	UserCommandMessage
	DisconnectedMessage
)

func main() {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", Port))
	if err != nil {
		log.Fatalf("Couldn't listen on port %s:%s\n", Port, err)
	}
	log.Printf("Server listening on port %s\n", Port)
	appState := AppState{
		banned: make([]string, 0),	
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Couldn't accept connection from %s\n", conn.RemoteAddr().String())
		}
		handleConnection(conn, &appState)
	}
}

type Client struct {
	conn net.Conn
	id string
}

type Message struct {
	typ MessageType 
	data []byte
	sender *Client
}

type AppState struct {
	banned []string
}

func (c *Client) Read() (*Message, error) {
	buff := make([]byte, 1024)

	n, err := c.conn.Read(buff)
	if err != nil {
		return nil, err
	}

	return &Message {
		typ: UserCommandMessage,
		data: buff[:n],
		sender: c,
	}, nil
}

func (c *Client) Send(data []byte) (int, error) {
	return c.conn.Write(data)
}

func (c *Client) Close() error {
	return c.conn.Close()
}

const SpamPeriod = 1 
const MaxStrikes = 10

func handleConnection(conn net.Conn, state *AppState) {
	log.Printf("Client %s connected", conn.RemoteAddr().String()); 

	client := Client{
		conn: conn,
	}

	tcpAddr := conn.RemoteAddr().(*net.TCPAddr)
	if slices.Contains(state.banned, tcpAddr.IP.String()) {
		client.Send([]byte("You are banned\n"));
	} else {
		handleMessage(&Message{
			typ: ConnectedMessage,
			data: nil,
			sender: &client,
		})

		before := time.Now() 
		strikes := 0

		for {
			data, err := client.Read()
			if err != nil {
				log.Printf("Connected client failed to read message because of disconnection: %s\n", err);
				break
			}

			now := time.Now()
			if now.Sub(before).Seconds() < SpamPeriod {
				strikes++
				if strikes >= MaxStrikes {
					state.banned = append(state.banned, tcpAddr.IP.String())
					log.Printf("Client %s has been banned at %d\n", tcpAddr.String(), now.Second())
					break
				}
				before = now
				continue
			}

			err = handleMessage(data)
			if err != nil {
				log.Printf("Connected client failed to send message because of disconnection: %s\n", err)
				break
			}

			before = now
		}
	}

	handleMessage(&Message{
		typ: DisconnectedMessage,
		data: nil,
		sender: &client,
	})
}

func handleMessage(msg *Message) error {
	var (
		buff []byte = nil
	)

	switch (msg.typ) {
	case ConnectedMessage:
		buff = []byte("Welcome to the FTP server\n");
	case UserCommandMessage:
		buff = executeCommand(msg.data, func() {
			msg.sender.Close()	
		})
	case DisconnectedMessage:
		msg.sender.Close()
	default:
		return fmt.Errorf("ERROR: Uknown message type %d\n", msg.typ)
	}

	if buff != nil {
		_, err := msg.sender.Send(buff)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func executeCommand(byt []byte, handleExit func()) []byte {
	cmd := string(byt)	
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return []byte("ERROR: Invalid empty message\n");
	}

	switch fields[0] {
		case "ls":
			out, err := exec.Command("ls", fields[1:]...).Output()
			if err != nil {
				return []byte(err.Error())
			}
			return out 
		case "cd":
			if len(fields) < 2 {
				return []byte("ERROR: Expected destination directory\n")
			}
			err := os.Chdir(fields[1])
			if err != nil {
				return []byte(err.Error() + "\n")
			}
			return nil	
		case "get":
			if len(fields) < 2 {
				return []byte("ERROR: Expected desitnation directory\n")
			}
			data, err := os.ReadFile(fields[1]) 
			if err != nil {
				return []byte(err.Error() + "\n")
			}
			return data
		case "pwd":
			out, err := exec.Command("pwd").Output()
			if err != nil {
				return []byte(err.Error() + "\n")
			}
			return out
		case "exit":
			handleExit()
			return nil
		default:
			return []byte("ERROR: Unknown command\n");
	}	
}
