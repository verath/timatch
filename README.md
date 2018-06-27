# TI-Match

[![CircleCI](https://circleci.com/gh/verath/timatch.svg?style=svg)](https://circleci.com/gh/verath/timatch)

A Discord bot that sends a message to guilds it is connected to
when a match in a dota2 league is about to begin.

The name "TI-Match" comes from its primary use, which is to watch the
[The International](http://www.dota2.com/international/overview/) (TI for short).

Run the bot via Docker, supplying it a 
[Discord bot token](https://discordapp.com/developers/applications/me),
[Steam web API key](https://steamcommunity.com/dev/apikey) and the dota 2 league id to
watch (see e.g. http://dota2.prizetrac.kr/leagues):

```
docker build . -t verath/timatch
docker run -d verath/timatch -discordtoken "DISCORD_BOT_TOKEN" -steamkey "STEAM_API_KEY" -leagueid 5401
```

Add the bot to a guild by visiting the following url, replacing CLIENT_ID with the
client id of the discord application. This will grant the bot the SEND_MESSAGES
and SEND_TTS_MESSAGES permissions required.

```
https://discordapp.com/oauth2/authorize?scope=bot&permissions=6144&client_id=CLIENT_ID
```
