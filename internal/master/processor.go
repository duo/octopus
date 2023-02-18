package master

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"image"
	"io"
	"os"
	"runtime/debug"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	"github.com/duo/octopus/internal/common"
	"github.com/duo/octopus/internal/manager"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/gabriel-vasile/mimetype"

	log "github.com/sirupsen/logrus"
)

const (
	imgMinSize      = 1600
	imgMaxSize      = 1200
	imgSizeRatio    = 3.5
	imgSizeMaxRatio = 10
)

type ChatInfo struct {
	id       int64
	threadID int64
	title    string
}

// read events from limb
func (ms *MasterService) handleSlaveLoop() {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("Panic in handle slave loop: %v\n%s", panicErr, debug.Stack())
		}
	}()

	for event := range ms.in {
		if event.Type == common.EventSync {
			go ms.updateChats(event)
		} else {
			event := event
			go func() {
				ms.mutex.LockKey(event.Chat.ID)
				defer ms.mutex.UnlockKey(event.Chat.ID)

				ms.processSlaveEvent(event)
			}()
		}
	}
}

// process master message
func (ms *MasterService) processMasterMessage(ctx *ext.Context) error {
	masterLimb := common.Limb{
		Type:   "telegram",
		UID:    common.Itoa(ms.config.Master.AdminID),
		ChatID: common.Itoa(ctx.EffectiveChat.Id),
	}.String()

	rawMsg := ctx.EffectiveMessage

	log.Debugf("Receive Telegram message: %+v", rawMsg)

	// find linked limb chat
	if ctx.EffectiveChat.IsForum {
		topicID := rawMsg.MessageThreadId
		if topicID == 0 {
			return ms.replayLinkIssue(rawMsg, "*Chat on default topic not allowed.*")
		} else {
			if topic, err := manager.GetTopicByMaster(masterLimb, topicID); err != nil {
				log.Warnf("Get topic by master failed: %v", err)
				return err
			} else {
				if topic == nil {
					return ms.replayLinkIssue(rawMsg, "*No linked chat on topic found.*")
				} else {
					return ms.transferMasterMessage(ctx, topic.SlaveLimb)
				}
			}
		}
	} else if ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup" {
		if links, err := manager.GetLinksByMaster(masterLimb); err != nil {
			log.Warnf("Get links by master failed: %v", err)
			return err
		} else {
			if len(links) == 0 {
				return ms.replayLinkIssue(rawMsg, "*No linked chat on group found.*")
			} else if len(links) > 1 {
				return ms.replayLinkIssue(rawMsg, "*Multiple linked chat found.*")
			} else {
				return ms.transferMasterMessage(ctx, links[0].SlaveLimb)
			}
		}
	} else if rawMsg.ReplyToMessage != nil {
		logMsg, err := manager.GetMessageByMasterMsgId(
			masterLimb,
			common.Itoa(rawMsg.ReplyToMessage.MessageId),
		)
		if err != nil {
			log.Warnf("Get message by master message id failed: %v", err)
			return err
		} else if logMsg == nil {
			return ms.replayLinkIssue(rawMsg, "*No linked chat by reply found.*")
		} else {
			return ms.transferMasterMessage(ctx, logMsg.SlaveLimb)
		}
	} else {
		return ms.replayLinkIssue(rawMsg, "*No linked chat found.*")
	}
}

