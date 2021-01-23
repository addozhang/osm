package rds

import (
	"fmt"
	set "github.com/deckarep/golang-set"
	"github.com/openservicemesh/osm/pkg/catalog"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/openservicemesh/osm/pkg/trafficpolicy"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"strings"
)

//This function will attach pod ips to specified virtual_host.
//1. if virtual_host is route's root service, will attach ips to it
//2. if route's backend service, will NOT attach ips to it
//3. if neither two, will attache to one of them
func aggregateRoutesByAddress(outboundAggregatedRoutesByHostnames map[string]map[string]trafficpolicy.RouteWeightedClusters, allTrafficSplits []*split.TrafficSplit, catalog catalog.MeshCataloger) {
	allAddresses := set.NewSet()
	for hostname, routePolicyWeightedCluster := range outboundAggregatedRoutesByHostnames {
		var namespace string
		for _, cluster := range routePolicyWeightedCluster {
			if cluster.WeightedClusters.Cardinality() > 0 {
				for v := range cluster.WeightedClusters.Iterator().C {
					c := v.(service.WeightedCluster)
					namespace = strings.Split(c.ClusterName.String(), "/")[0]
					break
				}
			}
		}
		hostService := service.MeshService{
			Namespace: namespace,
			Name:      hostname,
		}

		if !shouldAttachePodIPs(hostService, allTrafficSplits) {
			continue
		}

		endpoints, err := catalog.ListEndpointsForService(hostService)
		if err != nil {
			continue
		}
		if len(endpoints) == 0 {
			continue
		}

		addresses := set.NewSet()
		for _, ep := range endpoints {
			addr := fmt.Sprintf("%s:%d", ep.IP.String(), ep.Port)
			if !allAddresses.Contains(addr) {
				addresses.Add(addr)
				allAddresses.Add(addr)
			} else {
				fmt.Printf("==%s==%s==\n", hostname, addr)
			}
		}
		if addresses.Cardinality() == 0 {
			continue
		}
		for pathRegex, cluster := range routePolicyWeightedCluster {
			cluster.Hostnames = cluster.Hostnames.Union(addresses)
			routePolicyWeightedCluster[pathRegex] = cluster
		}
	}
}

func shouldAttachePodIPs(svc service.MeshService, allTrafficSplits []*split.TrafficSplit) bool {
	for _, split := range allTrafficSplits {
		if split.Namespace != svc.Namespace {
			continue
		}
		if split.Spec.Service == svc.Name {
			return true //attach ips to root service
		}
		for _, backend := range split.Spec.Backends {
			if backend.Service == svc.Name {
				return false //do not attach ips to backend service
			}
		}
	}
	return true
}
