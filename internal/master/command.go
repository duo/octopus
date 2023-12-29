package master

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/duo/octopus/internal/common"
	"github.com/duo/octopus/internal/manager"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	log "github.com/sirupsen/logrus"
)

const (
	maxShowBindedLinks = 7
)

func onCommand(bot *gotgbot.Bot, ctx *ext.Context, config *common.Configure) error {
	text := ctx.EffectiveMessage.Text
	if strings.HasPrefix(text, "/help") {
		_, err := bot.SendMessage(
			ctx.EffectiveChat.Id,
			"help - Show command list.\nlink - Manage remote chat link.\nchat - Generate a remote chat head.",
			nil,
		)
		return err
	} else if strings.HasPrefix(text, "/link") {
		if ctx.EffectiveChat.IsForum && ctx.EffectiveMessage.MessageThreadId != 0 {
			_, err := bot.SendMessage(
				ctx.EffectiveChat.Id,
				"Link in topic not support.",
				&gotgbot.SendMessageOpts{
					MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
				},
			)
			return err
		} else if ctx.EffectiveChat.Type == "private" {
			_, err := bot.SendMessage(ctx.EffectiveChat.Id, "Link in private chat does not support.", nil)
			return err
		}

		cb := Callback{
			Category: "link",
			Acction:  "list",
		}
		parts := strings.Split(text, " ")
		if len(parts) == 2 {
			cb.Query = parts[1]
		}

		return handleLink(bot, ctx, config, ctx.Message.From.Id, cb)
	} else if strings.HasPrefix(text, "/chat") {
		cb := Callback{
			Category: "chat",
			Acction:  "list",
		}
		parts := strings.Split(text, " ")
		if len(parts) == 2 {
			cb.Query = parts[1]
		}

		return handleChat(bot, ctx, config, ctx.Message.From.Id, cb)
	} else {
		_, err := bot.SendMessage(
			ctx.EffectiveChat.Id,
			"Command not support.",
			&gotgbot.SendMessageOpts{
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
		return err
	}
}

func handleLink(bot *gotgbot.Bot, ctx *ext.Context, config *common.Configure, userID int64, cb Callback) error {
	if cb.Acction == "close" {
		_, _, err := ctx.EffectiveMessage.EditText(
			bot,
			"_Canceled by user._",
			&gotgbot.EditMessageTextOpts{ParseMode: "Markdown"},
		)
		return err
	} else if cb.Acction == "bind" {
		masterLimb := common.Limb{
			Type:   "telegram",
			UID:    common.Itoa(userID),
			ChatID: common.Itoa(ctx.EffectiveChat.Id),
		}.String()

		if err := manager.AddLink(&manager.Link{
			MasterLimb: masterLimb,
			SlaveLimb:  cb.Data,
		}); err != nil {
			log.Warnf("Add link failed: %v", err)
		}
	} else if cb.Acction == "unbind" {
		if id, err := common.Atoi(cb.Data); err != nil {
			log.Warnf("Parse callback data failed: %v", err)
		} else if err := manager.DelLinkById(id); err != nil {
			log.Warnf("Delete link failed: %v", err)
		}
	}

	return showLinks(bot, ctx, config, userID, cb)
}

func handleChat(bot *gotgbot.Bot, ctx *ext.Context, config *common.Configure, userID int64, cb Callback) error {
	if cb.Acction == "close" {
		_, _, err := ctx.EffectiveMessage.EditText(
			bot,
			"_Canceled by user._",
			&gotgbot.EditMessageTextOpts{ParseMode: "Markdown"},
		)
		return err
	} else if cb.Acction == "talk" {
		chat, err := manager.GetChat(cb.Data)
		if err != nil {
			log.Warnf("Get chat failed: %v", err)
			return err
		}

		masterLimb := common.Limb{
			Type:   "telegram",
			UID:    common.Itoa(userID),
			ChatID: common.Itoa(ctx.EffectiveChat.Id),
		}.String()

		if err := manager.AddMessage(&manager.Message{
			MasterLimb:        masterLimb,
			MasterMsgID:       common.Itoa(ctx.EffectiveMessage.MessageId),
			MasterMsgThreadID: common.Itoa(ctx.EffectiveMessage.MessageThreadId),
			SlaveLimb:         chat.Limb,
			SlaveMsgID:        "0",
		}); err != nil {
			log.Warnf("Add message failed: %v", err)
			return err
		}

		_, _, err = ctx.EffectiveMessage.EditText(
			bot,
			fmt.Sprintf(
				"*Reply this message to talk with %s*",
				common.EscapeText("Markdown", chat.Title),
			),
			&gotgbot.EditMessageTextOpts{ParseMode: "Markdown"},
		)
		return err
	}

	return showChats(bot, ctx, config, cb)
}

func showLinks(bot *gotgbot.Bot, ctx *ext.Context, config *common.Configure, userID int64, cb Callback) error {
	masterLimb := common.Limb{
		Type:   "telegram",
		UID:    common.Itoa(userID),
		ChatID: common.Itoa(ctx.EffectiveChat.Id),
	}.String()

	count, err := manager.GetChatCount(cb.Query)
	if err != nil {
		log.Warnf("Get chat cout failed: %v", err)
		return err
	}

	pager := manager.CalcPager(cb.Page, config.Master.PageSize, count)

	links, err := manager.GetLinkList()
	if err != nil {
		log.Warnf("Get link list failed: %v", err)
		return err
	}

	chats, err := manager.GetChatList(pager.CurrentPage, config.Master.PageSize, cb.Query)
	if err != nil {
		log.Warnf("Get chat list failed: %v", err)
		return err
	}

	if len(chats) == 0 {
		_, err := bot.SendMessage(
			ctx.EffectiveChat.Id,
			"No chat currently avaiable.",
			&gotgbot.SendMessageOpts{
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
		return err
	}

	text := "Links:"

	bindLinks, err := manager.GetLinksByMaster(masterLimb)
	if err != nil {
		log.Warnf("Get links by master failed: %v", err)
		return err
	}
	for idx, l := range bindLinks {
		if idx >= maxShowBindedLinks {
			break
		}
		limb, _ := common.LimbFromString(l.SlaveLimb)
		text += fmt.Sprintf("\n🔗%s(%s) from (%s %s)", l.Title, limb.ChatID, limb.Type, limb.UID)
	}
	if len(bindLinks) > maxShowBindedLinks {
		text += fmt.Sprintf("\n\nand %d more...", len(bindLinks)-maxShowBindedLinks)
	}

	keyboard := [][]gotgbot.InlineKeyboardButton{}
	for _, chat := range chats {
		limb, _ := common.LimbFromString(chat.Limb)
		info := fmt.Sprintf("%s(%s) from (%s %s)", chat.Title, limb.ChatID, limb.Type, limb.UID)
		if chat.ChatType == "private" {
			info = "👤" + info
		} else {
			info = "👥" + info
		}

		cb := Callback{
			Category: "link",

			Query: cb.Query,
			Page:  pager.CurrentPage,
		}

		idx := slices.IndexFunc(links, func(l *manager.Link) bool {
			return l.MasterLimb == masterLimb && l.SlaveLimb == chat.Limb
		})

		if idx == -1 {
			cb.Acction = "bind"
			cb.Data = chat.Limb
		} else {
			info = "🔗" + info

			cb.Acction = "unbind"
			cb.Data = common.Itoa(links[idx].ID)
		}

		btn := gotgbot.InlineKeyboardButton{Text: info, CallbackData: putCallback(cb)}
		keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{btn})
	}

	var bottom = []gotgbot.InlineKeyboardButton{}
	if pager.HasPrev {
		cb := Callback{
			Category: "link",
			Acction:  "list",
			Query:    cb.Query,
			Page:     pager.PrevPage,
		}
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: "< Prev", CallbackData: putCallback(cb)})
	} else {
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: " ", CallbackData: "0"})
	}
	{
		info := fmt.Sprintf("%d / %d (%d) | Cancel",
			pager.CurrentPage, pager.NumPages, pager.NumItems)
		cb := Callback{
			Category: "link",
			Acction:  "close",
		}
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: info, CallbackData: putCallback(cb)})
	}
	if pager.HasNext {
		cb := Callback{
			Category: "link",
			Acction:  "list",
			Query:    cb.Query,
			Page:     pager.NextPage,
		}
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: "Next >", CallbackData: putCallback(cb)})
	} else {
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: " ", CallbackData: "0"})
	}
	keyboard = append(keyboard, bottom)

	if ctx.EffectiveMessage.From.Id == bot.User.Id {
		_, _, err := ctx.EffectiveMessage.EditText(
			bot,
			text,
			&gotgbot.EditMessageTextOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: keyboard,
				},
			},
		)
		return err
	} else {
		_, err := bot.SendMessage(
			ctx.EffectiveChat.Id,
			text,
			&gotgbot.SendMessageOpts{
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: keyboard,
				},
			},
		)
		return err
	}
}

