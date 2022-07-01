#!/bin/sh
rm -f bettit.db && source ./.env && go build && ./bettit
