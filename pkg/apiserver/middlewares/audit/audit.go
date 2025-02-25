/*
Copyright 2021 KubeCube Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package audit

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"

	"github.com/kubecube-io/kubecube/pkg/authentication/authenticators/token"
	"github.com/kubecube-io/kubecube/pkg/clog"
	"github.com/kubecube-io/kubecube/pkg/utils/constants"
	"github.com/kubecube-io/kubecube/pkg/utils/env"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

const (
	eventActionCreate = "create"
	eventActionUpdate = "update"
	eventActionDelete = "delete"
	eventActionQuery  = "query"

	eventRespBody         = "responseBody"
	eventResourceKubeCube = "[KubeCube]"
)

var (
	whiteList = map[string]string{
		constants.ApiPathRoot + "/audit": "POST",
	}
	auditSvc env.AuditSvcApi
)

func init() {
	auditSvc = env.AuditSVC()
}

func withinWhiteList(url *url.URL, method string, whiteList map[string]string) bool {
	queryUrl := url.Path
	if _, ok := whiteList[queryUrl]; ok {
		if whiteList[queryUrl] == method {
			return true
		}
		return false
	}
	return false
}

func Audit() gin.HandlerFunc {
	return func(c *gin.Context) {

		w := &responseBodyWriter{body: &bytes.Buffer{}, ResponseWriter: c.Writer}
		c.Writer = w
		c.Next()
		if !withinWhiteList(c.Request.URL, c.Request.Method, whiteList) {
			clog.Debug("[audit] get event information")
			e := &Event{
				EventTime:         time.Now().Unix(),
				EventVersion:      "V1",
				SourceIpAddress:   c.ClientIP(),
				RequestMethod:     c.Request.Method,
				ResponseStatus:    c.Writer.Status(),
				Url:               c.Request.URL.String(),
				UserIdentity:      getUserIdentity(c),
				UserAgent:         "HTTP",
				RequestParameters: getParameters(c),
				EventType:         constants.EventTypeUserWrite,
				RequestId:         uuid.New().String(),
				ResponseElements:  w.body.String(),
				EventSource:       env.AuditEventSource(),
			}

			// get response
			resp, isExist := c.Get(eventRespBody)
			if isExist == true {
				e.ResponseElements = resp.(string)
			}

			// if request failed, set errCode and ErrorMessage
			if e.ResponseStatus != http.StatusOK {
				e.ErrorCode = strconv.Itoa(c.Writer.Status())
				e.ErrorMessage = e.ResponseElements
			}

			// get event name and description
			eventName, isExist := c.Get(constants.EventName)
			if isExist == true {
				e.EventName = eventName.(string)
				e.Description = eventName.(string)
			} else {
				e = getEventName(c, *e)
			}

			// get resource type
			resourceType, isExists := c.Get(constants.EventResourceType)
			if isExists == true {
				e.ResourceReports = []Resource{{ResourceType: resourceType.(string)}}
			} else {
				if e.ResponseElements != "" {

				}
			}

			go sendEvent(e)
		}
	}

}

func sendEvent(e *Event) {
	clog.Debug("[audit] send event to audit service")
	jsonstr, err := json.Marshal(e)
	if err != nil {
		clog.Error("[audit] json marshal event error: %v", err)
		return
	}
	buffer := bytes.NewBuffer(jsonstr)
	request, err := http.NewRequest(auditSvc.Method, auditSvc.URL, buffer)
	if err != nil {
		clog.Error("[audit] create http request error: %v", err)
		return
	}
	headers := strings.Split(auditSvc.Header, ";")
	for _, header := range headers {
		kv := strings.Split(header, "=")
		if len(kv) != 2 {
			continue
		}
		request.Header.Set(kv[0], kv[1])
	}
	client := http.Client{}
	resp, err := client.Do(request.WithContext(context.TODO()))
	if err != nil {
		clog.Error("[audit] client.Do error: %s", err)
		return
	}
	defer resp.Body.Close()

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		clog.Error("[audit] read response body error: %s", err)
		return
	}
	clog.Debug("[audit] response is %s", string(respBytes))
	return
}

// get event name and description
func getEventName(c *gin.Context, e Event) *Event {
	var (
		methodStr string
		object    string
	)

	url := c.Request.RequestURI
	method := c.Request.Method

	switch method {
	case http.MethodPost:
		methodStr = eventActionCreate
		break
	case http.MethodPut:
		methodStr = eventActionUpdate
		break
	case http.MethodDelete:
		methodStr = eventActionDelete
		break
	case http.MethodGet:
		methodStr = eventActionQuery
	}

	queryUrl := strings.Split(fmt.Sprint(url), "?")[0]
	urlstrs := strings.Split(queryUrl, "/")
	for i, str := range urlstrs {
		if str == constants.K8sResourceNamespace {
			if i+2 < len(urlstrs) {
				object = urlstrs[i+2]
			} else {
				object = constants.K8sResourceNamespace
			}
			break
		}

		if str == constants.K8sResourceVersion && urlstrs[i+1] != constants.K8sResourceNamespace {
			object = urlstrs[i+1]
		}
	}

	e.EventName = eventResourceKubeCube + " " + methodStr + " " + object
	e.Description = e.EventName
	return &e
}

// get user name from token
func getUserIdentity(c *gin.Context) *UserIdentity {

	userIdentity := &UserIdentity{}
	accountId, isExist := c.Get(constants.EventAccountId)
	if isExist {
		userIdentity.AccountId = accountId.(string)
		return userIdentity
	}

	userInfo, err := token.GetUserFromReq(c.Request)
	if err != nil {
		return nil
	}
	userIdentity.AccountId = userInfo.Username
	return userIdentity
}

// get request params, includes header、body and queryString
func getParameters(c *gin.Context) string {
	var parameters parameters

	parameters.Headers = c.Request.Header.Clone()
	parameters.Body = getBodyFromReq(c)

	query := make(map[string]string)
	params := c.Params
	for _, param := range params {
		query[param.Key] = param.Value
	}
	parameters.Query = query
	paramJson, err := json.Marshal(parameters)
	if err != nil {
		clog.Error("marshal param error: %s", err)
		return ""
	}
	return string(paramJson)

}

type parameters struct {
	Query   map[string]string   `json:"querystring"`
	Body    string              `json:"body"`
	Headers map[string][]string `json:"header"`
}

func getBodyFromReq(c *gin.Context) string {
	switch c.Request.Method {
	case http.MethodPatch:
	case http.MethodPut:
	case http.MethodPost:
		data, _ := ioutil.ReadAll(c.Request.Body)
		return string(data)
	}
	return ""
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r responseBodyWriter) WriteString(s string) (n int, err error) {
	r.body.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}
