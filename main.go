package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/pborman/getopt/v2"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var apiToken string = ""

const DBFILE = "./bettit.db.d/bettit.db"

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

func FetchAPIToken() string {
	if token := os.Getenv("REDDIT_API_ACCESS_TOKEN"); token != "" {
		log.Info("Using pre-fetched API token.")
		return token
	}
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
		token := gjson.GetBytes(body, "access_token").String()
		if token != "" {
			log.Info("Successfully fetched new API token")
			return token
		} else {
			Log("Error fetching API token", string(body)).Fatal()
		}
	}
	Log("Error fetching API token", "Unknown error").Fatal()
	return ""
}

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
}

func main() {

	nRouterOpts := RouterOptions{}
	clientOptions.Timeout = *getopt.IntLong("client-timeout", 'c', 5, "Timeout for requests made to Reddit API.")
	nRouterOpts.GetCacheTime = *getopt.IntLong("get-cache-time", 'g', 60, "Time in seconds for caching GET-requests.")
	nRouterOpts.GetCacheExpiration = *getopt.IntLong("get-cache-exp", 'e', 300, "Expiry time in seconds for GET-requests.")
	nRouterOpts.PostRateLimitN = *getopt.IntLong("post-rate-limit-numerator", 'r', 5,
		`Numerator of the post rate limit.
The default denominator being 60 seconds, that means default rate is 5 per minute.`,
	)
	nRouterOpts.PostRateLimitD = *getopt.IntLong("post-rate-limit-denominator", 'd', 60,
		`Denominator of the post rate limit.
By default 60, the default numerator being 5, that means default rate is 5 per minute.`,
	)
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

REDDIT_API_ACCESS_TOKEN
	Access token for your script application from: https://www.reddit.com/api/v1/access_token
	If this is defined, the following environment variables are not required.
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

	apiToken = FetchAPIToken()
	InitDatabase()
	LoadTemplates()
	r := GettitRouter(nRouterOpts)
	r.Static("/res", "./public")
	r.Run()
}