// convert master message to octopus event and push
func (ms *MasterService) transferMasterMessage(ctx *ext.Context, slaveLimb string) error {
	chat, err := manager.GetChat(slaveLimb)
	if err != nil {
		return err
	}
	if chat == nil {
		return errors.New(slaveLimb + " not found.")
	}

	limb, _ := common.LimbFromString(slaveLimb)
	meLimb := common.Limb{
		Type:   limb.Type,
		UID:    limb.UID,
		ChatID: limb.UID,
	}.String()

	// get self
	me, err := manager.GetChat(meLimb)
	if err != nil {
		return err
	}
	if me == nil {
		return errors.New(meLimb + " not found.")
	}

	rawMsg := ctx.EffectiveMessage

	// generate a basic event
	event := &common.OctopusEvent{
		Vendor: common.Vendor{
			Type: limb.Type,
			UID:  limb.UID,
		},
		ID:        common.Itoa(rawMsg.MessageId),
		Timestamp: rawMsg.Date,
		From: common.User{
			ID:       limb.UID,
			Username: me.Title,
			Remark:   me.Title,
		},
		Chat: common.Chat{
			Type:  chat.ChatType,
			ID:    limb.ChatID,
			Title: chat.Title,
		},
		Type:    common.EventText,
		Content: rawMsg.Text,
		Callback: func(event *common.OctopusEvent, err error) {
			ms.transferCallback(rawMsg, event, err)
		},
	}

	// process reply message
	if rawMsg.ReplyToMessage != nil &&
		//(!ctx.EffectiveChat.IsForum || rawMsg.ReplyToMessage.MessageId != rawMsg.ReplyToMessage.MessageThreadId) {
		rawMsg.ReplyToMessage.MessageId != rawMsg.ReplyToMessage.MessageThreadId {
		masterLimb := common.Limb{
			Type:   "telegram",
			UID:    common.Itoa(ms.config.Master.AdminID),
			ChatID: common.Itoa(ctx.EffectiveChat.Id),
		}.String()
		logMsg, err := manager.GetMessageByMasterMsgId(
			masterLimb,
			common.Itoa(rawMsg.ReplyToMessage.MessageId),
		)
		if err == nil && logMsg != nil && logMsg.SlaveMsgID != "0" {
			event.Reply = &common.ReplyInfo{
				ID:        logMsg.SlaveMsgID,
				Timestamp: logMsg.Timestamp,
				Sender:    logMsg.SlaveSender,
				Content:   logMsg.Content,
			}
		}
	}

	if rawMsg.Photo != nil {
		// TODO: group media
		event.Type = common.EventPhoto
		if blob, err := ms.download(rawMsg.Photo[len(rawMsg.Photo)-1].FileId); err != nil {
			return err
		} else {
			event.Data = []*common.BlobData{blob}
		}
	} else if rawMsg.Sticker != nil {
		event.Type = common.EventPhoto
		if blob, err := ms.download(rawMsg.Sticker.FileId); err != nil {
			return err
		} else {
			event.Data = []*common.BlobData{blob}
		}
	} else if rawMsg.Animation != nil {
		event.Type = common.EventPhoto
		if blob, err := ms.download(rawMsg.Animation.FileId); err != nil {
			return err
		} else {
			event.Data = []*common.BlobData{blob}
		}
	} else if rawMsg.Voice != nil {
		event.Type = common.EventAudio
		if blob, err := ms.download(rawMsg.Voice.FileId); err != nil {
			return err
		} else {
			event.Data = blob
		}
	} else if rawMsg.Audio != nil {
		event.Type = common.EventAudio
		if blob, err := ms.download(rawMsg.Audio.FileId); err != nil {
			return err
		} else {
			if rawMsg.Audio.FileName != "" {
				blob.Name = rawMsg.Audio.FileName
			}
			event.Data = blob
		}
	} else if rawMsg.Video != nil {
		event.Type = common.EventVideo
		if blob, err := ms.download(rawMsg.Video.FileId); err != nil {
			return err
		} else {
			if rawMsg.Video.FileName != "" {
				blob.Name = rawMsg.Video.FileName
			}
			event.Data = blob
		}
	} else if rawMsg.Document != nil {
		event.Type = common.EventFile
		if blob, err := ms.download(rawMsg.Document.FileId); err != nil {
			return err
		} else {
			if rawMsg.Document.FileName != "" {
				blob.Name = rawMsg.Document.FileName
			}
			event.Data = blob
		}
	} else if rawMsg.Location != nil {
		event.Type = common.EventLocation
		event.Data = &common.LocationData{
			Name: "Location",
			Address: fmt.Sprintf(
				"Latitude: %.5f Longitude: %.5f",
				rawMsg.Location.Latitude,
				rawMsg.Location.Longitude,
			),
			Longitude: rawMsg.Location.Longitude,
			Latitude:  rawMsg.Location.Latitude,
		}
	} else if rawMsg.Venue != nil {
		event.Type = common.EventLocation
		event.Data = &common.LocationData{
			Name:      rawMsg.Venue.Title,
			Address:   rawMsg.Venue.Address,
			Longitude: rawMsg.Venue.Location.Longitude,
			Latitude:  rawMsg.Venue.Location.Latitude,
		}
	} else if rawMsg.Text == "" {
		return fmt.Errorf("message type not support: %+v", rawMsg)
	}

	ms.out <- event

	return nil
}

