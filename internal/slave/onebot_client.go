package slave

import (
	"cmp"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duo/octopus/internal/common"
	"github.com/duo/octopus/internal/filter"
	"github.com/duo/octopus/internal/onebot"
	"github.com/tidwall/gjson"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"

	log "github.com/sirupsen/logrus"
)

const (
	LAGRANGE_ONEBOT string = "Lagrange.OneBot"
)

type OnebotClient struct {
	vendor *common.Vendor
	agent  string
	config *common.Configure

	self    *onebot.FriendInfo
	friends map[int64]*onebot.FriendInfo
	groups  map[int64]*onebot.GroupInfo

	conn *websocket.Conn
	out  chan<- *common.OctopusEvent

	s2m filter.EventFilterChain
	m2s filter.EventFilterChain

	writeLock sync.Mutex

	websocketRequests     map[string]chan<- *onebot.Response
	websocketRequestsLock sync.RWMutex
	websocketRequestID    int64

	mutex common.KeyMutex
}

func NewOnebotClient(vendor *common.Vendor, agent string, config *common.Configure, conn *websocket.Conn, out chan<- *common.OctopusEvent) *OnebotClient {
	log.Infof("OnebotClient(%s) websocket connected", vendor)

	m2s := filter.NewEventFilterChain(
		filter.StickerM2SFilter{},
	)
	s2m := filter.NewEventFilterChain(
		filter.VoiceS2MFilter{},
		filter.EmoticonS2MFilter{},
		filter.StickerS2MFilter{},
	)

	return &OnebotClient{
		vendor:            vendor,
		agent:             agent,
		config:            config,
		friends:           make(map[int64]*onebot.FriendInfo),
		groups:            make(map[int64]*onebot.GroupInfo),
		conn:              conn,
		out:               out,
		m2s:               m2s,
		s2m:               s2m,
		websocketRequests: make(map[string]chan<- *onebot.Response),
		mutex:             common.NewHashed(47),
	}
}

func (oc *OnebotClient) Vendor() string {
	return oc.vendor.String()
}

// read message from ontbot client
func (oc *OnebotClient) run(stopFunc func()) {
	defer func() {
		log.Infof("OnebotClient(%s) disconnected from websocket", oc.vendor)
		_ = oc.conn.Close()
		stopFunc()
	}()

	for {
		var m map[string]interface{}
		if err := oc.conn.ReadJSON(&m); err != nil {
			log.Warnf("Error reading from websocket: %v", err)
			break
		}
		payload, err := onebot.UnmarshalPayload(m)
		if err != nil {
			log.Warnf("Failed to unmarshal payload: %v", err)
			continue
		}

		switch payload.PayloadType() {
		case onebot.PaylaodRequest:
			log.Warnf("Request %s not support", payload.(*onebot.Request).Action)
		case onebot.PayloadResponse:
			go oc.processResponse(payload.(*onebot.Response))
		case onebot.PayloadEvent:
			go oc.processEvent(payload.(onebot.IEvent))
		}
	}
}

