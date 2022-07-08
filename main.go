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
	"github.com/pborman/getopt/v2"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var apiToken string = ""

type ClientOptions struct {
	Timeout int
}

var clientOptions ClientOptions

func Log(message string, detail string) *log.Entry {
	return log.WithFields(log.Fields{
		"message": message,
		"detail":  detail,
	})
}

func LogE(e *DbError) error {
	log.WithFields(log.Fields{
		"message": e.message,
		"detail":  e.details,
	}).Error()
	return e
}

func InitAPI() {
	username := os.Getenv("REDDIT_APP_DEV_NAME")
	password := os.Getenv("REDDIT_APP_DEV_PW")
	id := os.Getenv("REDDIT_APP_ID")
	secret := os.Getenv("REDDIT_APP_SECRET")

	if username == "" || password == "" || id == "" || secret == "" {
		log.Fatal("Environment variables are not defined")
	}

	client := http.Client{
		Timeout: time.Second * time.Duration(clientOptions.Timeout),
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

	nRouterOpts := RouterOptions{}
	clientOptions.Timeout = *getopt.IntLong("client-timeout", 'c', 5, "Timeout for requests made to Reddit API.")
	nRouterOpts.GetCacheTime = *getopt.IntLong("get-cache-time", 'g', 60, "Time in seconds for caching GET-requests.")
	nRouterOpts.GetCacheExpiration = *getopt.IntLong("get-cache-exp", 'e', 300, "Expiry time in seconds for GET-requests.")
	nRouterOpts.PostCacheTime = *getopt.IntLong("post-cache-time", 'p', 3600,
		`Time in seconds for blocking identical POST-requests to /archive -endpoint.
An archive request initiates an request to the Reddit API.
To avoid unnecessary requests, this option is used.`,
	)

	getopt.SetUsage(func() {
		getopt.PrintUsage(os.Stderr)
		os.Stderr.WriteString(`
Make sure the following environment variables are defined to access Reddit API.
They are defined by creating a 'script' type application on: https://www.reddit.com/prefs/apps

REDDIT_APP_DEV_NAME
	Name of the user set as developer for your application on Reddit.
REDDIT_APP_DEV_PW
	Password for the above mentioned account.
REDDIT_APP_ID
	ID of the script application.
REDDIT_APP_SECRET
	Secret of the application.
`)
	})
	getopt.Parse()

	InitAPI()
	InitDatabase()
	LoadTemplates()
	r := GettitRouter(nRouterOpts)
	r.Static("/res", "./public")
	r.Run()
}
