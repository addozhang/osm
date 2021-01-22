package lds

import (
	"fmt"
	mapset "github.com/deckarep/golang-set"
	xds_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	xds_listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/pkg/errors"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"sort"
)

func (lb *listenerBuilder) getOutboundHTTPFilterChainForPod(upstream service.MeshService, port uint32) (*xds_listener.FilterChain, error) {
	// Get HTTP filter for service
	filter, err := lb.getOutboundHTTPFilter()
	if err != nil {
		log.Error().Err(err).Msgf("Error getting HTTP filter for upstream service %s", upstream)
		return nil, err
	}

	// Get filter match criteria for destination service
	filterChainMatch, err := lb.getOutboundFilterChainMatchForPod(upstream, port)
	if err != nil {
		log.Error().Err(err).Msgf("Error getting HTTP filter chain match for upstream service %s", upstream)
		return nil, err
	}

	filterChainName := fmt.Sprintf("%s:%s:pods", outboundMeshHTTPFilterChainPrefix, upstream)
	return &xds_listener.FilterChain{
		Name:             filterChainName,
		Filters:          []*xds_listener.Filter{filter},
		FilterChainMatch: filterChainMatch,
	}, nil
}

// getOutboundFilterChainMatchForService builds a filter chain to match the HTTP or TCP based destination traffic.
// Filter Chain currently matches on the following:
// 1. Destination IP of service endpoints
// 2. Destination port of the service
func (lb *listenerBuilder) getOutboundFilterChainMatchForPod(dstSvc service.MeshService, port uint32) (*xds_listener.FilterChainMatch, error) {
	filterMatch := &xds_listener.FilterChainMatch{
		DestinationPort: &wrapperspb.UInt32Value{
			Value: port,
		},
	}
	endpoints, err := lb.meshCatalog.ListEndpointsForService(dstSvc)
	if err != nil {
		log.Error().Err(err).Msgf("Error getting GetResolvableServiceEndpoints for %q", dstSvc)
		return nil, err
	}

	if len(endpoints) == 0 {
		err := errors.Errorf("Endpoints not found for service %q", dstSvc)
		log.Error().Err(err).Msgf("Error constructing HTTP filter chain match for service %q", dstSvc)
		return nil, err
	}

	endpointSet := mapset.NewSet()
	for _, endp := range endpoints {
		endpointSet.Add(endp.IP.String())
	}

	// For deterministic ordering
	sortedEndpoints := []string{}
	endpointSet.Each(func(elem interface{}) bool {
		sortedEndpoints = append(sortedEndpoints, elem.(string))
		return false
	})
	sort.Strings(sortedEndpoints)

	for _, ip := range sortedEndpoints {
		filterMatch.PrefixRanges = append(filterMatch.PrefixRanges, &xds_core.CidrRange{
			AddressPrefix: ip,
			PrefixLen: &wrapperspb.UInt32Value{
				Value: singleIpv4Mask,
			},
		})
	}

	return filterMatch, nil
}

func (lb *listenerBuilder) allTrafficSplits() []*split.TrafficSplit {
	allTrafficSplits, _, _, _, _ := lb.meshCatalog.ListSMIPolicies()
	return allTrafficSplits
}

func isTrafficSplitService(svc service.MeshService, allTrafficSplits []*split.TrafficSplit) bool {
	for _, trafficSplit := range allTrafficSplits {
		if trafficSplit.Namespace == svc.Namespace && trafficSplit.Spec.Service == svc.Name {
			return true
		}
	}
	return false
}
