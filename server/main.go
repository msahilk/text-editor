package main

import (
	"flag"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
	"sync"
	"text-editor/commons"
	"time"
)

type Clients struct {
	list               map[uuid.UUID]*client
	mu                 sync.RWMutex
	deleteRequests     chan deleteRequest
	readRequests       chan readRequest
	addRequests        chan *client
	nameUpdateRequests chan nameUpdate
}

type client struct {
	Conn   *websocket.Conn
	SiteID string
	id     uuid.UUID

	writeMutex sync.Mutex
	mutex      sync.Mutex

	Username string
}

type deleteRequest struct {
	id uuid.UUID

	done chan int
}

type readRequest struct {
	readAll bool

	id uuid.UUID

	resp chan *client
}

type nameUpdate struct {
	newName string
	id      uuid.UUID
}

func NewClients() *Clients {
	return &Clients{
		list:               make(map[uuid.UUID]*client),
		mu:                 sync.RWMutex{},
		deleteRequests:     make(chan deleteRequest),
		readRequests:       make(chan readRequest, 10000),
		addRequests:        make(chan *client),
		nameUpdateRequests: make(chan nameUpdate),
	}
}

var (
	siteID = 0

	mu sync.RWMutex

	upgrade = websocket.Upgrader{}

	messageChan = make(chan commons.Message)

	syncChan = make(chan commons.Message)

	clients = NewClients()
)

func main() {
	address := flag.String("Server", ":8080", "Server's network address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleConnection)

	go clients.handle()

	go handleMsg()

	go handleSync()

	log.Printf("Starting server on %s", *address)

	server := &http.Server{
		Addr:         *address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}

}

func handleConnection(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		color.Red("Error upgrading connection: %s", err)
		conn.Close()
		return

	}

	defer conn.Close()

	clientID := uuid.New()

	mu.Lock()
	siteID++

	client := &client{
		Conn:       conn,
		SiteID:     strconv.Itoa(siteID),
		id:         clientID,
		writeMutex: sync.Mutex{},
		mutex:      sync.Mutex{},
	}
	mu.Unlock()

	clients.add(client)

	siteIDMsg := commons.Message{Type: commons.SiteIDMessage, Text: client.SiteID, ID: clientID}
	clients.broadcastOne(siteIDMsg, clientID)

	docReq := commons.Message{Type: commons.DocReqMessage, ID: clientID}
	clients.broadcastAnyOther(docReq, clientID)

	clients.sendUsernames()

	for {
		var msg commons.Message

		if err := client.read(&msg); err != nil {
			color.Red("Failed to read message. Closing client connection with %s, error: %s", client.Username, err)
			return
		}

		if msg.Type == commons.DocSyncMessage {
			syncChan <- msg
			continue
		}

		msg.ID = clientID

		messageChan <- msg

	}
}

func handleMsg() {
	for {
		msg := <-messageChan

		t := time.Now().Format(time.ANSIC)

		if msg.Type == commons.JoinMessage {
			clients.updateName(msg.ID, msg.Username)
			color.Green("%s >> %s %s (ID: %s)\n", t, msg.Username, msg.Text, msg.ID)
		} else if msg.Type == "operation" {
			color.Green("operation >> %+v from ID=%s\n", msg.Operation, msg.ID)
		} else {
			color.Green("%s >> unknown message type: %v\n", t, msg)
			clients.sendUsernames()
			continue
		}
		clients.broadcastAllExcept(msg, msg.ID)
	}

}

func handleSync() {
	for {
		syncMsg := <-syncChan

		switch syncMsg.Type {
		case commons.DocSyncMessage:
			clients.broadcastOne(syncMsg, syncMsg.ID)
		case commons.UsersMessage:
			color.Blue("usernames: %s", syncMsg.Text)
			clients.broadcastAll(syncMsg)
		}
	}
}

