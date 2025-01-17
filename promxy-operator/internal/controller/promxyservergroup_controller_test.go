/*
Copyright 2025.

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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kofv1alpha1 "github.com/k0rdent/kof/promxy-operator/api/v1alpha1"
)

var _ = Describe("PromxyServerGroup Controller", func() {
	Context("When reconciling a resource", func() {
		const promxyServerGroupName = "test-resource"
		const promxySecretName = "test-promxy-secret"
		const credentialsSecretName = "test-cluster-credentials"

		ctx := context.Background()

		promxyServerGroupNamespacedName := types.NamespacedName{
			Name:      promxyServerGroupName,
			Namespace: "default",
		}
		promxyservergroup := &kofv1alpha1.PromxyServerGroup{}

		credentialsSecretNamespacesName := types.NamespacedName{
			Name:      credentialsSecretName,
			Namespace: "default",
		}
		credentialsSecret := &coreV1.Secret{}

		promxySecretNamespacedName := types.NamespacedName{
			Name:      promxySecretName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PromxyServerGroup")
			err := k8sClient.Get(ctx, promxyServerGroupNamespacedName, promxyservergroup)
			if err != nil && errors.IsNotFound(err) {
				resource := &kofv1alpha1.PromxyServerGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      promxyServerGroupName,
						Namespace: "default",
						Labels:    make(map[string]string),
					},
					Spec: kofv1alpha1.PromxyServerGroupSpec{
						ClusterName: "test-cluster",
						Targets:     []string{"test.example.net:443"},
						PathPrefix:  "/storage/source",
						Scheme:      "https",
						HttpClient: kofv1alpha1.HTTPClientConfig{
							DialTimeout: metav1.Duration{Duration: time.Second},
							TLSConfig: kofv1alpha1.TLSConfig{
								InsecureSkipVerify: true,
							},
							BasicAuth: kofv1alpha1.BasicAuth{
								CredentialsSecretName: credentialsSecretName,
								UsernameKey:           "username",
								PasswordKey:           "password",
							},
						},
					},
				}
				resource.ObjectMeta.Labels[SecretNameLabel] = promxySecretName
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
			By("creating the Secret for the server group credentials")
			err = k8sClient.Get(ctx, credentialsSecretNamespacesName, credentialsSecret)
			if err != nil && errors.IsNotFound(err) {
				resource := &coreV1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      credentialsSecretName,
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "promxy-operator",
						},
					},
					StringData: map[string]string{
						"username": "u",
						"password": "p",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

		})

		AfterEach(func() {
			serverGroup := &kofv1alpha1.PromxyServerGroup{}
			err := k8sClient.Get(ctx, promxyServerGroupNamespacedName, serverGroup)
			Expect(err).NotTo(HaveOccurred())

			credentialsSecret := &coreV1.Secret{}
			err = k8sClient.Get(ctx, credentialsSecretNamespacesName, credentialsSecret)
			Expect(err).NotTo(HaveOccurred())

			promxySecret := &coreV1.Secret{}
			err = k8sClient.Get(ctx, promxySecretNamespacedName, promxySecret)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the PromxyServerGroup")
			Expect(k8sClient.Delete(ctx, serverGroup)).To(Succeed())
			By("Cleanup the Credentials Secret")
			Expect(k8sClient.Delete(ctx, credentialsSecret)).To(Succeed())
			By("Cleanup the Promxy Secret")
			Expect(k8sClient.Delete(ctx, promxySecret)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PromxyServerGroupReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				RemoteWriteUrl: "http://storage/write",
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: promxyServerGroupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			secret := &coreV1.Secret{}
			err = k8sClient.Get(ctx, promxySecretNamespacedName, secret)
			Expect(err).NotTo(HaveOccurred())

			promxyConfig := string(secret.Data["config.yaml"])
			promxyConfigYaml := make(map[string]interface{})
			Expect(promxyConfig).ToNot(BeNil())
			Expect(yaml.Unmarshal([]byte(promxyConfig), promxyConfigYaml)).ToNot(HaveOccurred())
			Expect("\n" + promxyConfig).To(Equal(`
global:
  evaluation_interval: 5s
  external_labels:
    source: promxy
remote_write:
  - url: "http://storage/write"
promxy:
  server_groups:
    - static_configs:
        - targets:
          - "test.example.net:443"
      path_prefix: "/storage/source"
      scheme: "https"
      http_client:
        dial_timeout: "1s"
        tls_config:
          insecure_skip_verify: true
        basic_auth:
          username: "u"
          password: "p"
      labels:
        promxyCluster: "test-cluster"
`))
		})
	})
})
