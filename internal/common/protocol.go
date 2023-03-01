package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	VENDOR_SEP    = ";"
	REMOTE_PREFIX = "remote:"
)

type OctopusMessage struct {
	ID   int64       `json:"id,omitempty"`
	Type MessageType `json:"type,omitempty"`
	Data any         `json:"data,omitempty"`
}

type OctopusRequest struct {
	Type RequestType `json:"type,omitempty"`
	Data any         `json:"data,omitempty"`
}

type OctopusResponse struct {
	Type  ResponseType   `json:"type,omitempty"`
	Error *ErrorResponse `json:"error,omitempty"`
	Data  any            `json:"data,omitempty"`
}

type ErrorResponse struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

type OctopusEvent struct {
	Vendor    Vendor     `json:"vendor,omitempty"`
	ID        string     `json:"id,omitempty"`
	ThreadID  string     `json:"thread_id,omitempty"`
	Timestamp int64      `json:"timestamp,omitempty"`
	From      User       `json:"from,omitempty"`
	Chat      Chat       `json:"chat,omitempty"`
	Type      EventType  `json:"type,omitempty"`
	Content   string     `json:"content,omitempty"`
	Reply     *ReplyInfo `json:"reply,omitempty"`
	Data      any        `json:"data,omitempty"`

	Callback func(*OctopusEvent, error) `json:"-"`
}

type Vendor struct {
	Type string `json:"type,omitempty"`
	UID  string `json:"uid,omitempty"`
}

type User struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	Remark   string `json:"remark,omitempty"`
}

type Chat struct {
	ID    string `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

type ReplyInfo struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"ts"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
}

type AppData struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"desc,omitempty"`
	Source      string `json:"source,omitempty"`
	URL         string `json:"url,omitempty"`

	Content string               `json:"raw,omitempty"`
	Blobs   map[string]*BlobData `json:"blobs,omitempty"`
}

type LocationData struct {
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
}

type BlobData struct {
	Name   string `json:"name,omitempty"`
	Mime   string `json:"mime,omitempty"`
	Binary []byte `json:"binary,omitempty"`
}

func (o *OctopusMessage) UnmarshalJSON(data []byte) error {
	type cloneType OctopusMessage

	rawMsg := json.RawMessage{}
	o.Data = &rawMsg

	if err := json.Unmarshal(data, (*cloneType)(o)); err != nil {
		return err
	}

	switch o.Type {
	case MsgRequest:
		var request *OctopusRequest
		if err := json.Unmarshal(rawMsg, &request); err != nil {
			return err
		}
		o.Data = request
	case MsgResponse:
		var response *OctopusResponse
		if err := json.Unmarshal(rawMsg, &response); err != nil {
			return err
		}
		o.Data = response
	}

	return nil
}

func (o *OctopusRequest) UnmarshalJSON(data []byte) error {
	type cloneType OctopusRequest

	rawMsg := json.RawMessage{}
	o.Data = &rawMsg

	if err := json.Unmarshal(data, (*cloneType)(o)); err != nil {
		return err
	}

	switch o.Type {
	case ReqEvent:
		var event *OctopusEvent
		if err := json.Unmarshal(rawMsg, &event); err != nil {
			return err
		}
		o.Data = event
	}

	return nil
}

func (o *OctopusResponse) UnmarshalJSON(data []byte) error {
	type cloneType OctopusResponse

	rawMsg := json.RawMessage{}
	o.Data = &rawMsg

	if err := json.Unmarshal(data, (*cloneType)(o)); err != nil {
		return err
	}

	if o.Error != nil {
		return nil
	}

	switch o.Type {
	case RespEvent:
		var event *OctopusEvent
		if err := json.Unmarshal(rawMsg, &event); err != nil {
			return err
		}
		o.Data = event
	default:
		var data string
		if err := json.Unmarshal(rawMsg, &data); err != nil {
			return err
		}
		o.Data = data
	}

	return nil
}