// process limb client event response
func (ms *MasterService) transferCallback(rawMSg *gotgbot.Message, event *common.OctopusEvent, cbErr error) {
	if cbErr != nil {
		ms.replayLinkIssue(rawMSg, fmt.Sprintf("*[FAIL]: %s*", strings.NewReplacer("*", "\\*").Replace(cbErr.Error())))
		return
	}

	masterLimb := common.Limb{
		Type:   "telegram",
		UID:    common.Itoa(ms.config.Master.AdminID),
		ChatID: common.Itoa(rawMSg.Chat.Id),
	}.String()
	slaveLimb := common.Limb{
		Type:   event.Vendor.Type,
		UID:    event.Vendor.UID,
		ChatID: event.Chat.ID,
	}.String()

	msg := &manager.Message{
		MasterLimb:        masterLimb,
		MasterMsgID:       common.Itoa(rawMSg.MessageId),
		MasterMsgThreadID: common.Itoa(rawMSg.MessageThreadId),
		SlaveLimb:         slaveLimb,
		SlaveMsgID:        event.ID,
		SlaveSender:       event.From.ID,
		Content:           event.Content,
		Timestamp:         event.Timestamp,
	}

	if err := manager.AddMessage(msg); err != nil {
		log.Warnf("Failed to add message: %+v %v", msg, err)
	} else {
		log.Debugf("Add message: %+v", msg)
	}
}

