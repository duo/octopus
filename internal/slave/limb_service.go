package slave

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/duo/octopus/internal/common"

	"github.com/gorilla/websocket"

	log "github.com/sirupsen/logrus"
)

var (
	errMissingToken = common.ErrorResponse{
		HTTPStatus: http.StatusForbidden,
		Code:       "M_MISSING_TOKEN",
		Message:    "Missing authorization header",
	}
	errUnknownToken = common.ErrorResponse{
		HTTPStatus: http.StatusForbidden,
		Code:       "M_UNKNOWN_TOKEN",
		Message:    "Unknown authorization token",
	}
	errMissingVendor = common.ErrorResponse{
		HTTPStatus: http.StatusForbidden,
		Code:       "M_MISSING_VENDOR",
		Message:    "Missing vendor header",
	}

	ErrWebsocketNotConnected = errors.New("websocket not connected")
	ErrWebsocketClosed       = errors.New("websocket closed before response received")

	upgrader = websocket.Upgrader{}
)

type LimbService struct {
	config *common.Configure

	in  <-chan *common.OctopusEvent
	out chan<- *common.OctopusEvent

	server *http.Server

	clients     map[string]Client
	clientsLock sync.Mutex

	mutex common.KeyMutex
}

// handle client connnection
func (ls *LimbService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/onebot/") {
		ls.handleOnebotConnection(w, r)
		return
	}

	ls.handleLimbConnection(w, r)
}

// handle limb client connnection
func (ls *LimbService) handleLimbConnection(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Basic ") {
		errMissingToken.Write(w)
		return
	}

	if authHeader[len("Basic "):] != ls.config.Service.Secret {
		errUnknownToken.Write(w)
		return
	}

	vendor := r.Header.Get("Vendor")
	if vendor == "" {
		errMissingVendor.Write(w)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warnf("Failed to upgrade websocket request: %v", err)
		return
	}

	ls.observe(fmt.Sprintf("LimbClient(%s) connected", vendor))

	lc := NewLimbClient(vendor, ls.config, conn, ls.out)
	ls.clientsLock.Lock()
	ls.clients[vendor] = lc
	ls.clientsLock.Unlock()
	lc.run(func() {
		ls.observe(fmt.Sprintf("LimbClient(%s) disconnected", vendor))
		ls.clientsLock.Lock()
		delete(ls.clients, vendor)
		ls.clientsLock.Unlock()
	})
}

// handle onebot client connnection
func (ls *LimbService) handleOnebotConnection(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		errMissingToken.Write(w)
		return
	}

	if authHeader[len("Bearer "):] != ls.config.Service.Secret {
		errUnknownToken.Write(w)
		return
	}

	selfID := r.Header.Get("X-Self-Id")
	if selfID == "" {
		errMissingVendor.Write(w)
		return
	}
	vendor := common.Vendor{
		Type: r.URL.Path[8:],
		UID:  selfID,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warnf("Failed to upgrade websocket request: %v", err)
		return
	}

	ls.observe(fmt.Sprintf("OnebotClient(%s) connected", vendor))

	oc := NewOnebotClient(&vendor, ls.config, conn, ls.out)
	ls.clientsLock.Lock()
	ls.clients[vendor.String()] = oc
	ls.clientsLock.Unlock()
	oc.run(func() {
		ls.observe(fmt.Sprintf("OnebotClient(%s) disconnected", vendor))
		ls.clientsLock.Lock()
		delete(ls.clients, vendor.String())
		ls.clientsLock.Unlock()
	})
}

func (ls *LimbService) Start() {
	log.Infoln("LimbService starting to listen on", ls.config.Service.Addr)
	go func() {
		err := ls.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalln("Error in listener:", err)
		}
	}()

	go ls.handleMasterLoop()
}

func (ls *LimbService) Stop() {
	log.Infoln("LimbService stopping")
	ls.clientsLock.Lock()
	for _, client := range ls.clients {
		client.Dispose()
	}
	ls.clientsLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := ls.server.Shutdown(ctx)
	if err != nil {
		log.Warnf("Failed to close server: %v", err)
	}
}

func NewLimbService(config *common.Configure, in <-chan *common.OctopusEvent, out chan<- *common.OctopusEvent) *LimbService {
	service := &LimbService{
		config:  config,
		in:      in,
		out:     out,
		clients: make(map[string]Client),
		mutex:   common.NewHashed(47),
	}
	service.server = &http.Server{
		Addr:    service.config.Service.Addr,
		Handler: service,
	}

	return service
}

// read events from master
func (ls *LimbService) handleMasterLoop() {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("Panic in handle master loop: %v\n%s", panicErr, debug.Stack())
		}
	}()

	for event := range ls.in {
		vendor := event.Vendor.String()
		ls.clientsLock.Lock()
		client, ok := ls.clients[vendor]
		ls.clientsLock.Unlock()

		if ok {
			event := event
			go func() {
				ls.mutex.LockKey(event.Chat.ID)
				defer ls.mutex.UnlockKey(event.Chat.ID)

				ls.handleEvent(client, event)
			}()
		} else {
			go event.Callback(nil, fmt.Errorf("LimbClient(%s) not found", vendor))
		}
	}
}

func (ls *LimbService) handleEvent(client Client, event *common.OctopusEvent) {
	if resp, err := client.SendEvent(event); err != nil {
		sendErr := fmt.Errorf("failed to send event to %s: %v", client.Vendor(), err)
		event.Callback(nil, sendErr)
	} else {
		event.ID = resp.ID
		event.Timestamp = resp.Timestamp
		event.Callback(event, nil)
	}
}

func (ls *LimbService) observe(msg string) {
	go func() {
		ls.out <- &common.OctopusEvent{
			Type:    common.EventObserve,
			Content: msg,
		}
	}()
}
