#!/bin/sh
# Script used to get the token via a shell outside the program and only then start it. Depends on: curl and jq.

res=$(curl -f -s -X POST -d "grant_type=password&username=$REDDIT_APP_DEV_NAME&password=$REDDIT_APP_DEV_PW" \
	--user-agent "Bettit Archive startup script" \
	--user "$REDDIT_APP_ID:$REDDIT_APP_SECRET" https://www.reddit.com/api/v1/access_token)

[[ -z "$res" ]] && ( echo "Error fetching API access token." ; exit 1 )

export REDDIT_API_ACCESS_TOKEN="$(echo $res | jq '.access_token')"
./bettit
