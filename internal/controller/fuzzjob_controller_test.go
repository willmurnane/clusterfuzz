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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	targetsv1alpha1 "github.com/willmurnane/clusterfuzz/api/v1alpha1"
)

var _ = Describe("FuzzJob Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		fuzzjob := &targetsv1alpha1.FuzzJob{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind FuzzJob")
			err := k8sClient.Get(ctx, typeNamespacedName, fuzzjob)
			if err != nil && errors.IsNotFound(err) {
				resource := &targetsv1alpha1.FuzzJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: targetsv1alpha1.FuzzJobSpec{
						Cores: 123,
						TargetRef: &targetsv1alpha1.TargetReference{
							Kind: "S3Target",
							Name: "some-target-name",
						},
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &targetsv1alpha1.FuzzJob{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance FuzzJob")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should fail to reconcile the resource when the target is missing", func() {
			By("Reconciling the created resource")
			controllerReconciler := &FuzzJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("s3targets.fuzz.will.murnane.family \"some-target-name\" not found"))
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
		It("should successfully reconcile the resource when the target exists", func() {
			By("creating the target resource for the FuzzJob")
			targetResource := &targetsv1alpha1.S3Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-target-name",
					Namespace: "default",
				},
				Spec: targetsv1alpha1.S3TargetSpec{
					Bucket: "some-bucket",
					Region: "some-region",
				},
			}
			Expect(k8sClient.Create(ctx, targetResource)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, targetResource)).To(Succeed())
			}()

			By("Reconciling the created resource")
			controllerReconciler := &FuzzJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
