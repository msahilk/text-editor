package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"text-editor/commons"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Clients manages connected client information and operations.
type Clients struct {
	// Stores active client data.
	list map[uuid.UUID]*client

	// Guards against concurrent map access.
	mu sync.RWMutex

	// Channel for client removal requests.
	deleteRequests chan deleteRequest

	// Channel for client retrieval requests.
	readRequests chan readRequest

	// Channel for adding new clients.
	addRequests chan *client

	// Channel for updating client usernames.
	nameUpdateRequests chan nameUpdate
}

// NewClients initializes and returns a Clients instance.
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

// client represents a connected user's session.
type client struct {
	Conn   *websocket.Conn
	SiteID string
	id     uuid.UUID

	// Protects against concurrent WebSocket writes.
	writeMu sync.Mutex

	// Guards client data modifications.
	mu sync.Mutex

	Username string
}

var (
	// Unique identifier for each client, increments monotonically.
	siteID = 0

	// Protects siteID increments.
	mu sync.Mutex

	// Converts HTTP connections to WebSocket.
	upgrader = websocket.Upgrader{}

	// Buffers client messages.
	messageChan = make(chan commons.Message)

	// Buffers document synchronization messages.
	syncChan = make(chan commons.Message)

	// Manages all connected clients.
	clients = NewClients()
)

func main() {
	addr := flag.String("addr", ":8080", "Server's network address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleConn)

	// Manages client state.
	go clients.handle()

	// Processes incoming messages.
	go handleMsg()

	// Manages document synchronization.
	go handleSync()

	// Initializes the server.
	log.Printf("Starting server on %s", *addr)

	server := &http.Server{
		Addr:         *addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      mux,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Fatal("Server startup failed, terminating.", err)
	}
}

// handleConn manages new WebSocket connections and message reading.
func handleConn(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		color.Red("WebSocket upgrade failed: %v\n", err)
		conn.Close()
		return
	}
	defer conn.Close()

	clientID := uuid.New()

	// Safely increment and assign siteID.
	mu.Lock()
	siteID++

	client := &client{
		Conn:    conn,
		SiteID:  strconv.Itoa(siteID),
		id:      clientID,
		writeMu: sync.Mutex{},
		mu:      sync.Mutex{},
	}
	mu.Unlock()

	clients.add(client)

	siteIDMsg := commons.Message{Type: commons.SiteIDMessage, Text: client.SiteID, ID: clientID}
	clients.broadcastOne(siteIDMsg, clientID)

	docReq := commons.Message{Type: commons.DocReqMessage, ID: clientID}
	clients.broadcastOneExcept(docReq, clientID)

	clients.sendUsernames()

	// Continuously read and process messages from the client.
	for {
		var msg commons.Message
		if err := client.read(&msg); err != nil {
			color.Red("Message read failed. Closing %s's connection. Error: %s", client.Username, err)
			return
		}

		// Route document sync messages separately.
		if msg.Type == commons.DocSyncMessage {
			syncChan <- msg
			continue
		}

		// Set message origin.
		msg.ID = clientID

		// Queue message for processing.
		messageChan <- msg
	}
}

// handleMsg processes and broadcasts messages from clients.
func handleMsg() {
	for {
		// Retrieve next message.
		msg := <-messageChan

		// Log message details.
		t := time.Now().Format(time.ANSIC)
		if msg.Type == commons.JoinMessage {
			clients.updateName(msg.ID, msg.Username)
			color.Green("%s >> %s %s (ID: %s)\n", t, msg.Username, msg.Text, msg.ID)
			clients.sendUsernames()
		} else if msg.Type == "operation" {
			color.Green("operation >> %+v from ID=%s\n", msg.Operation, msg.ID)
		} else {
			color.Green("%s >> unrecognized message type:  %v\n", t, msg)
			clients.sendUsernames()
			continue
		}

		clients.broadcastAllExcept(msg, msg.ID)
	}
}

// handleSync manages document synchronization messages.
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

// handle ensures thread-safe access to the Clients struct.
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
			c.list[n.id].mu.Lock()
			c.list[n.id].Username = n.newName
			c.list[n.id].mu.Unlock()
		}
	}
}

// deleteRequest facilitates client removal from the client list.
type deleteRequest struct {
	// Client to be removed.
	id uuid.UUID

	// Signals completion of deletion.
	done chan int
}

