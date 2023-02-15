# Octopus
A Telegram bot bridge other IM (qq, wechat, etc.) conversations together.

## Dependencies
* go
* ffmpeg (optional for qq/wechat audio)

# Docker
* [octopus](https://hub.docker.com/r/lxduo/octopus)
```shell
docker run -d -p 11111:11111 --name=octopus --restart=always -v octopus:/data lxduo/octopus:latest
```

# Limbs
* [octopus-qq](https://github.com/duo/octopus-qq)
* [octopus-wechat](https://github.com/duo/octopus-wechat)
* [octopus-wechat-web](https://github.com/duo/octopus-wechat-web)

# Documentation

## Bot
Create a bot with [@BotFather](https://t.me/botfather), get a token.
Set /setjoingroups Enable and /setprivacy Disable

## Configuration
* configure.yaml
```yaml
master:
  api_url: http://10.0.0.10:8081 # Optional, Telegram local bot api server
  local_mode: true # Optional, local server mode
  admin_id: # Required, Telegram user id (administrator)
  token:  1234567:xxxxxxxx # Required, Telegram bot token
  proxy: http://1.1.1.1:7890 # Optional, proxy for Telegram
  archive: # Optional, archive client chat by topic
    - vendor: wechat # qq, wechat, etc
      uid: wxid_xxxxxxx # client id
      chat_id: 123456789 # topic enabled group id (grant related permissions to bot)
  telegraph: # Optional
    enable: true # Convert some message to telegra.ph article (e.g. QQ forward message)
  	proxy: http://1.1.1.1:7890 # Optional, proxy for telegra.ph
    tokens:
      - abcdefg # telegra.ph tokens

service:
  addr: 0.0.0.0:11111 # Required, listen address
  secret: hello # Required, user defined secret
  send_timeout: 3m # Optional

log:
  level: info
```

## Command
All messages will be sent to the admin directly by default, you can archive chat by topic or /link specific remote chat to a Telegram group.
```
/help Show command list.
/link Manage remote chat link.
/chat Generate a remote chat head.
```
