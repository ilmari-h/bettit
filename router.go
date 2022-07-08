package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/chenyahui/gin-cache"
	"github.com/chenyahui/gin-cache/persist"
	"github.com/gin-gonic/gin"
)

type Thread = []Comment

type Comment struct {
	id      int
	title   string
	content string
	replies []Comment
}

type RouterError struct {
	code    int
	message string
}

func (err *RouterError) Error() string {
	return err.message
}

func (err *RouterError) Code() int {
	return err.code
}

var archivePostCache map[string]int64 // Post ID to timestamp

type RouterOptions struct {
	GetCacheTime       int
	GetCacheExpiration int
	PostCacheTime      int
}

var routerOptions RouterOptions

func getThread(req *http.Request) ([]byte, *RouterError) {

	client := http.Client{
		Timeout: time.Second * time.Duration(clientOptions.Timeout),
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, &RouterError{code: http.StatusBadRequest, message: err.Error()}
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	if res.StatusCode != 200 {
		Log(
			fmt.Sprintf("Bad response to request at: %s", req.URL.Path),
			fmt.Sprintf("Status: %s", res.Status),
		).Error()
		return nil, &RouterError{code: res.StatusCode, message: "Recieved unsuccessful response from Reddit API."}
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, &RouterError{code: http.StatusBadRequest, message: err.Error()}
	}

	return body, nil
}

func routeGetIndex(c *gin.Context) {
	if status := RenderIndexPage(c.Writer); status != 200 {
		RenderErrorPage(status, c.Writer)
	}
}
func routeGetAbout(c *gin.Context) {
	if status := RenderAboutPage(c.Writer); status != 200 {
		RenderErrorPage(status, c.Writer)
	}
}

func NewThreadRequest(sub string, threadId string, commentId string) (*http.Request, error) {

	requestUrl := fmt.Sprintf("https://oauth.reddit.com/r/%s/comments/%s?sort=confidence.json", sub, threadId)
	if commentId != "" {
		requestUrl = fmt.Sprintf("https://oauth.reddit.com/r/%s/comments/%s/comment/%s?sort=confidence.json", sub, threadId, commentId)
	}
	req, err := http.NewRequest(http.MethodGet, requestUrl, nil)
	req.Header.Set("User-Agent", "Bettit-API/0.1, Archives for Reddit Threads")
	req.Header.Set("Authorization", "bearer "+apiToken)

	// Validate request correctness.
	if req.Host != "oauth.reddit.com" {
		Log("Incorrect API request attempt", requestUrl).Error()
		return nil, &RouterError{code: http.StatusBadRequest, message: ""}
	}

	if err != nil {
		Log("Error forming API request.", err.Error()).Error()
		return nil, &RouterError{code: http.StatusBadRequest, message: ""}
	}
	return req, nil
}

func readThreadUrl(turl *url.URL) (struct {
	Sub string
	Id  string
}, bool) {
	urlParts := strings.Split(turl.Path, "/")
	if len(urlParts) < 5 {
		return struct {
			Sub string
			Id  string
		}{"", ""}, true
	}
	idIsValid :=
		regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(urlParts[4]) && // Alphanumeric ID
			len(urlParts[4]) <= 6 && // Correct length
			urlParts[1] == "r" && // check format
			urlParts[3] == "comments" // check format

	return struct {
		Sub string
		Id  string
	}{urlParts[2], urlParts[4]}, !idIsValid
}

func routePostArchive(c *gin.Context) {
	input := c.PostForm("archivef")

	// Parse an API request based on input
	url, _ := url.Parse(input)
	urlParts, urlErr := readThreadUrl(url)

	if urlErr {
		RenderErrorPage(400, c.Writer)
		return
	}

	if ts, exists := archivePostCache[urlParts.Id]; exists && int64(routerOptions.PostCacheTime) > time.Now().Unix()-ts {
		RenderAlreadyExists(urlParts.Id, c.Writer)
		return
	}

	req, err := NewThreadRequest(urlParts.Sub, urlParts.Id, "")
	if err != nil {
		RenderErrorPage(400, c.Writer)
		return
	}

	threadBytes, tErr := getThread(req)
	if tErr != nil {
		RenderErrorPage(tErr.code, c.Writer)
		return
	}

	if dbError := archiveThread(urlParts.Sub, threadBytes); dbError != nil {
		RenderAlreadyExists(urlParts.Id, c.Writer)
	} else {
		archivePostCache[urlParts.Id] = time.Now().Unix()
		RenderRedirectPage(urlParts.Id, c.Writer)
	}
}

func routeGetPage(c *gin.Context) {
	threadId := c.Param("threadid")
	if status := RenderThreadPage(threadId, c.Writer); status != 200 {
		RenderErrorPage(status, c.Writer)
	}
}

func GettitRouter(opts RouterOptions) *gin.Engine {

	routerOptions = opts
	archivePostCache = make(map[string]int64)
	memCache := persist.NewMemoryStore(time.Second * time.Duration(routerOptions.GetCacheExpiration))
	getCacheTime := time.Second * time.Duration(routerOptions.GetCacheTime)

	r := gin.Default()
	r.GET("/", cache.CacheByRequestURI(memCache, getCacheTime), routeGetIndex)
	r.GET("/about", cache.CacheByRequestURI(memCache, getCacheTime), routeGetAbout)
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "API is live.")
	})
	r.GET("/:threadid", cache.CacheByRequestURI(memCache, getCacheTime), routeGetPage)
	r.POST("/archive", routePostArchive)
	return r
}