// readRequest facilitates client information retrieval.
type readRequest struct {
	// Indicates if all clients should be retrieved.
	readAll bool

	// Specific client ID to retrieve (if not readAll).
	id uuid.UUID

	// Channel for sending retrieved client(s).
	resp chan *client
}

// getAll retrieves all active clients.
func (c *Clients) getAll() chan *client {
	c.mu.RLock()
	resp := make(chan *client, len(c.list))
	c.mu.RUnlock()
	c.readRequests <- readRequest{readAll: true, resp: resp}
	return resp
}

// get retrieves a specific client by ID.
func (c *Clients) get(id uuid.UUID) chan *client {
	resp := make(chan *client)

	c.readRequests <- readRequest{readAll: false, id: id, resp: resp}
	return resp
}

// add introduces a new client to the list.
func (c *Clients) add(client *client) {
	c.addRequests <- client
}

// nameUpdate facilitates client username changes.
type nameUpdate struct {
	id      uuid.UUID
	newName string
}

// updateName modifies a client's username.
func (c *Clients) updateName(id uuid.UUID, newName string) {
	c.nameUpdateRequests <- nameUpdate{id, newName}
}

// delete removes a client from the active list.
func (c *Clients) delete(id uuid.UUID) {
	req := deleteRequest{id, make(chan int)}
	c.deleteRequests <- req
	<-req.done
	c.sendUsernames()
}

// broadcastAll sends a message to every active client.
func (c *Clients) broadcastAll(msg commons.Message) {
	color.Blue("Broadcasting to all users. Text: %s", msg.Text)
	for client := range c.getAll() {
		if err := client.send(msg); err != nil {
			color.Red("ERROR: %s", err)
			c.delete(client.id)
		}
	}
}

// broadcastAllExcept sends a message to all clients except one.
func (c *Clients) broadcastAllExcept(msg commons.Message, except uuid.UUID) {
	for client := range c.getAll() {
		if client.id == except {
			continue
		}
		if err := client.send(msg); err != nil {
			color.Red("ERROR: %s", err)
			c.delete(client.id)
		}
	}
}

// broadcastOne sends a message to a specific client.
func (c *Clients) broadcastOne(msg commons.Message, dst uuid.UUID) {
	client := <-c.get(dst)
	if err := client.send(msg); err != nil {
		color.Red("ERROR: %s", err)
		c.delete(client.id)
	}
}

// broadcastOneExcept sends a message to any client except one.
func (c *Clients) broadcastOneExcept(msg commons.Message, except uuid.UUID) {
	for client := range c.getAll() {
		if client.id == except {
			continue
		}
		if err := client.send(msg); err != nil {
			color.Red("ERROR: %s", err)
			c.delete(client.id)
			continue
		}
		break
	}
}

// close terminates a client's connection and removes them from the list.
func (c *Clients) close(id uuid.UUID) {
	c.mu.RLock()
	client, ok := c.list[id]
	if ok {
		if err := client.Conn.Close(); err != nil {
			color.Red("Connection closure failed: %s\n", err)
		}
	} else {
		color.Red("Connection closure failed: client not found")
		return
	}
	color.Red("Removing %v from client list.\n", c.list[id].Username)
	c.mu.RUnlock()

	c.mu.Lock()
	delete(c.list, id)
	c.mu.Unlock()

}

// read retrieves a message from the client's connection.
func (c *client) read(msg *commons.Message) error {
	err := c.Conn.ReadJSON(msg)

	c.mu.Lock()
	name := c.Username
	c.mu.Unlock()

	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			color.Red("Message read from %s failed: %v", name, err)
		}
		color.Red("Client %v disconnected", name)
		clients.delete(c.id)
		return err
	}
	return nil
}

// send transmits a message over the client's connection.
func (c *client) send(v interface{}) error {
	c.writeMu.Lock()
	err := c.Conn.WriteJSON(v)
	c.writeMu.Unlock()
	return err
}

// sendUsernames broadcasts the list of active users to all clients.
func (c *Clients) sendUsernames() {
	var users string
	for client := range c.getAll() {
		users += client.Username + ","
	}

	syncChan <- commons.Message{Text: users, Type: commons.UsersMessage}
}
