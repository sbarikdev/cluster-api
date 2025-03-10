/*
Copyright 2021 The Kubernetes Authors.

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

package topology

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

var (
	fakeRef1 = &corev1.ObjectReference{
		Kind:       "refKind1",
		Namespace:  "refNamespace1",
		Name:       "refName1",
		APIVersion: "refAPIVersion1",
	}

	fakeRef2 = &corev1.ObjectReference{
		Kind:       "refKind2",
		Namespace:  "refNamespace2",
		Name:       "refName2",
		APIVersion: "refAPIVersion2",
	}
)

func TestComputeInfrastructureCluster(t *testing.T) {
	// templates and ClusterClass
	infrastructureClusterTemplate := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "template1").Obj()
	clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate).
		Obj()

	// aggregating templates and cluster class into topologyClass (simulating getClass)
	topologyClass := &clusterTopologyClass{
		clusterClass:                  clusterClass,
		infrastructureClusterTemplate: infrastructureClusterTemplate,
	}

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	t.Run("Generates the infrastructureCluster from the template", func(t *testing.T) {
		g := NewWithT(t)

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeInfrastructureCluster(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.Infrastructure.Ref,
			template:    topologyClass.infrastructureClusterTemplate,
			labels:      nil,
			annotations: nil,
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("If there is already a reference to the infrastructureCluster, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithInfrastructureRef := cluster.DeepCopy()
		clusterWithInfrastructureRef.Spec.InfrastructureRef = fakeRef1

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: clusterWithInfrastructureRef,
		}

		obj, err := computeInfrastructureCluster(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.Infrastructure.Ref,
			template:    topologyClass.infrastructureClusterTemplate,
			labels:      nil,
			annotations: nil,
			currentRef:  current.cluster.Spec.InfrastructureRef,
			obj:         obj,
		})
	})
}

func TestComputeControlPlaneInfrastructureMachineTemplate(t *testing.T) {
	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Obj()
	clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Obj()

	// aggregating templates and cluster class into topologyClass (simulating getClass)
	topologyClass := &clusterTopologyClass{
		clusterClass: clusterClass,
		controlPlane: &controlPlaneTopologyClass{
			infrastructureMachineTemplate: infrastructureMachineTemplate,
		},
	}

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				ControlPlane: clusterv1.ControlPlaneTopology{
					Metadata: clusterv1.ObjectMeta{
						Labels:      map[string]string{"l2": ""},
						Annotations: map[string]string{"a2": ""},
					},
				},
			},
		},
	}

	t.Run("Generates the infrastructureMachineTemplate from the template", func(t *testing.T) {
		g := NewWithT(t)

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeControlPlaneInfrastructureMachineTemplate(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			template:    topologyClass.controlPlane.infrastructureMachineTemplate,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("If there is already a reference to the infrastructureMachineTemplate, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		currentInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "cluster1-template1").Obj()

		controlPlane := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err := setNestedRef(controlPlane, currentInfrastructureMachineTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
			controlPlane: &controlPlaneTopologyState{
				object:                        controlPlane,
				infrastructureMachineTemplate: currentInfrastructureMachineTemplate,
			},
		}

		obj, err := computeControlPlaneInfrastructureMachineTemplate(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			template:    topologyClass.controlPlane.infrastructureMachineTemplate,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  objToRef(currentInfrastructureMachineTemplate),
			obj:         obj,
		})
	})
}

func TestComputeControlPlane(t *testing.T) {
	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	controlPlaneTemplate := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "template1").Obj()
	clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneTemplate(controlPlaneTemplate).
		Obj()

	// aggregating templates and cluster class into topologyClass (simulating getClass)
	topologyClass := &clusterTopologyClass{
		clusterClass: clusterClass,
		controlPlane: &controlPlaneTopologyClass{
			template: controlPlaneTemplate,
		},
	}

	// current cluster objects
	version := "v1.21.2"
	replicas := 3
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				Version: version,
				ControlPlane: clusterv1.ControlPlaneTopology{
					Metadata: clusterv1.ObjectMeta{
						Labels:      map[string]string{"l2": ""},
						Annotations: map[string]string{"a2": ""},
					},
					Replicas: &replicas,
				},
			},
		},
	}

	t.Run("Generates the ControlPlane from the template", func(t *testing.T) {
		g := NewWithT(t)

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeControlPlane(topologyClass, current, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, "spec", "version")
		assertNestedField(g, obj, int64(replicas), "spec", "replicas")
		assertNestedFieldUnset(g, obj, "spec", "machineTemplate", "infrastructureRef")
	})
	t.Run("Skips setting replicas if required", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects
		clusterWithoutReplicas := cluster.DeepCopy()
		clusterWithoutReplicas.Spec.Topology.ControlPlane.Replicas = nil

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: clusterWithoutReplicas,
		}

		obj, err := computeControlPlane(topologyClass, current, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, "spec", "version")
		assertNestedFieldUnset(g, obj, "spec", "replicas")
		assertNestedFieldUnset(g, obj, "spec", "machineTemplate", "infrastructureRef")
	})
	t.Run("Generates the ControlPlane from the template and adds the infrastructure machine template if required", func(t *testing.T) {
		g := NewWithT(t)

		// templates and ClusterClass
		infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Obj()
		clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
			WithControlPlaneMetadata(labels, annotations).
			WithControlPlaneTemplate(controlPlaneTemplate).
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
			Obj()

		// aggregating templates and cluster class into topologyClass (simulating getClass)
		topologyClass := &clusterTopologyClass{
			clusterClass: clusterClass,
			controlPlane: &controlPlaneTopologyClass{
				template:                      controlPlaneTemplate,
				infrastructureMachineTemplate: infrastructureMachineTemplate,
			},
		}

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeControlPlane(topologyClass, current, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, "spec", "version")
		assertNestedField(g, obj, int64(replicas), "spec", "replicas")
		assertNestedField(g, obj, map[string]interface{}{
			"kind":       infrastructureMachineTemplate.GetKind(),
			"namespace":  infrastructureMachineTemplate.GetNamespace(),
			"name":       infrastructureMachineTemplate.GetName(),
			"apiVersion": infrastructureMachineTemplate.GetAPIVersion(),
		}, "spec", "machineTemplate", "infrastructureRef")
	})
	t.Run("If there is already a reference to the ControlPlane, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithControlPlaneRef := cluster.DeepCopy()
		clusterWithControlPlaneRef.Spec.ControlPlaneRef = fakeRef1

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: clusterWithControlPlaneRef,
		}

		obj, err := computeControlPlane(topologyClass, current, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  current.cluster.Spec.ControlPlaneRef,
			obj:         obj,
		})
	})
}

func TestComputeCluster(t *testing.T) {
	// generated objects
	infrastructureCluster := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructureCluster1").Obj()
	controlPlane := newFakeControlPlane(metav1.NamespaceDefault, "controlplane1").Obj()

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
	current := &clusterTopologyState{
		cluster: cluster,
	}

	g := NewWithT(t)

	obj := computeCluster(current, infrastructureCluster, controlPlane)
	g.Expect(obj).ToNot(BeNil())

	// TypeMeta
	g.Expect(obj.APIVersion).To(Equal(cluster.APIVersion))
	g.Expect(obj.Kind).To(Equal(cluster.Kind))

	// ObjectMeta
	g.Expect(obj.Name).To(Equal(cluster.Name))
	g.Expect(obj.Namespace).To(Equal(cluster.Namespace))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyLabelName, ""))

	// Spec
	g.Expect(obj.Spec.InfrastructureRef).To(Equal(objToRef(infrastructureCluster)))
	g.Expect(obj.Spec.ControlPlaneRef).To(Equal(objToRef(controlPlane)))
}

func TestComputeMachineDeployment(t *testing.T) {
	workerInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "linux-worker-inframachinetemplate").Obj()
	workerBootstrapTemplate := newFakeBootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstraptemplate").Obj()

	labels := map[string]string{"fizz": "buzz", "foo": "bar"}
	annotations := map[string]string{"annotation-1": "annotation-1-val"}

	fakeClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithWorkerMachineDeploymentClass("linux-worker", labels, annotations, workerInfrastructureMachineTemplate, workerBootstrapTemplate).
		Obj()
	class := &clusterTopologyClass{
		clusterClass: fakeClass,
		machineDeployments: map[string]*machineDeploymentTopologyClass{
			"linux-worker": {
				metadata: clusterv1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				bootstrapTemplate:             workerBootstrapTemplate,
				infrastructureMachineTemplate: workerInfrastructureMachineTemplate,
			},
		},
	}

	current := &clusterTopologyState{
		cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
	}

	replicas := 5
	mdTopology := clusterv1.MachineDeploymentTopology{
		Metadata: clusterv1.ObjectMeta{
			Labels: map[string]string{"foo": "baz"},
		},
		Class:    "linux-worker",
		Name:     "big-pool-of-machines",
		Replicas: &replicas,
	}

	t.Run("Generates the machine deployment and the referenced templates", func(t *testing.T) {
		g := NewWithT(t)
		actual, err := computeMachineDeployment(class, current, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMd := actual.object
		g.Expect(*actualMd.Spec.Replicas).To(Equal(int32(replicas)))
		g.Expect(actualMd.Spec.ClusterName).To(Equal("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("big-pool-of-machines"))

		g.Expect(actualMd.Labels).To(HaveKeyWithValue("foo", "baz"))
		g.Expect(actualMd.Labels).To(HaveKeyWithValue("fizz", "buzz"))
		g.Expect(actualMd.Labels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		g.Expect(actualMd.Spec.Template.Spec.InfrastructureRef.Name).ToNot(Equal("linux-worker-inframachinetemplate"))
		g.Expect(actualMd.Spec.Template.Spec.Bootstrap.ConfigRef.Name).ToNot(Equal("linux-worker-bootstraptemplate"))
	})

	t.Run("If there is already a machine deployment, it preserves the object name and the reference names", func(t *testing.T) {
		g := NewWithT(t)

		currentReplicas := int32(3)
		currentMd := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "existing-deployment-1",
				Labels: map[string]string{"a": "1", "b": "2"},
			},
			Spec: clusterv1.MachineDeploymentSpec{
				Replicas: &currentReplicas,
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: objToRef(workerBootstrapTemplate),
						},
						InfrastructureRef: *objToRef(workerInfrastructureMachineTemplate),
					},
				},
			},
		}
		current.machineDeployments = map[string]*machineDeploymentTopologyState{
			"big-pool-of-machines": {
				object:                        currentMd,
				bootstrapTemplate:             workerBootstrapTemplate,
				infrastructureMachineTemplate: workerInfrastructureMachineTemplate,
			},
		}

		actual, err := computeMachineDeployment(class, current, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMd := actual.object
		g.Expect(*actualMd.Spec.Replicas).NotTo(Equal(currentReplicas))
		g.Expect(actualMd.Name).To(Equal("existing-deployment-1"))

		g.Expect(actualMd.Labels).NotTo(HaveKey("a"))
		g.Expect(actualMd.Labels).NotTo(HaveKey("b"))
		g.Expect(actualMd.Labels).To(HaveKeyWithValue("foo", "baz"))
		g.Expect(actualMd.Labels).To(HaveKeyWithValue("fizz", "buzz"))
		g.Expect(actualMd.Labels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		g.Expect(actualMd.Spec.Template.Spec.InfrastructureRef.Name).To(Equal("linux-worker-inframachinetemplate"))
		g.Expect(actualMd.Spec.Template.Spec.Bootstrap.ConfigRef.Name).To(Equal("linux-worker-bootstraptemplate"))
	})

	t.Run("If a machine deployment references a topology class that does not exist, machine deployment generation fails", func(t *testing.T) {
		g := NewWithT(t)
		mdTopology = clusterv1.MachineDeploymentTopology{
			Metadata: clusterv1.ObjectMeta{
				Labels: map[string]string{"foo": "baz"},
			},
			Class: "windows-worker",
			Name:  "big-pool-of-machines",
		}

		_, err := computeMachineDeployment(class, current, mdTopology)
		g.Expect(err).To(HaveOccurred())
	})
}

func TestTemplateToObject(t *testing.T) {
	template := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "infrastructureClusterTemplate").Obj()
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	t.Run("Generates an object from a template", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToObject(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      nil,
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("Overrides the generated name if there is already a reference", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToObject(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      fakeRef2,
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// ObjectMeta
		assertTemplateToObject(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
			currentRef:  fakeRef2,
			obj:         obj,
		})
	})
}

func TestTemplateToTemplate(t *testing.T) {
	template := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "infrastructureClusterTemplate").Obj()
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	t.Run("Generates a template from a template", func(t *testing.T) {
		g := NewWithT(t)
		obj := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      nil,
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("Overrides the generated name if there is already a reference", func(t *testing.T) {
		g := NewWithT(t)
		obj := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      fakeRef2,
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
			currentRef:  fakeRef2,
			obj:         obj,
		})
	})
}

type assertTemplateInput struct {
	cluster             *clusterv1.Cluster
	templateRef         *corev1.ObjectReference
	template            *unstructured.Unstructured
	labels, annotations map[string]string
	currentRef          *corev1.ObjectReference
	obj                 *unstructured.Unstructured
}

func assertTemplateToObject(g *WithT, in assertTemplateInput) {
	// TypeMeta
	g.Expect(in.obj.GetAPIVersion()).To(Equal(in.template.GetAPIVersion()))
	g.Expect(in.obj.GetKind()).To(Equal(strings.TrimSuffix(in.template.GetKind(), "Template")))

	// ObjectMeta
	if in.currentRef != nil {
		g.Expect(in.obj.GetName()).To(Equal(in.currentRef.Name))
	} else {
		g.Expect(in.obj.GetName()).To(HavePrefix(in.cluster.Name))
	}
	g.Expect(in.obj.GetNamespace()).To(Equal(in.cluster.Namespace))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, in.cluster.Name))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyLabelName, ""))
	for k, v := range in.labels {
		g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(k, v))
	}
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, in.templateRef.GroupVersionKind().GroupKind().String()))
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, in.templateRef.Name))
	for k, v := range in.annotations {
		g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(k, v))
	}
	// Spec
	expectedSpec, ok, err := unstructured.NestedMap(in.template.UnstructuredContent(), "spec", "template", "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	cloneSpec, ok, err := unstructured.NestedMap(in.obj.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	for k, v := range expectedSpec {
		g.Expect(cloneSpec).To(HaveKeyWithValue(k, v))
	}
}

func assertTemplateToTemplate(g *WithT, in assertTemplateInput) {
	// TypeMeta
	g.Expect(in.obj.GetAPIVersion()).To(Equal(in.template.GetAPIVersion()))
	g.Expect(in.obj.GetKind()).To(Equal(in.template.GetKind()))

	// ObjectMeta
	if in.currentRef != nil {
		g.Expect(in.obj.GetName()).To(Equal(in.currentRef.Name))
	} else {
		g.Expect(in.obj.GetName()).To(HavePrefix(in.cluster.Name))
	}
	g.Expect(in.obj.GetNamespace()).To(Equal(in.cluster.Namespace))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, in.cluster.Name))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyLabelName, ""))
	for k, v := range in.labels {
		g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(k, v))
	}
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, in.templateRef.GroupVersionKind().GroupKind().String()))
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, in.templateRef.Name))
	for k, v := range in.annotations {
		g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(k, v))
	}
	// Spec
	expectedSpec, ok, err := unstructured.NestedMap(in.template.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	cloneSpec, ok, err := unstructured.NestedMap(in.obj.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(Equal(expectedSpec))
}

func assertNestedField(g *WithT, obj *unstructured.Unstructured, value interface{}, fields ...string) {
	v, ok, err := unstructured.NestedFieldCopy(obj.UnstructuredContent(), fields...)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(v).To(Equal(value))
}

func assertNestedFieldUnset(g *WithT, obj *unstructured.Unstructured, fields ...string) {
	_, ok, err := unstructured.NestedFieldCopy(obj.UnstructuredContent(), fields...)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeFalse())
}