// process events from limb client
func (ms *MasterService) processSlaveEvent(event *common.OctopusEvent) {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("Panic in handle slave event: %+v %v\n%s", event, panicErr, debug.Stack())
		}
	}()

	log.Debugf("Receive octopus event: %+v", event)

	adminID := ms.config.Master.AdminID

	// handle observe event
	if event.Type == common.EventObserve {
		ms.bot.SendMessage(
			adminID,
			fmt.Sprintf("*[INFO]: %s*", strings.NewReplacer("*", "\\*").Replace(event.Content)),
			&gotgbot.SendMessageOpts{
				ParseMode: "Markdown",
			})
		return
	}

	slaveLimb := common.Limb{
		Type:   event.Vendor.Type,
		UID:    event.Vendor.UID,
		ChatID: event.Chat.ID,
	}.String()

	links, err := manager.GetLinksBySlave(slaveLimb)
	if err != nil {
		log.Warnf("Get links by slave failed: %v", err)
		return
	}
	log.Debugf("Links by slave(%s): %+v", slaveLimb, links)

	var replyMap = map[int64]int64{}
	// get reply map for quote and revoke
	if event.Reply != nil {
		messages, err := manager.GetMessagesBySlaveReply(slaveLimb, event.Reply)
		if err != nil {
			log.Warnf("Get reply messages failed: %v", err)
			return
		}
		for _, m := range messages {
			limb, err := common.LimbFromString(m.MasterLimb)
			if err != nil {
				log.Warnf("Parse limb(%v) failed: %v", m.MasterLimb, err)
				continue
			}
			chatID, err := common.Atoi(limb.ChatID)
			if err != nil {
				log.Warnf("Parse chatId(%v) failed: %v", limb.ChatID, err)
				continue
			}
			masterMsgID, err := common.Atoi(m.MasterMsgID)
			if err != nil {
				log.Warnf("Parse mastetMsgId(%v) failed: %v", m.MasterMsgID, err)
				continue
			}
			replyMap[chatID] = masterMsgID
		}
	}

	chats := []*ChatInfo{}

	if len(links) > 0 {
		// find linked Telegram chat
		for _, l := range links {
			limb, err := common.LimbFromString(l.MasterLimb)
			if err != nil {
				log.Warnf("Parse limb(%v) failed: %v", l.MasterLimb, err)
				continue
			}
			chatID, err := common.Atoi(limb.ChatID)
			if err != nil {
				log.Warnf("Parse chatId(%v) failed: %v", limb.ChatID, err)
				continue
			}
			chat, err := ms.bot.GetChat(chatID, nil)
			if err != nil {
				log.Warnf("Failed to get chat(%d) info from Telegram: %v", chatID, err)
				continue
			}
			if chat.IsForum {
				chats = append(chats, ms.createForumChatInfo(chatID, event))
			} else {
				chats = append(chats, &ChatInfo{
					id:    chatID,
					title: fmt.Sprintf("%s:", displayName(&event.From)),
				})
			}
		}
	} else if chatID, ok := ms.archiveChats[event.Vendor.String()]; ok {
		// find archive supergroup (topic enabled)
		chats = append(chats, ms.createForumChatInfo(chatID, event))
	} else {
		var title string
		if event.Chat.Type == "private" {
			title = fmt.Sprintf("👤 %s:", displayName(&event.From))
		} else {
			title = fmt.Sprintf("👥 %s [%s]:", displayName(&event.From), event.Chat.Title)
		}
		chats = append(chats, &ChatInfo{
			id:    adminID,
			title: title,
		})
	}

	for _, chat := range chats {
		var replyToMessageID int64 = 0
		if val, ok := replyMap[chat.id]; ok {
			replyToMessageID = val
		}

		switch event.Type {
		case common.EventRevoke:
			ms.bot.SendChatAction(chat.id, "typing", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			resp, err := ms.bot.SendMessage(
				chat.id,
				fmt.Sprintf(
					"%s\n~%s~",
					common.EscapeText("MarkdownV2", chat.title),
					common.EscapeText("MarkdownV2", event.Content),
				),
				&gotgbot.SendMessageOpts{
					ParseMode:        "MarkdownV2",
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventText, common.EventSystem:
			ms.bot.SendChatAction(chat.id, "typing", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			resp, err := ms.bot.SendMessage(
				chat.id,
				fmt.Sprintf("%s\n%s", chat.title, event.Content),
				&gotgbot.SendMessageOpts{
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventVoIP:
			ms.bot.SendChatAction(chat.id, "typing", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			resp, err := ms.bot.SendMessage(
				chat.id,
				fmt.Sprintf(
					"%s\n_%s_",
					common.EscapeText("MarkdownV2", chat.title),
					common.EscapeText("MarkdownV2", event.Content),
				),
				&gotgbot.SendMessageOpts{
					ParseMode:        "MarkdownV2",
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventLocation:
			location := event.Data.(*common.LocationData)
			resp, err := ms.bot.SendVenue(
				chat.id,
				location.Latitude,
				location.Longitude,
				fmt.Sprintf("%s\n%s", chat.title, location.Name),
				location.Address,
				&gotgbot.SendVenueOpts{
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventApp:
			app := event.Data.(*common.AppData)
			text := fmt.Sprintf("%s\n<u>%s</u>\n\n%s",
				chat.title,
				html.EscapeString(app.Title),
				html.EscapeString(app.Description),
			)
			if app.URL != "" {
				source := html.EscapeString(app.Source)
				if source == "" {
					source = app.URL
				}
				text = fmt.Sprintf("%s\n\nvia <a href=\"%s\">%s</a>",
					text,
					app.URL,
					source,
				)
			}

			if ms.config.Master.Telegraph.Enable && len(ms.config.Master.Telegraph.Tokens) > 0 {
				if page, err := ms.postApp(app); err == nil {
					text = fmt.Sprintf("<a href=\"%s\">%s</a>",
						page.URL,
						page.Title,
					)
				}
			}

			ms.bot.SendChatAction(chat.id, "typing", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			resp, err := ms.bot.SendMessage(
				chat.id,
				text,
				&gotgbot.SendMessageOpts{
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
					ParseMode:        "HTML",
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventAudio:
			ms.bot.SendChatAction(chat.id, "upload_voice", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			blob := event.Data.(*common.BlobData)
			resp, err := ms.bot.SendVoice(
				chat.id,
				blob.Binary,
				&gotgbot.SendVoiceOpts{
					Caption:          chat.title,
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventVideo:
			ms.bot.SendChatAction(chat.id, "upload_video", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			blob := event.Data.(*common.BlobData)
			//mime := mimetype.Detect(blob.Binary)
			//fileName := fmt.Sprintf("%s%s", msg.ID, mime.Extension())
			text := fmt.Sprintf("%s\n%s", chat.title, event.Content)
			resp, err := ms.bot.SendVideo(
				chat.id,
				//&gotgbot.NamedFile{
				//	File:     bytes.NewReader(blob.Binary),
				//	FileName: fileName,
				//},
				blob.Binary,
				&gotgbot.SendVideoOpts{
					Caption:          text,
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventFile:
			ms.bot.SendChatAction(chat.id, "upload_document", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
			blob := event.Data.(*common.BlobData)
			resp, err := ms.bot.SendDocument(
				chat.id,
				&gotgbot.NamedFile{
					File:     bytes.NewReader(blob.Binary),
					FileName: blob.Name,
				},
				&gotgbot.SendDocumentOpts{
					Caption:          chat.title,
					MessageThreadId:  chat.threadID,
					ReplyToMessageId: replyToMessageID,
				},
			)
			ms.logMessage(chat, event, resp, err)
		case common.EventPhoto:
			text := fmt.Sprintf("%s\n%s", chat.title, event.Content)
			photos := event.Data.([]*common.BlobData)
			if len(photos) == 1 {
				photo := photos[0]
				ms.bot.SendChatAction(chat.id, "upload_photo", &gotgbot.SendChatActionOpts{MessageThreadId: chat.threadID})
				mime := mimetype.Detect(photo.Binary)
				if mime.String() == "image/gif" {
					resp, err := ms.bot.SendAnimation(
						chat.id,
						&gotgbot.NamedFile{
							File:     bytes.NewReader(photo.Binary),
							FileName: photo.Name + ".gif",
						},
						&gotgbot.SendAnimationOpts{
							Caption:          text,
							MessageThreadId:  chat.threadID,
							ReplyToMessageId: replyToMessageID,
						},
					)
					ms.logMessage(chat, event, resp, err)
				} else if isSendAsFile(photo.Binary) {
					resp, err := ms.bot.SendDocument(
						chat.id,
						&gotgbot.NamedFile{
							File:     bytes.NewReader(photo.Binary),
							FileName: photo.Name,
						},
						&gotgbot.SendDocumentOpts{
							Caption:          text,
							MessageThreadId:  chat.threadID,
							ReplyToMessageId: replyToMessageID,
						},
					)
					ms.logMessage(chat, event, resp, err)
				} else {
					resp, err := ms.bot.SendPhoto(
						chat.id,
						photo.Binary,
						&gotgbot.SendPhotoOpts{
							Caption:          text,
							MessageThreadId:  chat.threadID,
							ReplyToMessageId: replyToMessageID,
						},
					)
					ms.logMessage(chat, event, resp, err)
				}
			} else {
				var mediaGroup []gotgbot.InputMedia
				for i, photo := range photos {
					if i == 10 {
						break
					}

					caption := ""
					if i == 0 {
						caption = text
					}

					/*
						mime := mimetype.Detect(photo.Binary)
						if mime.String() == "image/gif" {
							mediaGroup = append(mediaGroup, gotgbot.InputMediaAnimation{
								Media: &gotgbot.NamedFile{
									File:     bytes.NewReader(photo.Binary),
									FileName: photo.Name,
								},
								Caption: caption,
							})
						} else if isSendAsFile(photo.Binary) {
							mediaGroup = append(mediaGroup, gotgbot.InputMediaDocument{
								Media: &gotgbot.NamedFile{
									File:     bytes.NewReader(photo.Binary),
									FileName: photo.Name,
								},
								Caption: caption,
							})
						} else {
							mediaGroup = append(mediaGroup, gotgbot.InputMediaPhoto{
								Media: &gotgbot.NamedFile{
									File:     bytes.NewReader(photo.Binary),
									FileName: photo.Name,
								},
								Caption: caption,
							})
						}
					*/
					mediaGroup = append(mediaGroup, gotgbot.InputMediaPhoto{
						Media: &gotgbot.NamedFile{
							File:     bytes.NewReader(photo.Binary),
							FileName: photo.Name,
						},
						Caption: caption,
					})
				}
				resps, err := ms.bot.SendMediaGroup(
					chat.id,
					mediaGroup,
					&gotgbot.SendMediaGroupOpts{
						MessageThreadId:  chat.threadID,
						ReplyToMessageId: replyToMessageID,
					},
				)
				if err != nil {
					log.Warnf("Failed to send to Telegram: %v", err)
				} else {
					for _, resp := range resps {
						ms.logMessage(chat, event, &resp, err)
					}
				}
			}
		default:
			log.Warnf("event type not support: %s", event.Type)
		}
	}
}

// update chats from limb client
func (ms *MasterService) updateChats(event *common.OctopusEvent) {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("Panic in update chats event: %+v %v\n%s", event, panicErr, debug.Stack())
		}
	}()

	chats := event.Data.([]*common.Chat)
	log.Infof("Update chats for %s, count: %d", event.Vendor, len(chats))
	for _, c := range chats {
		limb := common.Limb{
			Type:   event.Vendor.Type,
			UID:    event.Vendor.UID,
			ChatID: c.ID,
		}.String()
		chat := &manager.Chat{
			Limb:     limb,
			ChatType: c.Type,
			Title:    c.Title,
		}
		if err := manager.AddOrUpdateChat(chat); err != nil {
			log.Warnf("Failed to add or update chat: %v", err)
		}
	}
}

func (ms *MasterService) logMessage(chat *ChatInfo, event *common.OctopusEvent, resp *gotgbot.Message, err error) {
	if err != nil {
		log.Warnf("Failed to send to Telegram (chat %d, %d): %v", chat.id, chat.threadID, err)
	} else {
		masterLimb := common.Limb{
			Type:   "telegram",
			UID:    common.Itoa(ms.config.Master.AdminID),
			ChatID: common.Itoa(resp.Chat.Id),
		}.String()
		slaveLimb := common.Limb{
			Type:   event.Vendor.Type,
			UID:    event.Vendor.UID,
			ChatID: event.Chat.ID,
		}.String()
		msg := &manager.Message{
			MasterLimb:        masterLimb,
			MasterMsgID:       common.Itoa(resp.MessageId),
			MasterMsgThreadID: common.Itoa(resp.MessageThreadId),
			SlaveLimb:         slaveLimb,
			SlaveMsgID:        event.ID,
			SlaveSender:       event.From.ID,
			Content:           event.Content,
			Timestamp:         event.Timestamp,
		}
		if err := manager.AddMessage(msg); err != nil {
			log.Warnf("Failed to add message %+v: %v", msg, err)
		} else {
			log.Debugf("Add message: %+v", msg)
		}
	}
}

func (ms *MasterService) replayLinkIssue(msg *gotgbot.Message, text string) error {
	_, err := msg.Reply(ms.bot, text, &gotgbot.SendMessageOpts{
		ParseMode:       "Markdown",
		MessageThreadId: msg.MessageThreadId,
	})
	return err
}

func (ms *MasterService) createForumChatInfo(chatID int64, event *common.OctopusEvent) *ChatInfo {
	masterLimb := common.Limb{
		Type:   "telegram",
		UID:    common.Itoa(ms.config.Master.AdminID),
		ChatID: common.Itoa(chatID),
	}.String()
	slaveLimb := common.Limb{
		Type:   event.Vendor.Type,
		UID:    event.Vendor.UID,
		ChatID: event.Chat.ID,
	}.String()

	topic := ms.getOrCreateTopic(chatID, event.Chat.Title, masterLimb, slaveLimb)
	if topic == nil {
		var title string
		if event.Chat.Type == "private" {
			title = fmt.Sprintf("👤 %s:", displayName(&event.From))
		} else {
			title = fmt.Sprintf("👥 %s [%s]:", displayName(&event.From), event.Chat.Title)
		}
		return &ChatInfo{
			id:    chatID,
			title: title,
		}
	} else {
		topicID, _ := common.Atoi(topic.TopicID)
		return &ChatInfo{
			id:       chatID,
			threadID: topicID,
			title:    fmt.Sprintf("%s:", displayName(&event.From)),
		}
	}
}

func (ms *MasterService) getOrCreateTopic(chatID int64, title, masterLimb, slaveLimb string) *manager.Topic {
	topic, err := manager.GetTopic(masterLimb, slaveLimb)
	if err != nil {
		log.Warnf("Failed to get topic: %v", err)
	} else if topic == nil {
		resp, err := ms.bot.CreateForumTopic(chatID, title, &gotgbot.CreateForumTopicOpts{})
		if err != nil {
			log.Warnf("Failed to create topic: %v", err)
		} else {
			topic = &manager.Topic{
				MasterLimb: masterLimb,
				SlaveLimb:  slaveLimb,
				TopicID:    common.Itoa(resp.MessageThreadId),
			}
			if err := manager.AddTopic(topic); err != nil {
				log.Warnf("Failed to add topic: %v", err)
			}
		}
	}
	return topic
}

func (ms *MasterService) download(fileID string) (*common.BlobData, error) {
	if file, err := ms.bot.GetFile(fileID, &gotgbot.GetFileOpts{}); err != nil {
		return nil, err
	} else {
		var data []byte

		if ms.config.Master.LocalMode {
			data, err = os.ReadFile(file.FilePath)
			if err != nil {
				return nil, err
			}
		} else {
			response, err := ms.client.Get(file.GetURL(ms.bot))
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			data, err = io.ReadAll(response.Body)
			if err != nil {
				return nil, err
			}

		}

		mime := mimetype.Detect(data)
		return &common.BlobData{
			Name:   fmt.Sprintf("%s%s", file.FileUniqueId, mime.Extension()),
			Mime:   mime.String(),
			Binary: data,
		}, nil
	}
}

func displayName(user *common.User) string {
	if len(user.Remark) > 0 {
		return user.Remark
	}
	return user.Username
}

func isSendAsFile(data []byte) bool {
	image, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		var maxSize int
		var minSize int
		if image.Height > image.Width {
			maxSize = image.Height
			minSize = image.Width
		} else {
			maxSize = image.Width
			minSize = image.Height
		}
		imgRatio := float32(maxSize) / float32(minSize)

		if minSize > imgMinSize {
			return true
		}
		if maxSize > imgMaxSize && imgRatio > imgSizeRatio {
			return true
		}
		if imgRatio >= imgSizeMaxRatio {
			return true
		}
	} else {
		log.Warnf("Deocde image(%s) failed: %v", mimetype.Detect(data), err)
	}

	return false
}
