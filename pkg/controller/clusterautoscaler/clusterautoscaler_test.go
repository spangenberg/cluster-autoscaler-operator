package clusterautoscaler

import (
	"fmt"
	"github.com/openshift/cluster-autoscaler-operator/pkg/apis"
	autoscalingv1alpha1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"strings"
	"testing"
)

const (
	NvidiaGPU         = "nvidia.com/gpu"
	TestNamespace     = "test"
	TestCloudProvider = "testProvider"
)

var (
	ScaleDownUnneededTime        = "10s"
	ScaleDownDelayAfterAdd       = "60s"
	PodPriorityThreshold   int32 = -10
	MaxPodGracePeriod      int32 = 60
	MaxNodesTotal          int32 = 100
	CoresMin               int32 = 16
	CoresMax               int32 = 32
	MemoryMin              int32 = 32
	MemoryMax              int32 = 64
	NvidiaGPUMin           int32 = 4
	NvidiaGPUMax           int32 = 8
)

func init() {
	apis.AddToScheme(scheme.Scheme)
}

func NewClusterAutoscaler() *autoscalingv1alpha1.ClusterAutoscaler {
	// TODO: Maybe just deserialize this from a YAML file?
	return &autoscalingv1alpha1.ClusterAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterAutoscaler",
			APIVersion: "autoscaling.openshift.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: TestNamespace,
		},
		Spec: autoscalingv1alpha1.ClusterAutoscalerSpec{
			MaxPodGracePeriod:    &MaxPodGracePeriod,
			PodPriorityThreshold: &PodPriorityThreshold,
			ResourceLimits: &autoscalingv1alpha1.ResourceLimits{
				MaxNodesTotal: &MaxNodesTotal,
				Cores: &autoscalingv1alpha1.ResourceRange{
					Min: CoresMin,
					Max: CoresMax,
				},
				Memory: &autoscalingv1alpha1.ResourceRange{
					Min: MemoryMin,
					Max: MemoryMax,
				},
				GPUS: []autoscalingv1alpha1.GPULimit{
					{
						Type: NvidiaGPU,
						Min:  NvidiaGPUMin,
						Max:  NvidiaGPUMax,
					},
				},
			},
			ScaleDown: &autoscalingv1alpha1.ScaleDownConfig{
				Enabled:       true,
				DelayAfterAdd: &ScaleDownDelayAfterAdd,
				UnneededTime:  &ScaleDownUnneededTime,
			},
		},
	}
}

func includesStringWithPrefix(list []string, prefix string) bool {
	for i := range list {
		if strings.HasPrefix(list[i], prefix) {
			return true
		}
	}

	return false
}

func includeString(list []string, item string) bool {
	for i := range list {
		if list[i] == item {
			return true
		}
	}

	return false
}

func TestAutoscalerArgs(t *testing.T) {
	ca := NewClusterAutoscaler()

	args := AutoscalerArgs(ca, &Config{CloudProvider: TestCloudProvider, Namespace: TestNamespace})

	expected := []string{
		fmt.Sprintf("--scale-down-delay-after-add=%s", ScaleDownDelayAfterAdd),
		fmt.Sprintf("--scale-down-unneeded-time=%s", ScaleDownUnneededTime),
		fmt.Sprintf("--expendable-pods-priority-cutoff=%d", PodPriorityThreshold),
		fmt.Sprintf("--max-graceful-termination-sec=%d", MaxPodGracePeriod),
		fmt.Sprintf("--cores-total=%d:%d", CoresMin, CoresMax),
		fmt.Sprintf("--max-nodes-total=%d", MaxNodesTotal),
		fmt.Sprintf("--namespace=%s", TestNamespace),
		fmt.Sprintf("--cloud-provider=%s", TestCloudProvider),
	}

	for _, e := range expected {
		if !includeString(args, e) {
			t.Fatalf("missing arg: %s", e)
		}
	}

	expectedMissing := []string{
		"--scale-down-delay-after-delete",
		"--scale-down-delay-after-failure",
	}

	for _, e := range expectedMissing {
		if includesStringWithPrefix(args, e) {
			t.Fatalf("found arg expected to be missing: %s", e)
		}
	}
}

// This test ensures we can actually get an autoscaler with fakeclient/client.
// fakeclient.NewFakeClientWithScheme will os.Exit(1) with invalid scheme.
func TestCanGetca(t *testing.T) {
	_ = fakeclient.NewFakeClient(NewClusterAutoscaler())
}

// newFakeReconciler returns a new reconcile.Reconciler with a fake client
func newFakeReconciler(initObjects ...runtime.Object) *Reconciler {
	fakeClient := fakeclient.NewFakeClient(initObjects...)
	return &Reconciler{
		client: fakeClient,
		scheme: scheme.Scheme,
	}
}

func TestAvailableAndUpdated(t *testing.T) {
	ca := NewClusterAutoscaler()
	dep1 := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-autoscaler-test",
			Namespace: TestNamespace,
			Annotations: map[string]string{
				"release.openshift.io/version": "test-1",
			},
			Generation: 1,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			UpdatedReplicas:    1,
			Replicas:           1,
			AvailableReplicas:  1,
		},
	}
	dep2 := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-autoscaler-test",
			Namespace: TestNamespace,
			Annotations: map[string]string{
				"release.openshift.io/version": "test-2",
			},
			Generation: 1,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			UpdatedReplicas:    1,
			Replicas:           1,
			AvailableReplicas:  1,
		},
	}
	cfg1 := Config{
		ReleaseVersion: "test-1",
		Name:           "test",
		Namespace:      TestNamespace,
	}
	cfg2 := Config{
		ReleaseVersion: "test-2",
		Name:           "test",
		Namespace:      TestNamespace,
	}
	cfg3 := Config{
		ReleaseVersion: "test-2",
		Name:           "test2",
		Namespace:      TestNamespace,
	}
	tCases := []struct {
		expectedError error
		expectedOk    bool
		c             *Config
		d             *appsv1.Deployment
	}{
		// Case 0: should pass, returns true, nil.
		{
			expectedError: nil,
			expectedOk:    true,
			c:             &cfg1,
			d:             &dep1,
		},
		// Case 1: CA found is wrong version, should return false, nil.
		{
			expectedError: nil,
			expectedOk:    false,
			c:             &cfg2,
			d:             &dep1,
		},
		// Case 2: No CA in namespace, returns true, nil.
		{
			expectedError: nil,
			expectedOk:    true,
			c:             &cfg3,
			d:             &dep1,
		},
		// Case 3: No deployment found, returns false, nil.
		{
			expectedError: nil,
			expectedOk:    false,
			c:             &cfg1,
			d:             &appsv1.Deployment{},
		},
		// Case 4: Deployment wrong annotation, returns false, nil.
		{
			expectedError: nil,
			expectedOk:    false,
			c:             &cfg1,
			d:             &dep2,
		},
	}
	for i, tc := range tCases {
		r := newFakeReconciler(ca, tc.d)
		r.SetConfig(tc.c)
		ok, err := r.AvailableAndUpdated()
		assert.Equal(t, tc.expectedOk, ok, "case %v: expected true", i)
		assert.Equal(t, tc.expectedError, err, "case %v: expected nil", i)
	}

}