// send event to onebot client, and return response
func (oc *OnebotClient) SendEvent(event *common.OctopusEvent) (*common.OctopusEvent, error) {
	log.Debugf("Receive Octopus event: %+v", event)

	event = oc.m2s.Apply(event)

	targetID, err := common.Atoi(event.Chat.ID)
	if err != nil {
		return nil, err
	}

	segments := []onebot.ISegment{}

	if event.Reply != nil {
		segments = append(segments, onebot.NewReply(event.Reply.ID))
	}

	switch event.Type {
	case common.EventText:
		segments = append(segments, onebot.NewText(event.Content))
	case common.EventPhoto:
		photos := event.Data.([]*common.BlobData)
		for _, photo := range photos {
			binary := fmt.Sprintf("base64://%s", base64.StdEncoding.EncodeToString(photo.Binary))
			segments = append(segments, onebot.NewImage(binary))
		}
	case common.EventSticker:
		blob := event.Data.(*common.BlobData)
		binary := fmt.Sprintf("base64://%s", base64.StdEncoding.EncodeToString(blob.Binary))
		segments = append(segments, onebot.NewImage(binary))
	case common.EventVideo:
		blob := event.Data.(*common.BlobData)
		binary := fmt.Sprintf("base64://%s", base64.StdEncoding.EncodeToString(blob.Binary))
		segments = append(segments, onebot.NewVideo(binary))
	case common.EventAudio:
		blob := event.Data.(*common.BlobData)
		binary := fmt.Sprintf("base64://%s", base64.StdEncoding.EncodeToString(blob.Binary))
		segments = append(segments, onebot.NewRecord(binary))
	case common.EventFile:
		// TODO:
		/*
			if oc.agent == LAGRANGE_ONEBOT {
			}
		*/
		blob := event.Data.(*common.BlobData)
		binary := fmt.Sprintf("base64://%s", base64.StdEncoding.EncodeToString(blob.Binary))
		segments = append(segments, onebot.NewFile(binary, blob.Name))
	case common.EventLocation:
		location := event.Data.(*common.LocationData)
		locationJson := fmt.Sprintf(`
		{
			"app": "com.tencent.map",
			"desc": "地图",
			"view": "LocationShare",
			"ver": "0.0.0.1",
			"prompt": "[位置]%s",
			"from": 1,
			"meta": {
			  "Location.Search": {
				"id": "12250896297164027526",
				"name": "%s",
				"address": "%s",
				"lat": "%.5f",
				"lng": "%.5f",
				"from": "plusPanel"
			  }
			},
			"config": {
			  "forward": 1,
			  "autosize": 1,
			  "type": "card"
			}
		}
		`, location.Name, location.Name, location.Address, location.Latitude, location.Longitude)
		segments = append(segments, onebot.NewJSON(locationJson))
	case common.EventRevoke:
		// TODO:
	default:
		return nil, fmt.Errorf("%s not support", event.Type)
	}

	var request *onebot.Request
	if event.Chat.Type == "private" {
		request = onebot.NewPrivateMsgRequest(targetID, segments)
	} else {
		request = onebot.NewGroupMsgRequest(targetID, segments)
	}

	if messageID, err := oc.sendMsg(request); err != nil {
		return nil, err
	} else {
		return &common.OctopusEvent{
			ID:        common.Itoa(messageID),
			Timestamp: time.Now().Unix(),
		}, nil
	}
}

