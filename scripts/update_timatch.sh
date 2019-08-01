#!/bin/bash
set -eo pipefail

TI_MATCH_DISCORD_TOKEN=""
TI_MATCH_STEAM_KEY=""
TI_MATCH_LEAGUE_ID=

echo "Pulling image from docker registry"
docker pull verath/timatch

running_container_id=$(docker ps --quiet --filter="name=timatch")
if [[ -n "$running_container_id" ]]; then
    echo "Stopping running container"
    docker stop ${running_container_id}
fi

stopped_container_id=$(docker ps --quiet --all --filter="name=timatch")
if [[ -n "$stopped_container_id" ]]; then
    echo "Removing stopped container"
    docker rm ${stopped_container_id}
fi

echo "Starting container"
docker run -d --name=timatch --restart=always verath/timatch    \
        -discordtoken "$TI_MATCH_DISCORD_TOKEN"                 \
        -steamkey "$TI_MATCH_STEAM_KEY"                         \
        -leagueid "$TI_MATCH_LEAGUE_ID"

