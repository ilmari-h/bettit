# Bettit
[![Go Report Badge](https://goreportcard.com/badge/github.com/ilmari-h/bettit)](https://goreportcard.com/report/github.com/ilmari-h/bettit) ![Test Run Badge](https://github.com/ilmari-h/bettit/actions/workflows/go.yml/badge.svg) 

Bettit (stands for better archives for Reddit) is an archival website for Reddit.

Reddit hosts countless threads of valuable information.
Unfortunately that information is trapped behind a heavy web bundle, with long load times and an opinion dividing user experience.
Bettit aims to solve this problem by allowing users to archive Reddit threads as plain HTML-files,
increasing the availability of information to people with slow connections or little patience.


## Installation and hosting

The website is minimal by design to make self-hosting easy with little dependencies. It's using the light-weight web framework [Gin](https://github.com/gin-gonic/gin) and sqlite as a database.

The host machine needs the following binaries to build and run the server: go, sqlite and gcc (required by [go-sqlite3](https://github.com/mattn/go-sqlite3) dependency).

`Dockerfile` and `docker-compose.yml` files are provided to deploy using docker-compose.