func (o *OctopusEvent) UnmarshalJSON(data []byte) error {
	type cloneType OctopusEvent

	rawMsg := json.RawMessage{}
	o.Data = &rawMsg

	if err := json.Unmarshal(data, (*cloneType)(o)); err != nil {
		return err
	}

	switch o.Type {
	case EventPhoto:
		var photos []*BlobData
		if err := json.Unmarshal(rawMsg, &photos); err != nil {
			return err
		}
		o.Data = photos
	case EventSticker, EventAudio, EventVideo, EventFile:
		var blob *BlobData
		if err := json.Unmarshal(rawMsg, &blob); err != nil {
			return err
		}
		o.Data = blob
	case EventLocation:
		var location *LocationData
		if err := json.Unmarshal(rawMsg, &location); err != nil {
			return err
		}
		o.Data = location
	case EventApp:
		var app *AppData
		if err := json.Unmarshal(rawMsg, &app); err != nil {
			return err
		}
		o.Data = app
	case EventSync:
		var chats []*Chat
		if err := json.Unmarshal(rawMsg, &chats); err != nil {
			return err
		}
		o.Data = chats
	}

	return nil
}

const (
	MsgRequest MessageType = iota
	MsgResponse
)

const (
	ReqDisconnect RequestType = iota
	ReqPing
	ReqEvent
)

const (
	RespClosed ResponseType = iota
	RespPing
	RespEvent
)

const (
	EventText EventType = iota
	EventPhoto
	EventAudio
	EventVideo
	EventFile
	EventLocation
	EventNotice
	EventApp
	EventRevoke
	EventVoIP
	EventSystem
	EventSync
	EventObserve
	EventSticker
)

type MessageType int

func (t MessageType) String() string {
	switch t {
	case MsgRequest:
		return "request"
	case MsgResponse:
		return "response"
	default:
		return "unknown"
	}
}

type RequestType int

func (t RequestType) String() string {
	switch t {
	case ReqDisconnect:
		return "disconnect"
	case ReqPing:
		return "ping"
	case ReqEvent:
		return "event"
	default:
		return "unknown"
	}
}

type ResponseType int

func (t ResponseType) String() string {
	switch t {
	case RespClosed:
		return "closed"
	case RespPing:
		return "ping"
	case RespEvent:
		return "event"
	default:
		return "unknown"
	}
}

type EventType int

func (t EventType) String() string {
	switch t {
	case EventText:
		return "text"
	case EventPhoto:
		return "photo"
	case EventAudio:
		return "audio"
	case EventVideo:
		return "video"
	case EventFile:
		return "file"
	case EventLocation:
		return "location"
	case EventNotice:
		return "notice"
	case EventApp:
		return "app"
	case EventRevoke:
		return "revoke"
	case EventVoIP:
		return "voip"
	case EventSystem:
		return "system"
	case EventSync:
		return "sync"
	case EventObserve:
		return "observe"
	case EventSticker:
		return "sticker"
	default:
		return "unknown"
	}
}

func (v Vendor) String() string {
	return fmt.Sprintf("%s%s%s", v.Type, VENDOR_SEP, v.UID)
}

func VendorFromString(str string) (*Vendor, error) {
	parts := strings.Split(str, VENDOR_SEP)
	if len(parts) != 2 {
		return nil, errors.New("vendor format invalid")
	}

	return &Vendor{parts[0], parts[1]}, nil
}

func (er *ErrorResponse) Error() string {
	return fmt.Sprintf("%s: %s", er.Code, er.Message)
}

func (er ErrorResponse) Write(w http.ResponseWriter) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(er.HTTPStatus)
	_ = Respond(w, &er)
}

func Respond(w http.ResponseWriter, data any) error {
	w.Header().Add("Content-Type", "application/json")
	dataStr, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.Write(dataStr)
	return err
}
