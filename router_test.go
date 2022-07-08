package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

var routerOptsT = RouterOptions{
	GetCacheTime:       1,
	GetCacheExpiration: 1,
	PostCacheTime:      1,
}
var router *gin.Engine = GettitRouter(routerOptsT)

func ReloadTestEnv() {
	router = GettitRouter(routerOptsT)
	InitAPI()
	InitDatabase()
	LoadTemplates()
}

func TestHealthGET(t *testing.T) {
	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resData, _ := ioutil.ReadAll(w.Body)
	assert.Contains(t, string(resData), "API is live")
	assert.Equal(t, 200, w.Code)
}

func TestArchivePOST(t *testing.T) {

	ReloadTestEnv()

	form := url.Values{}
	form.Add("archivef", "https://www.reddit.com/r/test/comments/agi5zf/test/")
	req, _ := http.NewRequest("POST", "/archive", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, 200)
}

func TestArchivePOSTErr(t *testing.T) {

	ReloadTestEnv()

	form := url.Values{}
	form.Add("archivef", "https://www.reddit.com/r/test/")
	req, _ := http.NewRequest("POST", "/archive", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, 400)
}
