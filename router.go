package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

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

func getThread(req *http.Request) ([]byte, *RouterError) {

	client := http.Client{
		Timeout: time.Second * 5,
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

func routeIndex(c *gin.Context) {
	RenderIndexPage(c.Writer)
	c.Status(200)
}

func NewThreadRequest(sub string, threadId string, commentId string) (*http.Request, error) {

	requestUrl := fmt.Sprintf("https://oauth.reddit.com/r/%s/comments/%s.json", sub, threadId)
	if commentId != "" {
		requestUrl = fmt.Sprintf("https://oauth.reddit.com/r/%s/comments/%s/comment/%s.json", sub, threadId, commentId)
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

func routeArchive(c *gin.Context) {
	input := c.PostForm("archivef")

	// Parse an API request based on input
	url, _ := url.Parse(input)
	urlParts := strings.Split(url.Path, "/")

	subreddit := urlParts[2]
	id := urlParts[4]

	req, err := NewThreadRequest(subreddit, id, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{})
		return
	}

	threadBytes, tErr := getThread(req)
	if tErr != nil {
		Log("Error reading thread template", err.Error()).Error()
		c.JSON(tErr.Code(), gin.H{})
		return
	}

	if dbError := archiveThread(subreddit, threadBytes); dbError != nil {
		RenderRedirectPage(true, "http://localhost:8080/"+id, c.Writer)
	} else {
		RenderRedirectPage(false, "http://localhost:8080/"+id, c.Writer)
	}
}

func routePage(c *gin.Context) {
	threadId := c.Param("threadid")

	if rerr := RenderThreadPage(threadId, c.Writer); rerr != nil {
		c.Status(404)
	} else {
		c.Status(200)
	}
}

func GettitRouter() *gin.Engine {

	r := gin.Default()
	r.GET("/", routeIndex)
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "API is live.")
	})
	r.GET("/:threadid", routePage)
	r.POST("/archive", routeArchive)
	return r
}
