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

package quota

import (
	"context"
	"errors"
	"net/http"

	"github.com/kubecube-io/kubecube/pkg/clog"
	"github.com/kubecube-io/kubecube/pkg/quota/k8s"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ResourceQuotaValidator struct {
	PivotClient client.Client
	decoder     *admission.Decoder
}

func (r *ResourceQuotaValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	oldQuota := &v1.ResourceQuota{}
	currentQuota := &v1.ResourceQuota{}

	defer func() {
		clog.Debug("operation: %v, current quota: %+v, old quota: %+v", req.Operation, currentQuota, oldQuota)
	}()

	switch req.Operation {
	case admissionv1.Create:
		err := r.decoder.Decode(req, currentQuota)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		oldQuota = nil
	case admissionv1.Update:
		err := r.decoder.Decode(req, currentQuota)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		err = r.decoder.DecodeRaw(req.OldObject, oldQuota)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	case admissionv1.Delete:
		err := r.decoder.DecodeRaw(req.OldObject, oldQuota)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		currentQuota = nil
	}

	q := k8s.NewQuotaOperator(r.PivotClient, currentQuota, oldQuota, context.Background())

	if req.Operation != admissionv1.Delete {
		isOverLoad, err := q.Overload()
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		if isOverLoad {
			reason := "request of resource currentQuota overload"
			clog.Warn(reason)
			return admission.Errored(http.StatusNotAcceptable, errors.New(reason))
		}
	}

	if k8s.IsRelyOnObj(currentQuota, oldQuota) {
		go callback(q, req.Operation == admissionv1.Delete)
	}

	return admission.Allowed("")
}

// InjectDecoder injects the decoder.
func (r *ResourceQuotaValidator) InjectDecoder(d *admission.Decoder) error {
	r.decoder = d
	return nil
}