func (oc *OnebotClient) Dispose() {
	oldConn := oc.conn
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

func (oc *OnebotClient) processResponse(resp *onebot.Response) {
	log.Debugf("Receive response: %+v", resp)
	oc.websocketRequestsLock.RLock()
	respChan, ok := oc.websocketRequests[resp.Echo]
	oc.websocketRequestsLock.RUnlock()
	if ok {
		select {
		case respChan <- resp:
		default:
			log.Warnf("Failed to handle response to %s: channel didn't accept response", resp.Echo)
		}
	} else {
		log.Warnf("Dropping response to %s: unknown request ID", resp.Echo)
	}
}

func (oc *OnebotClient) processEvent(event onebot.IEvent) {
	log.Debugf("Receive event: %+v", event)

	key := oc.getEventKey(event)
	oc.mutex.LockKey(key)
	defer oc.mutex.UnlockKey(key)

	switch event.EventType() {
	case onebot.MessagePrivate:
		oc.processPrivateMessage(event.(*onebot.Message))
	case onebot.MessageGroup:
		oc.processGroupMessage(event.(*onebot.Message))
	case onebot.MetaLifecycle:
		oc.processMetaLifycycle(event.(*onebot.LifeCycle))
	case onebot.NoticeOfflineFile:
		oc.processOfflineFile(event.(*onebot.OfflineFile))
	case onebot.NoticeGroupUpload:
		oc.processGroupUpload(event.(*onebot.OfflineFile))
	case onebot.NoticeGroupRecall:
		oc.processGroupRecall(event.(*onebot.GroupRecall))
	case onebot.NoticeFriendRecall:
		oc.processFriendRecall(event.(*onebot.FriendRecall))
	case onebot.MetaHeartbeat:
		log.Debugf("Receive heartbeat: %+v", event.(*onebot.Heartbeat).Status)
	}
}

func (oc *OnebotClient) getEventKey(event onebot.IEvent) string {
	switch event.EventType() {
	case onebot.MessagePrivate:
		m := event.(*onebot.Message)
		targetID := m.Sender.UserID
		if m.PostType == "message_sent" { // sent by self
			targetID = m.TargetID
		}
		return common.Itoa(targetID)
	case onebot.MessageGroup:
		return common.Itoa(event.(*onebot.Message).GroupID)
	case onebot.NoticeOfflineFile:
		return common.Itoa(event.(*onebot.OfflineFile).UserID)
	case onebot.NoticeGroupUpload:
		return common.Itoa(event.(*onebot.OfflineFile).GroupID)
	case onebot.NoticeGroupRecall:
		return common.Itoa(event.(*onebot.GroupRecall).GroupID)
	case onebot.NoticeFriendRecall:
		return common.Itoa(event.(*onebot.FriendRecall).UserID)
	}

	return ""
}

func (oc *OnebotClient) processPrivateMessage(m *onebot.Message) {
	segments := m.Message.([]onebot.ISegment)
	if len(segments) == 0 {
		return
	}

	event := oc.generateEvent(fmt.Sprint(m.MessageID), m.Time)

	targetID := m.Sender.UserID
	if m.PostType == "message_sent" { // sent by self
		targetID = m.TargetID
	}
	targetName := common.Itoa(targetID)
	if target, ok := oc.friends[targetID]; ok {
		targetName = cmp.Or(target.Remark, target.Nickname)
	}

	event.From = common.User{
		ID:       common.Itoa(m.Sender.UserID),
		Username: m.Sender.Nickname,
		Remark:   m.Sender.Card,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(targetID),
		Title: targetName,
	}

	oc.processMessage(event, segments)
}

func (oc *OnebotClient) processGroupMessage(m *onebot.Message) {
	segments := m.Message.([]onebot.ISegment)
	if len(segments) == 0 {
		return
	}

	event := oc.generateEvent(fmt.Sprint(m.MessageID), m.Time)

	targetName := common.Itoa(m.GroupID)
	if target, ok := oc.groups[m.GroupID]; ok {
		targetName = target.Name
	}

	event.From = common.User{
		ID:       common.Itoa(m.Sender.UserID),
		Username: m.Sender.Nickname,
		Remark:   m.Sender.Card,
	}
	event.Chat = common.Chat{
		Type:  "group",
		ID:    common.Itoa(m.GroupID),
		Title: targetName,
	}

	oc.processMessage(event, m.Message.([]onebot.ISegment))
}

func (oc *OnebotClient) processMessage(event *common.OctopusEvent, segments []onebot.ISegment) {
	event.Type = common.EventText

	var summary []string

	photos := []*common.BlobData{}
	for _, s := range segments {
		switch v := s.(type) {
		case *onebot.TextSegment:
			summary = append(summary, v.Content())
		case *onebot.FaceSegment:
			summary = append(summary, fmt.Sprintf("/[Face%s]", v.ID()))
		case *onebot.AtSegment:
			targetName := v.Target()

			groupID, _ := common.Atoi(event.Chat.ID)
			memberID, _ := common.Atoi(v.Target())
			if member, err := oc.getGroupMemberInfo(groupID, memberID, false); err == nil {
				targetName = cmp.Or(member.Card, member.Nickname)
			}
			summary = append(summary, fmt.Sprintf("@%s ", targetName))
		case *onebot.ImageSegment:
			summary = append(summary, "[图片]")
			if v.URL() == "" {
				if bin, err := oc.getMedia(onebot.GetImage, v.File()); err != nil {
					log.Warnf("Download image failed: %v", err)
				} else {
					photos = append(photos, bin)
				}
			} else {
				if bin, err := common.Download(v.URL()); err != nil {
					log.Warnf("Download image failed: %v", err)
				} else {
					bin.Name = v.File()
					photos = append(photos, bin)
				}
			}
		case *onebot.FileSegment:
			if bin, err := oc.getMedia(onebot.GetFile, v.FileID()); err != nil {
				log.Warnf("Download file failed: %v", err)
				event.Content = "[文件下载失败]"
			} else {
				event.Type = common.EventFile
				event.Data = bin
			}
		case *onebot.RecordSegment:
			if bin, err := oc.getMedia(onebot.GetRecord, v.File()); err != nil {
				log.Warnf("Download record failed: %v", err)
				event.Content = "[语音下载失败]"
			} else {
				event.Type = common.EventAudio
				event.Data = bin
			}
		case *onebot.VideoSegment:
			if bin, err := oc.getMedia(onebot.GetFile, v.FileID()); err != nil {
				log.Warnf("Download video failed: %v", err)
				event.Content = "[视频下载失败]"
			} else {
				event.Type = common.EventVideo
				event.Data = bin
			}
		case *onebot.ReplySegment:
			event.Reply = &common.ReplyInfo{
				ID:        v.ID(),
				Timestamp: 0,
			}
		case *onebot.ForwardSegment:
			event.Type = common.EventApp
			event.Data = oc.convertForward(v.ID())
		case *onebot.JSONSegment:
			content := v.Content()
			view := gjson.Get(content, "view").String()
			if view == "LocationShare" {
				name := gjson.Get(content, "meta.*.name").String()
				address := gjson.Get(content, "meta.*.address").String()
				latitude := gjson.Get(content, "meta.*.lat").Float()
				longitude := gjson.Get(content, "meta.*.lng").Float()
				event.Type = common.EventLocation
				event.Data = &common.LocationData{
					Name:      name,
					Address:   address,
					Longitude: longitude,
					Latitude:  latitude,
				}
			} else {
				if url := gjson.Get(content, "meta.*.qqdocurl").String(); len(url) > 0 {
					title := gjson.Get(content, "meta.*.title").String()
					desc := gjson.Get(content, "meta.*.desc").String()
					prompt := gjson.Get(content, "prompt").String()
					event.Type = common.EventApp
					event.Data = &common.AppData{
						Title:       prompt,
						Description: desc,
						Source:      title,
						URL:         url,
					}
				} else if jumpUrl := gjson.Get(content, "meta.*.jumpUrl").String(); len(jumpUrl) > 0 {
					//title := gjson.Get(v.Content, "meta.*.title").String()
					desc := gjson.Get(content, "meta.*.desc").String()
					prompt := gjson.Get(content, "prompt").String()
					tag := gjson.Get(content, "meta.*.tag").String()
					event.Type = common.EventApp
					event.Data = &common.AppData{
						Title:       prompt,
						Description: desc,
						Source:      tag,
						URL:         jumpUrl,
					}
				}
			}
		default:
			summary = append(summary, fmt.Sprintf("[%v]", s.SegmentType()))
		}
	}

	if len(summary) > 0 {
		if len(summary) == 1 && segments[0].SegmentType() == onebot.Image {
			event.Type = common.EventPhoto
			event.Data = photos

			if segments[0].(*onebot.ImageSegment).IsSticker {
				event.Type = common.EventSticker
				event.Data = photos[0]
			}
		} else {
			event.Content = strings.Join(summary, "")

			if len(photos) > 0 {
				event.Type = common.EventPhoto
				event.Data = photos
			}
		}
	}

	oc.pushEvent(event)
}

func (oc *OnebotClient) processMetaLifycycle(m *onebot.LifeCycle) {
	if m.SubType == "connect" {
		time.Sleep(time.Minute)
		oc.updateChats()
	}
}

func (oc *OnebotClient) processOfflineFile(m *onebot.OfflineFile) {
	// FIXME: can't get message id
	event := oc.generateEvent(fmt.Sprint(time.Now().Unix()), time.Now().UnixMilli())

	targetID := m.UserID
	targetName := common.Itoa(targetID)
	if target, ok := oc.friends[targetID]; ok {
		targetName = cmp.Or(target.Remark, target.Nickname)
	}

	event.From = common.User{
		ID:       common.Itoa(targetID),
		Username: targetName,
		Remark:   targetName,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(targetID),
		Title: targetName,
	}

	if bin, err := common.Download(m.File.URL); err != nil {
		log.Warnf("Download file failed: %v", err)
		event.Content = "[文件下载失败]"
	} else {
		bin.Name = m.File.Name
		event.Type = common.EventFile
		event.Data = bin
	}

	oc.pushEvent(event)
}

func (oc *OnebotClient) processGroupUpload(m *onebot.OfflineFile) {
	// FIXME: can't get message id
	event := oc.generateEvent(fmt.Sprint(time.Now().Unix()), time.Now().UnixMilli())

	groupName := common.Itoa(m.GroupID)
	if group, ok := oc.groups[m.GroupID]; ok {
		groupName = group.Name
	}

	targetName := common.Itoa(m.UserID)
	if member, err := oc.getGroupMemberInfo(m.GroupID, m.UserID, false); err == nil {
		targetName = cmp.Or(member.Card, member.Nickname)
	}

	event.From = common.User{
		ID:       common.Itoa(m.UserID),
		Username: targetName,
		Remark:   targetName,
	}
	event.Chat = common.Chat{
		Type:  "group",
		ID:    common.Itoa(m.GroupID),
		Title: groupName,
	}

	if bin, err := common.Download(m.File.URL); err != nil {
		log.Warnf("Download file failed: %v", err)
		event.Content = "[文件下载失败]"
	} else {
		bin.Name = m.File.Name
		event.Type = common.EventFile
		event.Data = bin
	}

	oc.pushEvent(event)
}

func (oc *OnebotClient) processGroupRecall(m *onebot.GroupRecall) {
	event := oc.generateEvent(fmt.Sprint(time.Now().Unix()), time.Now().UnixMilli())

	groupName := common.Itoa(m.GroupID)
	if group, ok := oc.groups[m.GroupID]; ok {
		groupName = group.Name
	}

	targetName := common.Itoa(m.OperatorID)
	if member, err := oc.getGroupMemberInfo(m.GroupID, m.OperatorID, false); err == nil {
		targetName = cmp.Or(member.Card, member.Nickname)
	}

	event.From = common.User{
		ID:       common.Itoa(m.OperatorID),
		Username: targetName,
		Remark:   targetName,
	}
	event.Chat = common.Chat{
		Type:  "group",
		ID:    common.Itoa(m.GroupID),
		Title: groupName,
	}

	event.Type = common.EventRevoke
	event.Content = "recalled a message"

	event.Reply = &common.ReplyInfo{
		ID:        common.Itoa(m.MessageID),
		Timestamp: 0,
		Sender:    targetName,
	}

	oc.pushEvent(event)
}

func (oc *OnebotClient) processFriendRecall(m *onebot.FriendRecall) {
	event := oc.generateEvent(fmt.Sprint(time.Now().Unix()), time.Now().UnixMilli())

	if m.UserID == oc.self.ID { // recall self
		log.Infof("Failed to recall self sent private message #%d", m.MessageID)
		return
	}

	targetID := m.UserID
	targetName := common.Itoa(targetID)
	if target, ok := oc.friends[targetID]; ok {
		targetName = cmp.Or(target.Remark, target.Nickname)
	}

	event.From = common.User{
		ID:       common.Itoa(targetID),
		Username: targetName,
		Remark:   targetName,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(targetID),
		Title: targetName,
	}

	event.Type = common.EventRevoke
	event.Content = "recalled a message"

	event.Reply = &common.ReplyInfo{
		ID:        common.Itoa(m.MessageID),
		Timestamp: 0,
		Sender:    targetName,
	}

	oc.pushEvent(event)
}

func (oc *OnebotClient) getGroupMemberInfo(groupID, userID int64, noCache bool) (*onebot.Sender, error) {
	resp, err := oc.request(onebot.NewGetGroupMemberInfoRequest(groupID, userID, noCache))
	if err == nil {
		var s onebot.Sender
		err = mapstructure.WeakDecode(resp, &s)
		return &s, err
	}

	return nil, err
}

func (oc *OnebotClient) getMedia(t onebot.RequestType, file string) (*common.BlobData, error) {
	var request *onebot.Request
	switch t {
	case onebot.GetRecord:
		request = onebot.NewGetRecordRequest(file)
	case onebot.GetImage:
		request = onebot.NewGetImageRequest(file)
	case onebot.GetFile:
		request = onebot.NewGetFileRequest(file)
	default:
		return nil, fmt.Errorf("request type not support: %+v", t)
	}

	count := 0

	for {
		count += 1
		resp, err := oc.request(request)
		if err == nil {
			var f onebot.FileInfo
			if err = mapstructure.WeakDecode(resp, &f); err != nil {
				return nil, err
			}

			if f.Base64 != "" {
				var data []byte
				if data, err = base64.StdEncoding.DecodeString(f.Base64); err == nil {
					return &common.BlobData{
						Name:   f.FileName,
						Mime:   mimetype.Detect(data).String(),
						Binary: data,
					}, nil
				}
			} else {
				var bin *common.BlobData
				if bin, err = common.Download(f.URL); err == nil {
					bin.Name = f.FileName
					return bin, nil
				}
			}
		}

		if count > 3 {
			return nil, err
		}
		time.Sleep(3 * time.Second)
	}
}

func (oc *OnebotClient) getMsg(id int32) (*onebot.BareMessage, error) {
	resp, err := oc.request(onebot.NewGetMsgRequest(id))
	if err == nil {
		var message onebot.BareMessage
		err = mapstructure.WeakDecode(resp, &message)
		return &message, err
	}

	return nil, err
}

func (oc *OnebotClient) forwardFriendSingleMsg(userID int64, messageID int32) error {
	_, err := oc.request(onebot.NewPrivateForwardRequest(userID, messageID))
	return err
}

func (oc *OnebotClient) forwardGroupSingleMsg(groupID int64, messageID int32) error {
	_, err := oc.request(onebot.NewGroupForwardRequest(groupID, messageID))
	return err
}

func (oc *OnebotClient) getForwardMsg(id string) ([]*onebot.Message, error) {
	resp, err := oc.request(onebot.NewGetForwardMsgRequest(id))
	if err == nil {
		messages := []*onebot.Message{}
		msgs := resp.(map[string]interface{})["messages"]
		for _, msg := range msgs.([]interface{}) {
			if message, err := onebot.UnmarshalPayload(msg.(map[string]interface{})); err == nil {
				messages = append(messages, message.(*onebot.Message))
			}
		}
		return messages, err
	}
	return nil, err
}

func (oc *OnebotClient) sendMsg(request *onebot.Request) (int64, error) {
	resp, err := oc.request(request)
	if err == nil {
		return int64(resp.(map[string]interface{})["message_id"].(float64)), nil
	}
	return 0, err
}

// Lagrange.OneBot
func (oc *OnebotClient) uploadPrivateFile(userID int64, file string, name string) error {
	_, err := oc.request(onebot.NewUploadPrivateFileRequest(userID, file, name))
	return err
}

// Lagrange.OneBot
func (oc *OnebotClient) uploadGroupFile(groupID int64, file string, name string) error {
	_, err := oc.request(onebot.NewUploadGroupFileRequest(groupID, file, name))
	return err
}

func (oc *OnebotClient) convertForward(id string) *common.AppData {
	var summary []string
	var content []string
	var blobs = map[string]*common.BlobData{}

	var handleForward func(level int, nodes []*onebot.Message)
	handleForward = func(level int, nodes []*onebot.Message) {
		summary = append(summary, "ForwardMessage:\n")
		if level > 0 {
			content = append(content, "<blockquote>")
		}

		for _, node := range nodes {
			name := cmp.Or(node.Sender.Card, node.Sender.Nickname, fmt.Sprint(node.Sender.UserID))

			summary = append(summary, fmt.Sprintf("%s:\n", name))
			content = append(content, fmt.Sprintf("<strong>%s:</strong><p>", name))
			for _, s := range node.Message.([]onebot.ISegment) {
				switch v := s.(type) {
				case *onebot.TextSegment:
					summary = append(summary, v.Content())
					content = append(content, v.Content())
				case *onebot.FaceSegment:
					summary = append(summary, fmt.Sprintf("/[Face%s]", v.ID()))
					content = append(content, fmt.Sprintf("/[Face%s]", v.ID()))
				case *onebot.AtSegment:
					summary = append(summary, fmt.Sprintf("@%s ", v.Target()))
					content = append(content, fmt.Sprintf("@%s ", v.Target()))
				case *onebot.ImageSegment:
					summary = append(summary, "[图片]")

					var bin *common.BlobData
					var err error
					if v.URL() == "" {
						bin, err = oc.getMedia(onebot.GetImage, v.File())
					} else {
						bin, err = common.Download(v.URL())
					}
					if err != nil {
						log.Warnf("Download image failed: %v", err)
						content = append(content, "[图片]")
					} else {
						bin.Name = v.File()
						blobs[v.File()] = bin
						content = append(content, fmt.Sprintf("<img src=\"%s%s\">", common.REMOTE_PREFIX, v.File()))
					}
				case *onebot.ForwardSegment:
					if messages, err := oc.getForwardMsg(v.ID()); err == nil {
						handleForward(level+1, messages)
					} else {
						log.Warnf("Failed to get forward #%s", v.ID())
						summary = append(summary, "[转发]")
						content = append(content, "[转发]")
					}
				default:
					summary = append(summary, fmt.Sprintf("[%v]", s.SegmentType()))
					content = append(content, fmt.Sprintf("[%v]", s.SegmentType()))
				}
			}
			summary = append(summary, "\n")
			content = append(content, "</p>")
		}

		if level > 0 {
			content = append(content, "</blockquote>")
		}
	}

	if messages, err := oc.getForwardMsg(id); err == nil {
		handleForward(0, messages)
	} else {
		log.Warnf("Failed to get forward #%s", id)
	}

	return &common.AppData{
		Title:       fmt.Sprintf("[聊天记录 %s]", id),
		Description: strings.Join(summary, ""),
		Content:     strings.Join(content, ""),
		Blobs:       blobs,
	}
}

func (oc *OnebotClient) updateChats() {
	if resp, err := oc.request(onebot.NewGetFriendListRequest()); err == nil {
		friends := map[int64]*onebot.FriendInfo{}

		for _, friend := range resp.([]interface{}) {
			var f onebot.FriendInfo
			if err := mapstructure.WeakDecode(friend.(map[string]interface{}), &f); err != nil {
				continue
			}
			friends[f.ID] = &f
		}

		oc.friends = friends
	}

	if resp, err := oc.request(onebot.NewGetLoginInfoRequest()); err == nil {
		var f onebot.FriendInfo
		if err := mapstructure.WeakDecode(resp.(map[string]interface{}), &f); err == nil {
			oc.self = &f
			oc.friends[f.ID] = &f
		}
	}

	if resp, err := oc.request(onebot.NewGetGroupListRequest()); err == nil {
		groups := map[int64]*onebot.GroupInfo{}

		for _, group := range resp.([]interface{}) {
			var g onebot.GroupInfo
			if err := mapstructure.WeakDecode(group.(map[string]interface{}), &g); err != nil {
				continue
			}
			groups[g.ID] = &g
		}

		oc.groups = groups
	}

	// Sync chats
	event := oc.generateEvent("sync", time.Now().UnixMilli())

	chats := []*common.Chat{}

	for _, f := range oc.friends {
		chats = append(chats, &common.Chat{
			ID:    common.Itoa(f.ID),
			Type:  "private",
			Title: cmp.Or(f.Remark, f.Nickname),
		})
	}
	for _, g := range oc.groups {
		chats = append(chats, &common.Chat{
			ID:    common.Itoa(g.ID),
			Type:  "group",
			Title: g.Name,
		})
	}

	event.Type = common.EventSync
	event.Data = chats

	oc.pushEvent(event)
}

func (oc *OnebotClient) generateEvent(id string, ts int64) *common.OctopusEvent {
	return &common.OctopusEvent{
		Vendor:    *oc.vendor,
		ID:        id,
		Timestamp: ts,
	}
}

func (oc *OnebotClient) pushEvent(event *common.OctopusEvent) {
	event = oc.s2m.Apply(event)

	oc.out <- event
}

func (oc *OnebotClient) request(req *onebot.Request) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), oc.config.Service.SendTiemout)
	defer cancel()

	req.Echo = fmt.Sprint(atomic.AddInt64(&oc.websocketRequestID, 1))

	respChan := make(chan *onebot.Response, 1)

	oc.addWebsocketResponseWaiter(req.Echo, respChan)
	defer oc.removeWebsocketResponseWaiter(req.Echo, respChan)

	log.Debugf("Send request message #%s %s %+v", req.Echo, req.Action, req)
	if err := oc.sendMessage(req); err != nil {
		return nil, err
	}

	select {
	case resp := <-respChan:
		if resp.Status != "ok" {
			return resp, fmt.Errorf("%s response retcode: %d", resp.Status, resp.Retcode)
		} else {
			return resp.Data, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (oc *OnebotClient) sendMessage(msg *onebot.Request) error {
	conn := oc.conn
	if msg == nil {
		return nil
	} else if conn == nil {
		return ErrWebsocketNotConnected
	}
	oc.writeLock.Lock()
	defer oc.writeLock.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(oc.config.Service.SendTiemout))
	return conn.WriteJSON(msg)
}

func (oc *OnebotClient) addWebsocketResponseWaiter(echo string, waiter chan<- *onebot.Response) {
	oc.websocketRequestsLock.Lock()
	oc.websocketRequests[echo] = waiter
	oc.websocketRequestsLock.Unlock()
}

func (oc *OnebotClient) removeWebsocketResponseWaiter(echo string, waiter chan<- *onebot.Response) {
	oc.websocketRequestsLock.Lock()
	existingWaiter, ok := oc.websocketRequests[echo]
	if ok && existingWaiter == waiter {
		delete(oc.websocketRequests, echo)
	}
	oc.websocketRequestsLock.Unlock()
	close(waiter)
}
