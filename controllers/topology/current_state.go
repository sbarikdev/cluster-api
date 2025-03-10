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
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getCurrentState gets information about the current state of a Cluster by inspecting the state of the InfrastructureCluster,
// the ControlPlane, and the MachineDeployments associated with the Cluster.
func (r *ClusterReconciler) getCurrentState(ctx context.Context, cluster *clusterv1.Cluster, class *clusterv1.ClusterClass) (*clusterTopologyState, error) {
	clusterState := clusterTopologyState{
		cluster: cluster,
	}

	// Reference to the InfrastructureCluster can be nil and is expected to be on the first reconcile.
	// In this case the method should still be allowed to continue.
	if cluster.Spec.InfrastructureRef != nil {
		infra, err := r.getCurrentInfrastructureClusterState(ctx, cluster)
		if err != nil {
			return nil, err
		}
		clusterState.infrastructureCluster = infra
	}

	// Reference to the ControlPlane can be nil, and is expected to be on the first reconcile. In this case the method
	// should still be allowed to continue.
	if cluster.Spec.ControlPlaneRef != nil {
		cp, err := r.getCurrentControlPlaneState(ctx, cluster, class)
		if err != nil {
			return nil, err
		}
		clusterState.controlPlane = cp
	}

	// A Cluster may have zero or more MachineDeployments and a Cluster is expected to have zero MachineDeployments on
	// first reconcile.
	m, err := r.getCurrentMachineDeploymentState(ctx, cluster)
	if err != nil {
		return nil, err
	}
	clusterState.machineDeployments = m
	return &clusterState, nil
}

// getCurrentInfrastructureClusterState looks for the state of the InfrastructureCluster. If a reference is set but not
// found, either from an error or the object not being found, an error is thrown.
func (r *ClusterReconciler) getCurrentInfrastructureClusterState(ctx context.Context, cluster *clusterv1.Cluster) (*unstructured.Unstructured, error) {
	infra, err := r.getReference(ctx, cluster.Spec.InfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s %s", cluster.Spec.InfrastructureRef.Kind, cluster.Spec.InfrastructureRef.Name)
	}
	return infra, nil
}

// getCurrentControlPlaneState returns information on the ControlPlane being used by the Cluster. If a reference is not found,
// an error is thrown. If the ControlPlane requires MachineInfrastructure according to its ClusterClass an error will be
// thrown if the ControlPlane has no MachineTemplates.
func (r *ClusterReconciler) getCurrentControlPlaneState(ctx context.Context, cluster *clusterv1.Cluster, class *clusterv1.ClusterClass) (*controlPlaneTopologyState, error) {
	var err error
	res := &controlPlaneTopologyState{}

	// Get the control plane object.
	res.object, err = r.getReference(ctx, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s %s", cluster.Spec.ControlPlaneRef.Kind, cluster.Spec.ControlPlaneRef.Name)
	}

	// Some ControlPlane providers may not require MachineInfrastructure to run. This check returns early if the field
	// indicating MachineInfrastructure is required is not found in the ClusterClass of the given Cluster.
	if class.Spec.ControlPlane.MachineInfrastructure == nil {
		return res, nil
	}

	// Get this control plane machine infrastructure reference.
	machineInfrastructureRef, err := getNestedRef(res.object, "spec", "machineTemplate", "infrastructureRef")
	if err != nil {
		return res, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate reference for %s, %s", res.object.GetKind(), res.object.GetName())
	}
	res.infrastructureMachineTemplate, err = r.getReference(ctx, machineInfrastructureRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get InfrastructureMachineTemplate for %s, %s", res.object.GetKind(), res.object.GetName())
	}

	return res, nil
}

// getCurrentMachineDeploymentState queries for all MachineDeployments and filters them for their linked Cluster and
// whether they are managed by a ClusterClass using labels. A Cluster may have zero or more MachineDeployments. Zero is
// expected on first reconcile. If MachineDeployments are found for the Cluster their Infrastructure and Bootstrap references
// are inspected. Where these are not found the function will throw an error.
func (r *ClusterReconciler) getCurrentMachineDeploymentState(ctx context.Context, cluster *clusterv1.Cluster) (map[string]*machineDeploymentTopologyState, error) {
	state := make(map[string]*machineDeploymentTopologyState)

	// List all the machine deployments in the current cluster and in a managed topology.
	md := &clusterv1.MachineDeploymentList{}
	err := r.Client.List(ctx, md, client.MatchingLabels{
		clusterv1.ClusterLabelName:         cluster.Name,
		clusterv1.ClusterTopologyLabelName: "",
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read MachineDeployments for managed topology")
	}

	// Loop over each machine deployment and create the current
	// state by retrieving all required references.
	for i := range md.Items {
		m := &md.Items[i]

		// Retrieve the name which is usually assigned in Cluster's topology
		// from a well-defined label.
		mdTopologyName, ok := m.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
		if !ok || len(mdTopologyName) == 0 {
			return nil, fmt.Errorf("failed to find label %s in %s", clusterv1.ClusterTopologyMachineDeploymentLabelName, m.Name)
		}

		// Make sure that the name of the MachineDeployment stays unique.
		// If we've already have seen a MachineDeployment with the same name
		// this is an error, probably caused from manual modifications or a race condition.
		if _, ok := state[mdTopologyName]; ok {
			return nil, fmt.Errorf("duplicate machine deployment %s found for label %s: %s", m.Name, clusterv1.ClusterTopologyMachineDeploymentLabelName, mdTopologyName)
		}
		infraRef := &m.Spec.Template.Spec.InfrastructureRef
		if infraRef == nil {
			return nil, fmt.Errorf("MachineDeployment %s does not have a reference to a InfrastructureMachineTemplate", m.Name)
		}

		bootstrapRef := m.Spec.Template.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			return nil, fmt.Errorf("MachineDeployment %s does not have a reference to a Bootstrap Config", m.Name)
		}

		i, err := r.getReference(ctx, infraRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("MachineDeployment %s Infrastructure reference could not be retrieved", m.Name))
		}
		b, err := r.getReference(ctx, bootstrapRef)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("MachineDeployment %s Bootstrap reference could not be retrieved", m.Name))
		}
		state[mdTopologyName] = &machineDeploymentTopologyState{
			object:                        m,
			bootstrapTemplate:             b,
			infrastructureMachineTemplate: i,
		}
	}
	return state, nil
}
