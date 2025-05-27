package source

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

var (
	namespace   = "default"
	serviceName = "test-service"
	svc         = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      serviceName,
		},
	}
	svcNonExistent = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "non-existent",
		},
	}
)

func TestCoreV1EndpointsGetServiceEndpoints(t *testing.T) {
	t.Parallel()

	// Create an endpoints object with one ready and one not-ready address.
	endpointsObj := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      serviceName,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1"},
					{IP: "10.0.0.2"},
				},
				NotReadyAddresses: []v1.EndpointAddress{
					{IP: "10.0.0.3"},
					{IP: "10.0.0.4"},
				},
			},
		},
	}

	client := fake.NewClientset(endpointsObj)
	informerFactory := kubeinformers.NewSharedInformerFactory(client, 0)
	source := newCoreV1EndpointsServiceEndpointSource(informerFactory)

	ctx := t.Context()
	informerFactory.Start(ctx.Done())

	// Wait for cache sync
	if !cacheSyncWithTimeout(informerFactory.Core().V1().Endpoints().Informer(), 5*time.Second) {
		t.Fatal("failed to sync endpoints informer")
	}

	t.Run("with an existing service", func(t *testing.T) {
		t.Parallel()
		endpoints, err := source.GetServiceEndpoints(svc)
		require.NoError(t, err)
		require.Len(t, endpoints, 4)
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.1", Ready: true})
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.2", Ready: true})
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.3", Ready: false})
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.4", Ready: false})
	})

	t.Run("with a service that does not exist", func(t *testing.T) {
		t.Parallel()
		endpoints, err := source.GetServiceEndpoints(svcNonExistent)
		require.NoError(t, err)
		require.Empty(t, endpoints)
	})
}

func TestDiscoveryV1EndpointSlicesGetServiceEndpoints(t *testing.T) {
	t.Parallel()

	ns := "default"
	serviceName := "test-service"

	// Create an EndpointSlice with one ready endpoint.
	endpointSliceList := []runtime.Object{
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "test-service-1",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: serviceName,
				},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.0.1"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
				},
				{
					Addresses:  []string{"10.0.0.2"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(false)},
				},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "test-service-2",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: serviceName,
				},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.0.3"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
				},
				{
					Addresses:  []string{"10.0.0.4"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(false)},
				},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "test-service-3",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: serviceName,
				},
			},
			AddressType: discoveryv1.AddressTypeFQDN, // Deprecated & unsupported address type
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.0.5"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
				},
				{
					Addresses:  []string{"10.0.0.6"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(false)},
				},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "another-service-1",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: "another-service",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.1.1"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
				},
				{
					Addresses:  []string{"10.0.1.2"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(false)},
				},
			},
		},
	}

	client := fake.NewClientset(endpointSliceList...)
	informerFactory := kubeinformers.NewSharedInformerFactory(client, 0)
	source := newDiscoveryV1EndpointSlicesServiceEndpointsSource(informerFactory)

	ctx := t.Context()
	informerFactory.Start(ctx.Done())

	// Wait for cache sync
	if !cacheSyncWithTimeout(informerFactory.Discovery().V1().EndpointSlices().Informer(), 5*time.Second) {
		t.Fatal("failed to sync endpoint slices informer")
	}

	t.Run("with an existing service", func(t *testing.T) {
		t.Parallel()
		endpoints, err := source.GetServiceEndpoints(svc)
		require.NoError(t, err)
		require.Len(t, endpoints, 4)
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.1", Ready: true})
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.2", Ready: false})
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.3", Ready: true})
		require.Contains(t, endpoints, serviceEndpoint{Address: "10.0.0.4", Ready: false})
	})

	t.Run("with a service that does not exist", func(t *testing.T) {
		t.Parallel()
		endpoints, err := source.GetServiceEndpoints(svcNonExistent)
		require.NoError(t, err)
		require.Empty(t, endpoints)
	})
}

func TestConditionToBool(t *testing.T) {
	t.Parallel()

	require.True(t, conditionToBool(nil)) // nil Ready condition is considered ready
	require.True(t, conditionToBool(boolPtr(true)))
	require.False(t, conditionToBool(boolPtr(false)))
}

// cacheSyncWithTimeout waits for the cache to sync and returns true if synced
func cacheSyncWithTimeout(informer cache.SharedIndexInformer, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
}

func boolPtr(b bool) *bool {
	return &b
}
