package main

import (
	"fmt"
	"io/ioutil"
	"log"
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
		log.Printf(fmt.Sprintf("Bad response to request at: %s. Status: %s", req.URL.Path, res.Status))
		return nil, &RouterError{code: res.StatusCode, message: "Recieved unsuccessful response from Reddit API."}
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, &RouterError{code: http.StatusBadRequest, message: err.Error()}
	}

	return body, nil
}

func GettitRouter() *gin.Engine {

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "API is live.")
	})
	r.POST("/archive", func(c *gin.Context) {

		subreddit := c.Query("sub")
		id := c.Query("id")
		thread := c.Query("thread")

		requestUrl := fmt.Sprintf("https://reddit.com/r/%s/comments/%s/%s.json", subreddit, id, thread)
		req, err := http.NewRequest(http.MethodGet, requestUrl, nil)

		req.Header.Set("User-Agent", "bettit-archive")

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
			log.Printf("TRYING TO LOG ERROR %v", dbError)
			c.JSON(http.StatusBadRequest, gin.H{"message": dbError.Error()})
		} else {
			redirectPage := templates.Lookup("redirect.tmpl").Lookup("redirect")
			redirectPage.Execute(c.Writer, &RedirectTmpl{"http://localhost:8080/" + id})
		}
	})
	return r
}
