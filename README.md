# TI-Match

A Discord bot that sends a message to the #general channel of each guild it is connected to 
when a match in [The International 2017](http://www.dota2.com/international/overview/) is about to begin.

Run the bot via Docker, supplying it a 
[Discord bot token](https://discordapp.com/developers/applications/me) and a 
[Steam web API key](https://steamcommunity.com/dev/apikey):

```
docker build . -t vearth/owbot-bot
docker run -d vearth/owbot-bot -discordtoken "DISCORD_BOT_TOKEN" -steamkey "STEAM_API_KEY"
```

Add the bot to a guild by visiting the following url, replacing CLIENT_ID with the
client id of the discord application. This will grant the bot the SEND_MESSAGES
and SEND_TTS_MESSAGES permissions requried.

```
https://discordapp.com/oauth2/authorize?scope=bot&permissions=6144&client_id=CLIENT_ID
```
