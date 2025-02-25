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

package webhooks

import (
	hotplugWebhook "github.com/kubecube-io/kubecube/pkg/ctrlmgr/webhooks/hotplug"
	projectWebhook "github.com/kubecube-io/kubecube/pkg/ctrlmgr/webhooks/project"
	"github.com/kubecube-io/kubecube/pkg/ctrlmgr/webhooks/quota"
	tenantWebhook "github.com/kubecube-io/kubecube/pkg/ctrlmgr/webhooks/tenant"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	admisson "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWithWebhooks set up webhooks into manager
func SetupWithWebhooks(mgr manager.Manager) {
	hookServer := mgr.GetWebhookServer()

	client := mgr.GetClient()

	hookServer.Register("/validate-tenant-kubecube-io-v1-tenant", admisson.ValidatingWebhookFor(tenantWebhook.NewTenantValidator(client)))
	hookServer.Register("/validate-tenant-kubecube-io-v1-project", admisson.ValidatingWebhookFor(projectWebhook.NewProjectValidator(client)))
	hookServer.Register("/validate-hotplug-kubecube-io-v1-hotplug", admisson.ValidatingWebhookFor(hotplugWebhook.NewHotplugValidator(client)))

	hookServer.Register("/validate-quota-kubecube-io-v1-cube-resource-quota", &webhook.Admission{Handler: &quota.CubeResourceQuotaValidator{Client: client}})
}