func (c *Clients) handle() {
	for {
		select {
		case req := <-c.deleteRequests:
			c.close(req.id)
			req.done <- 1
			close(req.done)
		case req := <-c.readRequests:
			if req.readAll {
				for _, client := range c.list {
					req.resp <- client
				}
				close(req.resp)
			} else {
				req.resp <- c.list[req.id]
				close(req.resp)
			}
		case client := <-c.addRequests:
			c.mu.Lock()
			c.list[client.id] = client
			c.mu.Unlock()
		case n := <-c.nameUpdateRequests:
			c.list[n.id].mutex.Lock()
			c.list[n.id].Username = n.newName
			c.list[n.id].mutex.Unlock()
		}

	}
}

func (c *Clients) getAll() chan *client {
	c.mu.RLock()
	resp := make(chan *client, len(c.list))
	c.mu.RUnlock()
	c.readRequests <- readRequest{readAll: true, resp: resp}
	return resp
}

func (c *Clients) get(id uuid.UUID) chan *client {
	resp := make(chan *client)

	c.readRequests <- readRequest{readAll: false, id: id, resp: resp}
	return resp
}

func (c *Clients) add(client *client) {
	c.addRequests <- client
}

func (c *Clients) updateName(id uuid.UUID, newName string) {
	c.nameUpdateRequests <- nameUpdate{newName, id}
}

func (c *Clients) delete(id uuid.UUID) {
	req := deleteRequest{id: id, done: make(chan int)}
	c.deleteRequests <- req
	<-req.done
	c.sendUsernames()
}

func (c *Clients) broadcastAll(msg commons.Message) {
	color.Blue("sending message to all users. Text: %s", msg.Text)
	for client := range c.getAll() {
		if err := client.send(msg); err != nil {
			color.Red("Error sending message to all users: %s", err)
			c.delete(client.id)
		}
	}
}

func (c *Clients) broadcastOne(msg commons.Message, id uuid.UUID) {
	client := <-c.get(id)

	if err := client.send(msg); err != nil {
		color.Red("Error sending to client: %s, %s", client.Username, err)
		c.delete(client.id)
	}
}

func (c *Clients) broadcastAllExcept(msg commons.Message, id uuid.UUID) {
	for client := range c.getAll() {
		if client.id != id {
			if err := client.send(msg); err != nil {
				color.Red("Error sending message to client %s, %s", client.Username, err)
				c.delete(client.id)
			}
		}
	}
}

func (c *Clients) broadcastAnyOther(msg commons.Message, id uuid.UUID) {
	for client := range c.getAll() {
		if client.id != id {
			if err := client.send(msg); err != nil {
				color.Red("Error sending message to client %s, %s", client.Username, err)
				c.delete(client.id)
				continue
			}
			return
		}
	}
}

func (c *Clients) close(id uuid.UUID) {
	c.mu.RLock()
	client, ok := c.list[id]
	if ok {
		if err := client.Conn.Close(); err != nil {
			color.Red("Error closing connection: %s", err)
		}
	} else {
		color.Red("Client %s not found", id)
	}

	color.Blue("Closing connection with %s", client.Username)
	c.mu.RUnlock()

	c.mu.Lock()
	delete(c.list, id)
	c.mu.Unlock()

}

func (c *client) send(msg interface{}) error {
	c.writeMutex.Lock()
	err := c.Conn.WriteJSON(msg)
	c.writeMutex.Unlock()
	return err

}

func (c *client) read(msg *commons.Message) error {
	err := c.Conn.ReadJSON(msg)

	c.mutex.Lock()
	name := c.Username
	c.mutex.Unlock()

	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			color.Red("Failed to read message from client %s: %s", c.Username, err)
		}
		color.Red("client %v disconnected", name)
		return err

	}
	return nil
}

func (c *Clients) sendUsernames() {
	var users string

	for client := range c.getAll() {
		users += client.Username
		users += ","
	}
	syncChan <- commons.Message{Text: users, Type: commons.UsersMessage}
}
