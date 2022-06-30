package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var apiToken string = ""

func Log(message string, detail string) *log.Entry {
	return log.WithFields(log.Fields{
		"message": message,
		"detail":  detail,
	})
}

func auth() {
	username := os.Getenv("REDDIT_APP_DEV_NAME")
	password := os.Getenv("REDDIT_APP_DEV_PW")
	id := os.Getenv("REDDIT_APP_ID")
	secret := os.Getenv("REDDIT_APP_SECRET")

	client := http.Client{
		Timeout: time.Second * 5,
	}

	req, _ := http.NewRequest(
		http.MethodPost,
		"https://www.reddit.com/api/v1/access_token",
		bytes.NewBuffer([]byte(fmt.Sprintf("grant_type=password&username=%s&password=%s", username, password))),
	)

	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(id+":"+secret)))
	req.Header.Set("User-Agent", "Bettit-API/0.1, Archives for Reddit Threads")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err.Error())
	} else {
		defer resp.Body.Close()
		body, bodyErr := ioutil.ReadAll(resp.Body)
		if bodyErr != nil {
			Log("Error fetching API token", bodyErr.Error()).Fatal()
		}
		apiToken = gjson.GetBytes(body, "access_token").String()
		if apiToken != "" {
			log.Info("Successfully fetched new API token")
		} else {
			Log("Error fetching API token", string(body)).Fatal()
		}
	}
}

func init() {
	if gin.IsDebugging() {
		log.SetLevel(log.DebugLevel)
		log.SetOutput(os.Stdout)
	} else {
		log.SetLevel(log.InfoLevel)
		file, err := os.OpenFile("/var/log/bettit.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal("Can't open log file for writing:", err.Error())
		}
		log.SetOutput(file)
	}
}

func main() {
	auth()
	InitDatabase()
	GettitRouter().Run()
}
