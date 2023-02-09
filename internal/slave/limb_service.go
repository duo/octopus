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

	clients     map[string]*LimbClient
	clientsLock sync.Mutex
}

// handle limb client connnection
func (ls *LimbService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	lc := NewLimbClient(vendor, ls.config, conn, ls.out)
	ls.clientsLock.Lock()
	ls.clients[vendor] = lc
	ls.clientsLock.Unlock()
	lc.run(func() {
		ls.clientsLock.Lock()
		delete(ls.clients, vendor)
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
		clients: make(map[string]*LimbClient),
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

		if !ok {
			err := fmt.Errorf("LimbClient(%s) not found", vendor)
			event.Callback(nil, err)
			continue
		}

		go ls.handleEvent(client, event)
	}
}

func (ls *LimbService) handleEvent(client *LimbClient, event *common.OctopusEvent) {
	if resp, err := client.sendEvent(event); err != nil {
		sendErr := fmt.Errorf("failed to send event to %s: %v", client.vendor, err)
		event.Callback(nil, sendErr)
	} else {
		event.ID = resp.ID
		event.Timestamp = resp.Timestamp
		event.Callback(event, nil)
	}
}
