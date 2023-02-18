package master

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/duo/octopus/internal/common"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"

	log "github.com/sirupsen/logrus"
)

const (
	updateTimeout  = 7
	requestTimeout = 3 * time.Minute
)

type MasterService struct {
	config *common.Configure

	in  <-chan *common.OctopusEvent
	out chan<- *common.OctopusEvent

	client  http.Client
	bot     *gotgbot.Bot
	updater *ext.Updater

	archiveChats map[string]int64

	mutex common.KeyMutex
}

func (ms *MasterService) Start() {
	ms.client = http.Client{}

	if ms.config.Master.Proxy != "" {
		proxyUrl, err := url.Parse(ms.config.Master.Proxy)
		if err != nil {
			log.Panic(err)
		}
		ms.client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	bot, err := gotgbot.NewBot(ms.config.Master.Token, &gotgbot.BotOpts{
		Client: ms.client,
		RequestOpts: &gotgbot.RequestOpts{
			Timeout: requestTimeout,
			APIURL:  ms.config.Master.APIURL,
		},
		DefaultRequestOpts: &gotgbot.RequestOpts{
			Timeout: requestTimeout,
			APIURL:  ms.config.Master.APIURL,
		},
	})
	if err != nil {
		log.Panic("failed to create new bot: " + err.Error())
	}
	ms.bot = bot

	ms.updater = ext.NewUpdater(&ext.UpdaterOpts{
		Dispatcher: ext.NewDispatcher(&ext.DispatcherOpts{
			Error: func(bot *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
				log.Infoln("an error occurred while handling update:", err.Error())
				return ext.DispatcherActionNoop
			},
			MaxRoutines: ext.DefaultMaxRoutines,
		}),
	})
	dispatcher := ms.updater.Dispatcher

	dispatcher.AddHandler(handlers.NewCallback(callbackquery.All, ms.onCallback))
	dispatcher.AddHandler(handlers.NewMessage(message.All, ms.onMessage))

	log.Infof("MasterService starting for %s", bot.User.Username)
	err = ms.updater.StartPolling(bot, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: gotgbot.GetUpdatesOpts{
			Timeout: updateTimeout,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: requestTimeout,
			},
		},
	})
	if err != nil {
		log.Panic("failed to start polling: " + err.Error())
	}

	go ms.updater.Idle()
	go ms.handleSlaveLoop()
}

func (ms *MasterService) Stop() {
	log.Infoln("MasterService stopping")
	ms.updater.Stop()
}

func NewMasterService(config *common.Configure, in <-chan *common.OctopusEvent, out chan<- *common.OctopusEvent) *MasterService {
	archiveChats := make(map[string]int64)
	for _, archive := range config.Master.Archive {
		vendor := &common.Vendor{
			Type: archive.Vendor,
			UID:  archive.UID,
		}
		archiveChats[vendor.String()] = archive.ChatID
	}

	return &MasterService{
		config:       config,
		in:           in,
		out:          out,
		archiveChats: archiveChats,
		mutex:        common.NewHashed(47),
	}
}

func (ms *MasterService) onMessage(bot *gotgbot.Bot, ctx *ext.Context) error {
	// Ignore bot's message
	if ctx.EffectiveMessage.From.IsBot {
		return nil
	}

	// Ignore strenger's message
	if ctx.EffectiveMessage.From.Id != ms.config.Master.AdminID {
		return nil
	}

	// Handle command
	if isCommand(ctx.EffectiveMessage) {
		return onCommand(bot, ctx)
	}

	return ms.processMasterMessage(ctx)
}

func (ms *MasterService) onCallback(bot *gotgbot.Bot, ctx *ext.Context) error {
	cb, ok := getCallback(ctx.Update.CallbackQuery.Data)
	if !ok {
		return errors.New("failed to look up callback data")
	}

	switch cb.Category {
	case "link":
		return handleLink(bot, ctx, ctx.Update.CallbackQuery.From.Id, cb)
	case "chat":
		return handleChat(bot, ctx, ctx.Update.CallbackQuery.From.Id, cb)
	default:
		return errors.New("invalid callback data")
	}
}

func isCommand(msg *gotgbot.Message) bool {
	if msg.Entities == nil || len(msg.Entities) == 0 {
		return false
	}

	entity := msg.Entities[0]
	return entity.Offset == 0 && entity.Type == "bot_command"
}
