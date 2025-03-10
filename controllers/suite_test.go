/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/api/v1alpha4/index"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/internal/envtest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	// +kubebuilder:scaffold:imports
)

const (
	timeout         = time.Second * 30
	testClusterName = "test-cluster"
)

var (
	env        *envtest.Environment
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
	_ = apiextensionsv1.AddToScheme(fakeScheme)
}

func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		// Set up a ClusterCacheTracker and ClusterCacheReconciler to provide to controllers
		// requiring a connection to a remote cluster
		tracker, err := remote.NewClusterCacheTracker(
			mgr,
			remote.ClusterCacheTrackerOptions{
				Log:     ctrl.Log.WithName("remote").WithName("ClusterCacheTracker"),
				Indexes: remote.DefaultIndexes,
			},
		)
		if err != nil {
			panic(fmt.Sprintf("unable to create cluster cache tracker: %v", err))
		}
		if err := (&remote.ClusterCacheReconciler{
			Client:  mgr.GetClient(),
			Log:     ctrl.Log.WithName("remote").WithName("ClusterCacheReconciler"),
			Tracker: tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterCacheReconciler: %v", err))
		}
		if err := (&ClusterReconciler{
			Client:   mgr.GetClient(),
			recorder: mgr.GetEventRecorderFor("cluster-controller"),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterReconciler: %v", err))
		}
		if err := (&MachineReconciler{
			Client:   mgr.GetClient(),
			Tracker:  tracker,
			recorder: mgr.GetEventRecorderFor("machine-controller"),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MachineReconciler: %v", err))
		}
		if err := (&MachineSetReconciler{
			Client:   mgr.GetClient(),
			Tracker:  tracker,
			recorder: mgr.GetEventRecorderFor("machineset-controller"),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MMachineSetReconciler: %v", err))
		}
		if err := (&MachineDeploymentReconciler{
			Client:   mgr.GetClient(),
			recorder: mgr.GetEventRecorderFor("machinedeployment-controller"),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MMachineDeploymentReconciler: %v", err))
		}
		if err := (&MachineHealthCheckReconciler{
			Client:   mgr.GetClient(),
			Tracker:  tracker,
			recorder: mgr.GetEventRecorderFor("machinehealthcheck-controller"),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MachineHealthCheckReconciler : %v", err))
		}
	}

	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(timeout)

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                m,
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}

func ContainRefOfGroupKind(group, kind string) types.GomegaMatcher {
	return &refGroupKindMatcher{
		kind:  kind,
		group: group,
	}
}

type refGroupKindMatcher struct {
	kind  string
	group string
}

func (matcher *refGroupKindMatcher) Match(actual interface{}) (success bool, err error) {
	ownerRefs, ok := actual.([]metav1.OwnerReference)
	if !ok {
		return false, errors.Errorf("expected []metav1.OwnerReference; got %T", actual)
	}

	for _, ref := range ownerRefs {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return false, nil // nolint:nilerr // If we can't get the group version we can't match, but it's not a failure
		}
		if ref.Kind == matcher.kind && gv.Group == clusterv1.GroupVersion.Group {
			return true, nil
		}
	}

	return false, nil
}

func (matcher *refGroupKindMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %+v to contain refs of Group %s and Kind %s", actual, matcher.group, matcher.kind)
}

func (matcher *refGroupKindMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %+v not to contain refs of Group %s and Kind %s", actual, matcher.group, matcher.kind)
}
