#!/bin/sh
rm -f bettit.db && rm -rf ./archive/* && source ./.env && go build && ./bettit
