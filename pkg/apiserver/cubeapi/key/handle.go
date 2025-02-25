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

package key

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"k8s.io/api/authentication/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	key "github.com/kubecube-io/kubecube/pkg/apis/user/v1"
	"github.com/kubecube-io/kubecube/pkg/authentication/authenticators/jwt"
	"github.com/kubecube-io/kubecube/pkg/authentication/authenticators/token"
	"github.com/kubecube-io/kubecube/pkg/clients"
	"github.com/kubecube-io/kubecube/pkg/clog"
	"github.com/kubecube-io/kubecube/pkg/utils/constants"
	"github.com/kubecube-io/kubecube/pkg/utils/errcode"
	"github.com/kubecube-io/kubecube/pkg/utils/response"
)

const (
	UserLabel = "kubecube.io/user"
)

// create ak & sk
// @Summary create key
// @Description create ak & sk keys
// @Tags key
// @Success 200 {object} map[string]string
// @Failure 500 {object} errcode.ErrorInfo
// @Router /api/v1/cube/key/create  [get]
func CreateKey(c *gin.Context) {
	c.Set(constants.EventName, "create key")
	c.Set(constants.EventResourceType, "key")
	// get user info
	userInfo, err := token.GetUserFromReq(c.Request)
	if err != nil {
		response.FailReturn(c, errcode.AuthenticateError)
		return
	}

	// max key num <= 5
	pivotClient := clients.Interface().Kubernetes(constants.PivotCluster)
	ctx := context.Background()
	keyList := key.KeyList{}
	err = pivotClient.Cache().List(ctx, &keyList, client.MatchingLabels{UserLabel: userInfo.Username})
	if err != nil {
		response.FailReturn(c, errcode.ServerErr)
		return
	}
	if len(keyList.Items) >= 5 {
		response.FailReturn(c, errcode.MaxKeyErr)
		return
	}

	// create ak & sk
	accessKey := GetUUID()
	secretKey := GetUUID()

	// create key
	keyInfo := key.Key{
		TypeMeta: metav1.TypeMeta{
			Kind:       "key",
			APIVersion: "user.kubecube.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: accessKey,
			Labels: map[string]string{
				UserLabel: userInfo.Username,
			},
		},
		Spec: key.KeySpec{
			SecretKey: secretKey,
			User:      userInfo.Username,
		},
	}
	err = pivotClient.Direct().Create(ctx, &keyInfo)
	if err != nil {
		response.FailReturn(c, errcode.ServerErr)
		return
	}

	// return
	result := map[string]string{
		"accessKey": accessKey,
		"secretKey": secretKey,
	}
	response.SuccessReturn(c, result)
}

// @Summary delete key
// @Description delete ak & sk keys
// @Tags key
// @Param	accessKey		query	string	true	"access key"
// @Success 200 {object} response.SuccessInfo
// @Failure 500 {object} errcode.ErrorInfo
// @Router /api/v1/cube/key  [delete]
func DeleteKey(c *gin.Context) {
	c.Set(constants.EventName, "delete key")
	c.Set(constants.EventResourceType, "key")

	accessKey := c.Query("accessKey")
	// get user info
	userInfo, err := token.GetUserFromReq(c.Request)
	if err != nil {
		response.FailReturn(c, errcode.AuthenticateError)
		return
	}
	// get key
	pivotClient := clients.Interface().Kubernetes(constants.PivotCluster)
	ctx := context.Background()
	keyInfo := key.Key{}
	err = pivotClient.Cache().Get(ctx, types.NamespacedName{Name: accessKey}, &keyInfo)
	if err != nil {
		if errors.IsNotFound(err) {
			response.FailReturn(c, errcode.KeyNotExistErr)
			return
		}
		response.FailReturn(c, errcode.ServerErr)
		return
	}
	if userInfo.Username != keyInfo.Spec.User {
		response.FailReturn(c, errcode.NotMatchErr)
		return
	}
	err = pivotClient.Direct().Delete(ctx, &keyInfo)
	if err != nil {
		response.FailReturn(c, errcode.ServerErr)
		return
	}

	response.SuccessReturn(c, nil)
}

// @Summary list key
// @Description query key by token
// @Tags key
// @Success 200 {object} v1.KeyList
// @Failure 500 {object} errcode.ErrorInfo
// @Router /api/v1/cube/key  [get]
func ListKey(c *gin.Context) {
	c.Set(constants.EventName, "list key")
	c.Set(constants.EventResourceType, "key")

	// get user info
	userInfo, err := token.GetUserFromReq(c.Request)
	if err != nil {
		response.FailReturn(c, errcode.AuthenticateError)
		return
	}
	// get key
	pivotClient := clients.Interface().Kubernetes(constants.PivotCluster).Cache()
	ctx := context.Background()
	keyList := key.KeyList{}
	err = pivotClient.List(ctx, &keyList, client.MatchingLabels{UserLabel: userInfo.Username})
	if err != nil {
		response.FailReturn(c, errcode.ServerErr)
		return
	}
	response.SuccessReturn(c, keyList)
}

// @Summary get token by key
// @Description query key by ak&sk
// @Tags key
// @Param	accessKey	query	string	false "access key"
// @Param	secretKey		query	string	false	"secret key"
// @Success 200 {object} map[string]string
// @Failure 500 {object} errcode.ErrorInfo
// @Router /api/v1/cube/key/token  [get]
func GetTokenByKey(c *gin.Context) {
	c.Set(constants.EventName, "get token by key")
	c.Set(constants.EventResourceType, "key")

	accessKey := c.Query("accessKey")
	secretKey := c.Query("secretKey")

	// get key
	pivotClient := clients.Interface().Kubernetes(constants.PivotCluster).Cache()
	ctx := context.Background()
	keyInfo := key.Key{}
	err := pivotClient.Get(ctx, types.NamespacedName{Name: accessKey}, &keyInfo)
	if err != nil {
		if errors.IsNotFound(err) {
			response.FailReturn(c, errcode.KeyNotExistErr)
			return
		}
		response.FailReturn(c, errcode.ServerErr)
		return
	}
	if secretKey != keyInfo.Spec.SecretKey {
		response.FailReturn(c, errcode.SecretNotMatchErr)
		return
	}

	// is user exist
	user := key.User{}
	err = pivotClient.Get(ctx, types.NamespacedName{Name: keyInfo.Spec.User}, &user)
	if err != nil {
		if errors.IsNotFound(err) {
			response.FailReturn(c, errcode.UserNotExistErr)
			return
		}
		response.FailReturn(c, errcode.ServerErr)
		return
	}

	// gen token
	authJwtImpl := jwt.GetAuthJwtImpl()
	token, errInfo := authJwtImpl.GenerateToken(&v1beta1.UserInfo{Username: user.Name})
	if errInfo != nil {
		clog.Info("gen token fail, %v", errInfo)
		response.FailReturn(c, errcode.ServerErr)
		return
	}
	result := map[string]string{
		"token": token,
	}
	response.SuccessReturn(c, result)
}

func GetUUID() string {
	uuid := uuid.NewV4().String()

	return strings.ReplaceAll(uuid, "-", "")
}
