package slave

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duo/octopus/internal/common"

	"github.com/gorilla/websocket"

	log "github.com/sirupsen/logrus"
)

type LimbClient struct {
	vendor string
	config *common.Configure

	conn *websocket.Conn
	out  chan<- *common.OctopusEvent

	writeLock sync.Mutex

	websocketRequests     map[int64]chan<- *common.OctopusResponse
	websocketRequestsLock sync.RWMutex
	websocketRequestID    int64
}

func NewLimbClient(vendor string, config *common.Configure, conn *websocket.Conn, out chan<- *common.OctopusEvent) *LimbClient {
	log.Infof("LimbClient(%s) websocket connected", vendor)

	return &LimbClient{
		vendor:            vendor,
		config:            config,
		conn:              conn,
		out:               out,
		websocketRequests: make(map[int64]chan<- *common.OctopusResponse),
	}
}

func (lc *LimbClient) Dispose() {
	oldConn := lc.conn
	if oldConn == nil {
		return
	}
	msg := websocket.FormatCloseMessage(
		websocket.CloseGoingAway,
		fmt.Sprintf(`{"type": %d, "data": {"type": %d, "data": "server_shutting_down"}}`, common.MsgRequest, common.ReqDisconnect),
	)
	_ = oldConn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(3*time.Second))
	_ = oldConn.Close()
}

// read message from limb client
func (lc *LimbClient) run(stopFunc func()) {
	defer func() {
		log.Infof("LimbClient(%s) disconnected from websocket", lc.vendor)
		_ = lc.conn.Close()
		stopFunc()
	}()

	for {
		var msg common.OctopusMessage
		err := lc.conn.ReadJSON(&msg)
		if err != nil {
			log.Warnf("Error reading from websocket: %v", err)
			break
		}

		switch msg.Type {
		case common.MsgRequest:
			request := msg.Data.(*common.OctopusRequest)
			if request.Type == common.ReqPing {
				log.Debugln("Receive ping request")
			} else if request.Type == common.ReqEvent {
				lc.out <- request.Data.(*common.OctopusEvent)
			} else {
				log.Warnf("Request %s not support", request.Type)
			}
		case common.MsgResponse:
			lc.websocketRequestsLock.RLock()
			respChan, ok := lc.websocketRequests[msg.ID]
			lc.websocketRequestsLock.RUnlock()
			if ok {
				select {
				case respChan <- msg.Data.(*common.OctopusResponse):
				default:
					log.Warnf("Failed to handle response to %d: channel didn't accept response", msg.ID)
				}
			} else {
				log.Warnf("Dropping response to %d: unknown request ID", msg.ID)
			}
		}
	}
}

// send event to limb client, and return response
func (lc *LimbClient) sendEvent(event *common.OctopusEvent) (*common.OctopusEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lc.config.Service.SendTiemout)
	defer cancel()

	if data, err := lc.request(ctx, &common.OctopusRequest{
		Type: common.ReqEvent,
		Data: event,
	}); err != nil {
		return nil, err
	} else {
		return data.(*common.OctopusEvent), nil
	}
}

func (lc *LimbClient) request(ctx context.Context, req *common.OctopusRequest) (any, error) {
	msg := &common.OctopusMessage{
		ID:   atomic.AddInt64(&lc.websocketRequestID, 1),
		Type: common.MsgRequest,
		Data: req,
	}
	respChan := make(chan *common.OctopusResponse, 1)

	lc.addWebsocketResponseWaiter(msg.ID, respChan)
	defer lc.removeWebsocketResponseWaiter(msg.ID, respChan)

	log.Debugf("Send request message #%d %s", msg.ID, req.Type)
	if err := lc.sendMessage(msg); err != nil {
		return nil, err
	}

	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, resp.Error
		} else {
			switch resp.Type {
			case common.RespClosed:
				return nil, ErrWebsocketClosed
			case common.RespEvent:
				log.Debugf("Receive response for #%d %s", msg.ID, req.Type)
				return resp.Data, nil
			default:
				return nil, fmt.Errorf("response %s not support", resp.Type)
			}
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (lc *LimbClient) sendMessage(msg *common.OctopusMessage) error {
	conn := lc.conn
	if msg == nil {
		return nil
	} else if conn == nil {
		return ErrWebsocketNotConnected
	}
	lc.writeLock.Lock()
	defer lc.writeLock.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(lc.config.Service.SendTiemout))
	return conn.WriteJSON(msg)
}

func (lc *LimbClient) addWebsocketResponseWaiter(reqID int64, waiter chan<- *common.OctopusResponse) {
	lc.websocketRequestsLock.Lock()
	lc.websocketRequests[reqID] = waiter
	lc.websocketRequestsLock.Unlock()
}

func (lc *LimbClient) removeWebsocketResponseWaiter(reqID int64, waiter chan<- *common.OctopusResponse) {
	lc.websocketRequestsLock.Lock()
	existingWaiter, ok := lc.websocketRequests[reqID]
	if ok && existingWaiter == waiter {
		delete(lc.websocketRequests, reqID)
	}
	lc.websocketRequestsLock.Unlock()
	close(waiter)
}
