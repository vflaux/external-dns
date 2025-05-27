package source

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	discoveryinformers "k8s.io/client-go/informers/discovery/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	_ serviceEndpointsSource = &coreV1EndpointsServiceEndpointSource{}
	_ serviceEndpointsSource = &discoveryV1EndpointSlicesServiceEndpointsSource{}
)

type serviceEndpointsSource interface {
	GetServiceEndpoints(svc *v1.Service) ([]serviceEndpoint, error)
	AddEventHandler(handler cache.ResourceEventHandler)
}

type serviceEndpoint struct {
	Address   string
	TargetRef *v1.ObjectReference
	Ready     bool
}

type coreV1EndpointsServiceEndpointSource struct {
	informer coreinformers.EndpointsInformer
}

func newCoreV1EndpointsServiceEndpointSource(informerFactory kubeinformers.SharedInformerFactory) *coreV1EndpointsServiceEndpointSource {
	informer := informerFactory.Core().V1().Endpoints()
	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {},
		},
	)
	return &coreV1EndpointsServiceEndpointSource{informer: informer}
}

func (s *coreV1EndpointsServiceEndpointSource) GetServiceEndpoints(svc *v1.Service) ([]serviceEndpoint, error) {
	var serviceEndpoints []serviceEndpoint

	ep, err := s.informer.Lister().Endpoints(svc.Namespace).Get(svc.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Debugf("Service %s/%s has no endpoint", svc.Namespace, svc.Name)
			return serviceEndpoints, nil
		}
		return nil, fmt.Errorf("failed to get endpoints for service %s/%s: %w", svc.Namespace, svc.Name, err)
	}

	for _, subset := range ep.Subsets {
		for _, address := range subset.Addresses {
			serviceEndpoints = append(serviceEndpoints, serviceEndpoint{
				Address:   address.IP,
				TargetRef: address.TargetRef,
				Ready:     true,
			})
		}
		for _, address := range subset.NotReadyAddresses {
			serviceEndpoints = append(serviceEndpoints, serviceEndpoint{
				Address:   address.IP,
				TargetRef: address.TargetRef,
				Ready:     false,
			})
		}
	}

	return serviceEndpoints, nil
}

func (s *coreV1EndpointsServiceEndpointSource) AddEventHandler(handler cache.ResourceEventHandler) {
	s.informer.Informer().AddEventHandler(handler)
}

type discoveryV1EndpointSlicesServiceEndpointsSource struct {
	informer discoveryinformers.EndpointSliceInformer
}

func newDiscoveryV1EndpointSlicesServiceEndpointsSource(informerFactory kubeinformers.SharedInformerFactory) *discoveryV1EndpointSlicesServiceEndpointsSource {
	informer := informerFactory.Discovery().V1().EndpointSlices()
	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {},
		},
	)
	return &discoveryV1EndpointSlicesServiceEndpointsSource{informer: informer}
}

func (s *discoveryV1EndpointSlicesServiceEndpointsSource) GetServiceEndpoints(svc *v1.Service) ([]serviceEndpoint, error) {
	var addresses []serviceEndpoint

	selector := labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: svc.Name})

	endpointSlices, err := s.informer.Lister().EndpointSlices(svc.Namespace).List(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpointslices for service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	if len(endpointSlices) == 0 {
		log.Debugf("Service %s/%s has no EndpointSlices", svc.Namespace, svc.Name)
		return addresses, nil
	}

	for _, endpointSlice := range endpointSlices {
		if endpointSlice.AddressType != discoveryv1.AddressTypeIPv4 && endpointSlice.AddressType != discoveryv1.AddressTypeIPv6 {
			log.Debugf("Skipping EndpointSlice %s/%s because its address type is unsupported: %s", endpointSlice.Namespace, endpointSlice.Name, endpointSlice.AddressType)
			continue
		}
		for _, ep := range endpointSlice.Endpoints {
			if len(ep.Addresses) == 0 {
				log.Warnf("EndpointSlice %s/%s has no addresses for endpoint %v", endpointSlice.Namespace, endpointSlice.Name, ep)
				continue
			}
			address := ep.Addresses[0] // Take the first address, as per EndpointSlice spec
			addresses = append(addresses, serviceEndpoint{
				Address:   address,
				TargetRef: ep.TargetRef,
				Ready:     conditionToBool(ep.Conditions.Ready),
			})
		}
	}

	return addresses, nil
}

func (s *discoveryV1EndpointSlicesServiceEndpointsSource) AddEventHandler(handler cache.ResourceEventHandler) {
	s.informer.Informer().AddEventHandler(handler)
}

// conditionToBool converts an EndpointConditions condition to a bool value.
func conditionToBool(v *bool) bool {
	if v == nil {
		return true // nil means true
	}
	return *v
}
