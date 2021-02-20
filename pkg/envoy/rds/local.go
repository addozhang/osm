package rds

import (
	"fmt"
	set "github.com/deckarep/golang-set"
	"github.com/openservicemesh/osm/pkg/catalog"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/openservicemesh/osm/pkg/trafficpolicy"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
)

//This function will attach pod ips to specified virtual_host.
//1. if virtual_host is route's root service, will attach ips to it
//2. if route's backend service, will NOT attach ips to it
//3. if neither two, will attache to one of them
func aggregateRoutesByAddress(outboundAggregatedRoutesByHostnames map[string]map[string]trafficpolicy.RouteWeightedClusters, cataloger catalog.MeshCataloger, proxyServiceName service.MeshService, allTrafficSplits []*split.TrafficSplit, allTrafficPolicies []trafficpolicy.TrafficTarget) {

	allAddresses := set.NewSet()
	svcEndpointSet := map[string]set.Set{}
	for _, trafficPolicy := range allTrafficPolicies {
		isSourceService := trafficPolicy.Source.Equals(proxyServiceName)
		svc := trafficPolicy.Destination
		if isSourceService {
			if isBackendService(svc, allTrafficSplits) {
				continue //not map pod ip to backend service
			}
			endpoints, err := cataloger.ListEndpointsForService(svc)
			if err != nil || len(endpoints) == 0 {
				continue
			}
			for _, ep := range endpoints {
				if svcEndpointSet[svc.Name] == nil {
					svcEndpointSet[svc.Name] = set.NewSet()
				}
				address := fmt.Sprintf("%s:%d", ep.IP.String(), ep.Port)
				if !allAddresses.Contains(address) { //if pod ip not map to service (parent or prod) yet
					allAddresses.Add(address)
					svcEndpointSet[svc.Name].Add(address)
				}
			}
		}
	}

	for hostname, routePolicyWeightedCluster := range outboundAggregatedRoutesByHostnames {

		eps, exists := svcEndpointSet[hostname]
		if !exists || eps.Cardinality() == 0 {
			continue
		}

		for pathRegex, cluster := range routePolicyWeightedCluster {
			cluster.Hostnames = cluster.Hostnames.Union(eps)
			routePolicyWeightedCluster[pathRegex] = cluster
		}
	}
}

func isBackendService(svc service.MeshService, allTrafficSplits []*split.TrafficSplit) bool {
	for _, split := range allTrafficSplits {
		if split.Namespace != svc.Namespace {
			continue
		}
		if split.Spec.Service == svc.Name {
			return false //attach ips to root service
		}
		for _, backend := range split.Spec.Backends {
			if backend.Service == svc.Name {
				return true //do not attach ips to backend service
			}
		}
	}
	return false
}