func showChats(bot *gotgbot.Bot, ctx *ext.Context, config *common.Configure, cb Callback) error {
	count, err := manager.GetChatCount(cb.Query)
	if err != nil {
		log.Warnf("Get chat cout failed: %v", err)
		return err
	}

	pager := manager.CalcPager(cb.Page, config.Master.PageSize, count)

	chats, err := manager.GetChatList(pager.CurrentPage, config.Master.PageSize, cb.Query)
	if err != nil {
		log.Warnf("Get chat list failed: %v", err)
		return err
	}

	if len(chats) == 0 {
		_, err := bot.SendMessage(
			ctx.EffectiveChat.Id,
			"No chat currently avaiable.",
			&gotgbot.SendMessageOpts{
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
			},
		)
		return err
	}

	keyboard := [][]gotgbot.InlineKeyboardButton{}
	for _, chat := range chats {
		limb, _ := common.LimbFromString(chat.Limb)
		info := fmt.Sprintf("%s(%s) from (%s %s)", chat.Title, limb.ChatID, limb.Type, limb.UID)
		if chat.ChatType == "private" {
			info = "👤" + info
		} else {
			info = "👥" + info
		}

		cb := Callback{
			Category: "chat",
			Acction:  "talk",
			Data:     chat.Limb,
			Query:    cb.Query,
			Page:     pager.CurrentPage,
		}

		btn := gotgbot.InlineKeyboardButton{Text: info, CallbackData: putCallback(cb)}
		keyboard = append(keyboard, []gotgbot.InlineKeyboardButton{btn})
	}

	var bottom = []gotgbot.InlineKeyboardButton{}
	if pager.HasPrev {
		cb := Callback{
			Category: "chat",
			Acction:  "list",
			Query:    cb.Query,
			Page:     pager.PrevPage,
		}
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: "< Prev", CallbackData: putCallback(cb)})
	} else {
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: " ", CallbackData: "0"})
	}
	{
		info := fmt.Sprintf("%d / %d (%d) | Cancel",
			pager.CurrentPage, pager.NumPages, pager.NumItems)
		cb := Callback{
			Category: "chat",
			Acction:  "close",
		}
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: info, CallbackData: putCallback(cb)})
	}
	if pager.HasNext {
		cb := Callback{
			Category: "chat",
			Acction:  "list",
			Query:    cb.Query,
			Page:     pager.NextPage,
		}
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: "Next >", CallbackData: putCallback(cb)})
	} else {
		bottom = append(bottom, gotgbot.InlineKeyboardButton{Text: " ", CallbackData: "0"})
	}
	keyboard = append(keyboard, bottom)

	if ctx.EffectiveMessage.From.Id == bot.User.Id {
		_, _, err := ctx.EffectiveMessage.EditReplyMarkup(
			bot,
			&gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: keyboard,
				},
			},
		)
		return err
	} else {
		_, err := bot.SendMessage(
			ctx.EffectiveChat.Id,
			"Please choose a chat you'd like to talk.",
			&gotgbot.SendMessageOpts{
				MessageThreadId: ctx.EffectiveMessage.MessageThreadId,
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: keyboard,
				},
			},
		)
		return err
	}
}
