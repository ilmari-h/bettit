package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
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

func getThread(req *http.Request, c *gin.Context) ([]byte, *RouterError) {

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

func routeArchive(c *gin.Context) {
	subreddit := c.Query("sub")
	id := c.Query("id")
	thread := c.Query("thread")

	requestUrl := fmt.Sprintf("https://oauth.reddit.com/r/%s/comments/%s/%s.json", subreddit, id, thread)
	req, err := http.NewRequest(http.MethodGet, requestUrl, nil)

	req.Header.Set("User-Agent", "Bettit-API/0.1, Archives for Reddit Threads")
	req.Header.Set("Authorization", "bearer "+apiToken)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	threadBytes, tErr := getThread(req, c)
	if tErr != nil {
		c.JSON(tErr.Code(), gin.H{
			"message": tErr.Error(),
		})
		return
	}

	if dbError := txPostThread(subreddit, threadBytes); dbError != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": dbError.Error()})
	} else {
		redirectPage := templates.Lookup("redirect.tmpl").Lookup("redirect")
		redirectPage.Execute(c.Writer, &RedirectTmpl{"http://localhost:8080/" + id})
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
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "API is live.")
	})
	r.POST("/archive", routeArchive)
	r.GET("/:threadid", routePage)
	return r
}
